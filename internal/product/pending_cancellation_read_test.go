package product

import (
	"context"
	"errors"
	"github.com/nathanxiangang-web/panta/internal/acquisition"
	"github.com/nathanxiangang-web/panta/internal/jobs"
	"testing"
)

func TestCanceledWithoutJobOwnerReadDoesNotWeakenOtherPairs(t *testing.T) {
	s, f := newFixture(t)
	f.record = pending("95000000-0000-4000-8000-000000000003")
	f.record.Manifest.State = acquisition.StateCanceled
	r, e := s.Task(context.Background(), principal(), f.record.Manifest.ID)
	if e != nil || r.ManifestState != acquisition.StateCanceled || r.JobID != nil || r.JobState != nil || r.ClaimAttempts != 0 || r.FailureCount != 0 {
		t.Fatal(r, e)
	}
	for _, state := range []acquisition.State{acquisition.StateActive, acquisition.StateFailed, acquisition.StateRecoveryRequired, acquisition.StateAwaitingVisibility} {
		f.record.Manifest.State = state
		if _, e = s.Task(context.Background(), principal(), f.record.Manifest.ID); !errors.Is(e, ErrUnavailable) {
			t.Fatal("broken pair accepted", state, e)
		}
	}
	f.record.Manifest.State = acquisition.StateCanceled
	f.record.Job = &jobs.Job{}
	if _, e = s.Task(context.Background(), principal(), f.record.Manifest.ID); !errors.Is(e, ErrUnavailable) {
		t.Fatal("unexpected job accepted", e)
	}
}
