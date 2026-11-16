package acquisition

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nathanxiangang-web/panta/internal/jobs"
	"github.com/nathanxiangang-web/panta/internal/providers/contracts"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

type copyDonorStoreDouble struct {
	candidate CopyDonorCandidate
	err       error
}

func (store copyDonorStoreDouble) FindCopyDonor(context.Context, string, string, storage.ConnectionID, ManifestID) (CopyDonorCandidate, error) {
	if store.err != nil {
		return CopyDonorCandidate{}, store.err
	}
	return store.candidate, nil
}

func TestCopyCoordinatorFallsBackWhenNoDonorExists(t *testing.T) {
	fixture := newCopyPlanFixture()
	provider := &fakeObjectCopyProvider{}
	coordinator, err := NewCopyCoordinator(copyDonorStoreDouble{err: ErrCopyDonorUnavailable}, &fakeCopyStore{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	result, handled, err := coordinator.ExecuteCopy(context.Background(), fixture.requester,
		jobs.Job{ID: *fixture.requester.JobID, Type: JobTypeAcquisition, State: jobs.StateRunning}, provider, "worker", 1)
	if err != nil || handled || result.Outcome != "" {
		t.Fatalf("result = %#v, handled = %t, error = %v", result, handled, err)
	}
	if provider.copyCalls != 0 || provider.listCalls != 0 || provider.inspectCalls != 0 {
		t.Fatal("no-donor path performed provider IO")
	}
}

func TestCopyCoordinatorExecutesSingleCopyToReadyOutcome(t *testing.T) {
	fixture := newCopyPlanFixture()
	providerCopied := copiedObjectFromDonor(fixture.evidence, *fixture.requester.TargetProviderScope, "file-b")
	provider := &fakeObjectCopyProvider{donor: objectFromDonor(fixture.evidence), candidate: providerCopied,
		listResults: [][]contracts.StorageObject{{}, {}, {providerCopied}}}
	store := &fakeCopyStore{}
	donor := CopyDonorCandidate{Manifest: fixture.donor, Job: fixture.job, Task: fixture.task, Evidence: fixture.evidence}
	coordinator, err := NewCopyCoordinator(copyDonorStoreDouble{candidate: donor}, store, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	result, handled, err := coordinator.ExecuteCopy(context.Background(), fixture.requester,
		jobs.Job{ID: *fixture.requester.JobID, Type: JobTypeAcquisition, State: jobs.StateRunning}, provider, "worker", 1)
	if err != nil || !handled || result.Outcome != OutcomeProviderSucceeded || result.ResultName == nil {
		t.Fatalf("result = %#v, handled = %t, error = %v", result, handled, err)
	}
	if provider.copyCalls != 1 || store.operation == nil || store.operation.Status != CopyProviderConfirmed {
		t.Fatalf("calls = %d, operation = %#v", provider.copyCalls, store.operation)
	}
}

func TestCopyCoordinatorRejectsNonRunningRequesterJob(t *testing.T) {
	fixture := newCopyPlanFixture()
	donor := CopyDonorCandidate{Manifest: fixture.donor, Job: fixture.job, Task: fixture.task, Evidence: fixture.evidence}
	coordinator, err := NewCopyCoordinator(copyDonorStoreDouble{candidate: donor}, &fakeCopyStore{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	_, handled, err := coordinator.ExecuteCopy(context.Background(), fixture.requester,
		jobs.Job{ID: *fixture.requester.JobID, Type: JobTypeAcquisition, State: jobs.StateQueued}, &fakeObjectCopyProvider{}, "worker", 1)
	if !handled || !errors.Is(err, ErrExecutionJobMismatch) {
		t.Fatalf("handled = %t, error = %v", handled, err)
	}
}
