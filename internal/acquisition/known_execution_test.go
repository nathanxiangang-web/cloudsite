package acquisition_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nathanxiangang-web/panta/internal/acquisition"
)

func TestKnownPollOnlyNeverStartsOnMissingOrChangedReference(t *testing.T) {
	for _, mutation := range []string{"missing", "unknown", "wrong-reference", "wrong-provider", "wrong-connection", "wrong-job"} {
		t.Run(mutation, func(t *testing.T) {
			fixture := newExecutionFixture(t)
			manifest := executionManifest()
			scope := acquisition.KnownPollScope{ManifestID: executionManifestID, JobID: executionJobID, BindingID: executionBindingID, ConnectionID: executionConnID, ProviderID: executionProviderID, RootID: "root-1", ReferenceSHA256: acquisition.EvidenceSHA256("known"), SourceSHA256: acquisition.EvidenceSHA256(manifest.SourceRef)}
			task := acquisition.ProviderTask{ManifestID: executionManifestID, JobID: executionJobID, ProviderID: executionProviderID, ProviderTaskRef: "known", State: acquisition.ProviderTaskReferenceKnown, CreatedAt: executionNow, UpdatedAt: executionNow}
			switch mutation {
			case "unknown":
				task.State = acquisition.ProviderTaskStartReserved
				task.ProviderTaskRef = ""
			case "wrong-reference":
				task.ProviderTaskRef = "other"
			case "wrong-provider":
				scope.ProviderID = "other"
			case "wrong-connection":
				scope.ConnectionID = "other"
			case "wrong-job":
				scope.JobID = "other"
			}
			if mutation != "missing" {
				fixture.tasks.values[executionManifestID] = task
			}
			inputs, _ := acquisition.NewExecutionInputResolver(fixture.manifests, executionTopology("library"))
			service, err := acquisition.NewExecutionStepService(inputs, fixture.manifests, fixture.jobsvc, fixture.sessions, fixture.tasks, acquisition.WithExecutionClock(func() time.Time { return executionNow }), acquisition.WithKnownPollOnly(scope))
			if err != nil {
				t.Fatal(err)
			}
			_, err = service.Execute(context.Background(), fixture.request)
			if err == nil || !(errors.Is(err, acquisition.ErrKnownRecoveryConflict) || errors.Is(err, acquisition.ErrExecutionSideEffectUncertain)) {
				t.Fatalf("error=%v", err)
			}
			start, status, _ := fixture.provider.counts()
			if start != 0 || status != 0 || fixture.tasks.claimCalls != 0 {
				t.Fatal("poll-only guard permitted a provider operation")
			}
		})
	}
}
