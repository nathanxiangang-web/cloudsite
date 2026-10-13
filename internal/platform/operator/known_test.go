package operator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/nathanxiangang-web/panta/internal/acquisition"
	"github.com/nathanxiangang-web/panta/internal/app"
	"github.com/nathanxiangang-web/panta/internal/jobs"
)

func TestKnownReservationIsDurableExclusiveAndAuthorizationBound(t *testing.T) {
	path := filepath.Join(knownTestDirectory(t), "known-go.json")
	digest := acquisition.EvidenceSHA256("approved bytes")
	if reserveKnownAction(path, StartKnown, digest) == nil {
		t.Fatal("start without resume reservation")
	}
	var wait sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wait.Add(1)
		go func() { defer wait.Done(); results <- reserveKnownAction(path, ResumeKnown, digest) }()
	}
	wait.Wait()
	accepted := 0
	for i := 0; i < 8; i++ {
		if <-results == nil {
			accepted++
		}
	}
	if accepted != 1 {
		t.Fatalf("reservations=%d", accepted)
	}
	if reserveKnownAction(path, StartKnown, acquisition.EvidenceSHA256("changed bytes")) == nil {
		t.Fatal("start adopted changed authorization")
	}
	if err := reserveKnownAction(path, StartKnown, digest); err != nil {
		t.Fatal(err)
	}
	if reserveKnownAction(path, StartKnown, digest) == nil {
		t.Fatal("start repeated after reservation/crash")
	}
}

func TestKnownAuthorizationDefaultDeniedExpiredAndBounded(t *testing.T) {
	now := time.Now().UTC()
	digest := acquisition.EvidenceSHA256("synthetic")
	request := acquisition.KnownRecoveryRequest{Scope: acquisition.KnownPollScope{ManifestID: "m", JobID: "j", BindingID: "b", ConnectionID: "c", ProviderID: "115", RootID: "r", ReferenceSHA256: digest, SourceSHA256: digest}, ExpectedClaim: 22, ManifestUpdatedAt: now, JobUpdatedAt: now, RetryDelay: time.Second}
	for _, test := range []struct {
		name    string
		execute bool
		expiry  time.Time
		valid   bool
	}{{"default off", false, now.Add(time.Minute), false}, {"expired", true, now, false}, {"over budget", true, now.Add(21 * time.Minute), false}, {"valid", true, now.Add(time.Minute), true}} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(knownTestDirectory(t), "go.json")
			data, _ := json.Marshal(knownAuthorization{Execute: test.execute, ExpiresAt: test.expiry, Request: request, BinarySHA256: digest})
			// Test fixtures are local synthetic files, never production authorizations.
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			input, err := readKnownAuthorization(path, now)
			if (err == nil) != test.valid {
				t.Fatalf("valid=%v err=%v", test.valid, err)
			}
			if test.valid && input.digest != acquisition.EvidenceSHA256(string(data)) {
				t.Fatal("authorization digest differs")
			}
		})
	}
}

func knownTestDirectory(t *testing.T) string {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("protected mount requires Linux ownership semantics")
	}
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	return directory
}

func TestKnownStopPolicyDistinguishesCommittedRetryFromFatalError(t *testing.T) {
	if knownMustStop(app.WorkerEvent{Kind: app.WorkerStage, JobState: jobs.StateRetryWait, PollHandoff: "RETRY_WAIT", DiagnosticCategory: "POLL_TRANSPORT_TIMEOUT"}) {
		t.Fatal("committed retry mistaken for FIRST_ERROR_STOP")
	}
	for _, event := range []app.WorkerEvent{{Kind: app.WorkerTransientError}, {Kind: app.WorkerFatalRecoveryDebt}, {Kind: app.WorkerStage, JobState: jobs.StateRecoveryRequired}, {Kind: app.WorkerStage, JobState: jobs.StateSucceeded}} {
		if !knownMustStop(event) {
			t.Fatal("scoped worker did not stop")
		}
	}
}
