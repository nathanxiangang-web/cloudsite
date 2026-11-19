package acquisition

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nathanxiangang-web/panta/internal/providers/contracts"
)

type fakeCopyStore struct {
	operation *CopyOperation
	updates   int
	updateErr error
}

func (store *fakeCopyStore) CreateCopyOperation(_ context.Context, operation CopyOperation) error {
	copy := operation
	store.operation = &copy
	return nil
}

func (store *fakeCopyStore) CreateCopyOperationWithLease(_ context.Context, operation CopyOperation, _ string, _ int) error {
	return store.CreateCopyOperation(context.Background(), operation)
}

func (store *fakeCopyStore) GetCopyOperation(context.Context, ManifestID) (CopyOperation, error) {
	if store.operation == nil {
		return CopyOperation{}, ErrCopyOperationNotFound
	}
	return *store.operation, nil
}

func (store *fakeCopyStore) UpdateCopyOperation(_ context.Context, operation CopyOperation, _ time.Time) error {
	copy := operation
	store.operation = &copy
	store.updates++
	return store.updateErr
}

type fakeObjectCopyProvider struct {
	donor        contracts.StorageObject
	candidate    contracts.StorageObject
	listResults  [][]contracts.StorageObject
	inspectErr   error
	listErr      error
	copyErr      error
	copyCalls    int
	inspectCalls int
	listCalls    int
}

func (provider *fakeObjectCopyProvider) InspectObject(_ context.Context, reference contracts.StorageObjectReference) (contracts.StorageObject, error) {
	provider.inspectCalls++
	if provider.inspectErr != nil {
		return contracts.StorageObject{}, provider.inspectErr
	}
	if provider.candidate.Reference.Path == reference.Path {
		return provider.candidate, nil
	}
	return provider.donor, nil
}

func (provider *fakeObjectCopyProvider) ListObjects(context.Context, string) ([]contracts.StorageObject, error) {
	provider.listCalls++
	if provider.listErr != nil {
		return nil, provider.listErr
	}
	if len(provider.listResults) == 0 {
		return []contracts.StorageObject{}, nil
	}
	result := provider.listResults[0]
	if len(provider.listResults) > 1 {
		provider.listResults = provider.listResults[1:]
	}
	return result, nil
}

func (provider *fakeObjectCopyProvider) CopyObject(context.Context, contracts.StorageObjectReference, string) error {
	provider.copyCalls++
	return provider.copyErr
}

func objectFromDonor(evidence ResultObjectEvidence) contracts.StorageObject {
	return contracts.StorageObject{
		Reference: contracts.StorageObjectReference{Scope: evidence.ProviderParentScope, Path: evidence.ProviderObjectRef},
		Name:      evidence.ResultName, SizeBytes: evidence.SizeBytes,
		HashAlgorithm: evidence.HashAlgorithm, ContentHash: evidence.ContentHash,
	}
}

func copiedObjectFromDonor(evidence ResultObjectEvidence, scope, id string) contracts.StorageObject {
	object := objectFromDonor(evidence)
	object.Reference = contracts.StorageObjectReference{Scope: scope, Path: id}
	return object
}

func newCopyExecutorForTest(store CopyStore, provider contracts.ObjectCopyProvider) *CopyExecutor {
	executor, err := NewCopyExecutor(store, provider)
	if err != nil {
		panic(err)
	}
	return executor
}

func TestCopyExecutorConfirmsOnceAndReplaysWithoutCopy(t *testing.T) {
	fixture := newCopyPlanFixture()
	plan := fixture.plan(t)
	store := &fakeCopyStore{}
	copied := copiedObjectFromDonor(fixture.evidence, *fixture.requester.TargetProviderScope, "file-b")
	provider := &fakeObjectCopyProvider{donor: objectFromDonor(fixture.evidence), candidate: copied,
		listResults: [][]contracts.StorageObject{{}, {copied}, {copied}}}
	executor := newCopyExecutorForTest(store, provider)
	result, err := executor.Execute(context.Background(), plan, fixture.requester)
	if err != nil || result.Operation.Status != CopyProviderConfirmed || !result.CopyAttempted {
		t.Fatalf("result = %#v, error = %v", result, err)
	}
	if provider.copyCalls != 1 || result.Operation.ProviderResultRef == nil || *result.Operation.ProviderResultRef != "file-b" {
		t.Fatalf("calls = %d, result = %#v", provider.copyCalls, result)
	}
	replayed, err := executor.Execute(context.Background(), plan, fixture.requester)
	if err != nil || replayed.Operation.Status != CopyProviderConfirmed || provider.copyCalls != 1 {
		t.Fatalf("replayed = %#v, calls = %d, error = %v", replayed, provider.copyCalls, err)
	}
}

func TestCopyExecutorExistingReservationOnlyReconciles(t *testing.T) {
	fixture := newCopyPlanFixture()
	plan := fixture.plan(t)
	operation := plan.Operation
	store := &fakeCopyStore{operation: &operation}
	copied := copiedObjectFromDonor(fixture.evidence, *fixture.requester.TargetProviderScope, "file-b")
	provider := &fakeObjectCopyProvider{donor: objectFromDonor(fixture.evidence), candidate: copied,
		listResults: [][]contracts.StorageObject{{copied}, {copied}}}
	executor := newCopyExecutorForTest(store, provider)
	result, err := executor.Execute(context.Background(), plan, fixture.requester)
	if err != nil || result.CopyAttempted || provider.copyCalls != 0 || result.Operation.Status != CopyProviderConfirmed {
		t.Fatalf("result = %#v, calls = %d, error = %v", result, provider.copyCalls, err)
	}
}

func TestCopyExecutorUnknownNeverRetriesWrite(t *testing.T) {
	fixture := newCopyPlanFixture()
	plan := fixture.plan(t)
	store := &fakeCopyStore{}
	provider := &fakeObjectCopyProvider{donor: objectFromDonor(fixture.evidence), copyErr: errors.New("timeout")}
	executor := newCopyExecutorForTest(store, provider)
	result, err := executor.Execute(context.Background(), plan, fixture.requester)
	if !errors.Is(err, ErrCopyProviderOutcomeUnknown) || !result.RequiresRecovery || provider.copyCalls != 1 {
		t.Fatalf("result = %#v, calls = %d, error = %v", result, provider.copyCalls, err)
	}
	if store.operation == nil || store.operation.Status != CopyProviderUnknown {
		t.Fatalf("stored operation = %#v", store.operation)
	}
	replayed, err := executor.Execute(context.Background(), plan, fixture.requester)
	if err != nil || replayed.CopyAttempted || provider.copyCalls != 1 || !replayed.RequiresRecovery {
		t.Fatalf("replayed = %#v, calls = %d, error = %v", replayed, provider.copyCalls, err)
	}
}

func TestCopyExecutorRejectsDestinationBeforeCreatingOperation(t *testing.T) {
	fixture := newCopyPlanFixture()
	plan := fixture.plan(t)
	store := &fakeCopyStore{}
	existing := copiedObjectFromDonor(fixture.evidence, *fixture.requester.TargetProviderScope, "existing")
	provider := &fakeObjectCopyProvider{donor: objectFromDonor(fixture.evidence),
		listResults: [][]contracts.StorageObject{{existing}}}
	executor := newCopyExecutorForTest(store, provider)
	if _, err := executor.Execute(context.Background(), plan, fixture.requester); !errors.Is(err, ErrCopyDestinationConflict) {
		t.Fatalf("error = %v, want ErrCopyDestinationConflict", err)
	}
	if store.operation != nil || provider.copyCalls != 0 {
		t.Fatalf("unexpected side effect: operation = %#v, calls = %d", store.operation, provider.copyCalls)
	}
}

func TestCopyExecutorRejectsChangedDonorBeforePersistence(t *testing.T) {
	fixture := newCopyPlanFixture()
	plan := fixture.plan(t)
	changed := objectFromDonor(fixture.evidence)
	changed.ContentHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	provider := &fakeObjectCopyProvider{donor: changed}
	executor := newCopyExecutorForTest(&fakeCopyStore{}, provider)
	if _, err := executor.Execute(context.Background(), plan, fixture.requester); !errors.Is(err, ErrCopyDonorUnavailable) {
		t.Fatalf("error = %v, want ErrCopyDonorUnavailable", err)
	}
	if provider.copyCalls != 0 {
		t.Fatal("changed donor triggered copy")
	}
}
