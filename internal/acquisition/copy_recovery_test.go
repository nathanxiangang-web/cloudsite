package acquisition

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nathanxiangang-web/panta/internal/providers/contracts"
)

func TestCopyExecutorConfirmedReplayPerformsNoProviderIO(t *testing.T) {
	fixture := newCopyPlanFixture()
	plan := fixture.plan(t)
	operation := plan.Operation
	resultRef := "file-b"
	operation.Status = CopyProviderConfirmed
	operation.ProviderResultRef = &resultRef
	store := &fakeCopyStore{operation: &operation}
	provider := &fakeObjectCopyProvider{}
	executor := newCopyExecutorForTest(store, provider)
	result, err := executor.Execute(context.Background(), plan, fixture.requester)
	if err != nil || result.Operation.Status != CopyProviderConfirmed {
		t.Fatalf("result = %#v, error = %v", result, err)
	}
	if provider.inspectCalls != 0 || provider.listCalls != 0 || provider.copyCalls != 0 {
		t.Fatalf("unexpected provider IO: inspect=%d list=%d copy=%d", provider.inspectCalls, provider.listCalls, provider.copyCalls)
	}
}

type donorDeletedRecoveryProvider struct {
	candidate    contracts.StorageObject
	inspectCalls int
}

func (provider *donorDeletedRecoveryProvider) InspectObject(_ context.Context, reference contracts.StorageObjectReference) (contracts.StorageObject, error) {
	provider.inspectCalls++
	if reference.Path == provider.candidate.Reference.Path {
		return provider.candidate, nil
	}
	return contracts.StorageObject{}, errors.New("donor deleted")
}

func (provider *donorDeletedRecoveryProvider) ListObjects(context.Context, string) ([]contracts.StorageObject, error) {
	return []contracts.StorageObject{provider.candidate}, nil
}

func (provider *donorDeletedRecoveryProvider) CopyObject(context.Context, contracts.StorageObjectReference, string) error {
	return errors.New("unexpected copy")
}

func TestCopyExecutorUnknownRecoveryDoesNotRequireDonor(t *testing.T) {
	fixture := newCopyPlanFixture()
	plan := fixture.plan(t)
	operation := plan.Operation
	operation.Status = CopyProviderUnknown
	store := &fakeCopyStore{operation: &operation}
	candidate := copiedObjectFromDonor(fixture.evidence, *fixture.requester.TargetProviderScope, "file-b")
	provider := &donorDeletedRecoveryProvider{candidate: candidate}
	executor := newCopyExecutorForTest(store, provider)
	result, err := executor.Execute(context.Background(), plan, fixture.requester)
	if err != nil || result.Operation.Status != CopyProviderConfirmed || provider.inspectCalls != 1 {
		t.Fatalf("result = %#v, inspect = %d, error = %v", result, provider.inspectCalls, err)
	}
}

func TestCopyExecutorRejectsStaleBaselineBeforeWrite(t *testing.T) {
	fixture := newCopyPlanFixture()
	plan, err := NewCopyPlan(fixture.requester, fixture.donor, fixture.job, fixture.task,
		fixture.evidence, "copy-b", []string{"stale-file"}, planTimestamp())
	if err != nil {
		t.Fatal(err)
	}
	store := &fakeCopyStore{}
	provider := &fakeObjectCopyProvider{donor: objectFromDonor(fixture.evidence)}
	executor := newCopyExecutorForTest(store, provider)
	if _, err := executor.Execute(context.Background(), plan, fixture.requester); !errors.Is(err, ErrCopyDestinationConflict) {
		t.Fatalf("error = %v, want ErrCopyDestinationConflict", err)
	}
	if store.operation != nil || provider.copyCalls != 0 {
		t.Fatalf("unexpected side effect: operation = %#v, calls = %d", store.operation, provider.copyCalls)
	}
}

func TestCopyExecutorReturnsUnknownPersistenceFailure(t *testing.T) {
	fixture := newCopyPlanFixture()
	plan := fixture.plan(t)
	updateErr := errors.New("database update failed")
	store := &fakeCopyStore{updateErr: updateErr}
	provider := &fakeObjectCopyProvider{donor: objectFromDonor(fixture.evidence), copyErr: errors.New("timeout")}
	executor := newCopyExecutorForTest(store, provider)
	result, err := executor.Execute(context.Background(), plan, fixture.requester)
	if !errors.Is(err, ErrCopyProviderOutcomeUnknown) || !errors.Is(err, updateErr) || !result.RequiresRecovery {
		t.Fatalf("result = %#v, error = %v", result, err)
	}
}

func TestCopyExecutorRejectsNilScopeBeforeIO(t *testing.T) {
	fixture := newCopyPlanFixture()
	plan := fixture.plan(t)
	fixture.requester.TargetProviderScope = nil
	store := &fakeCopyStore{}
	provider := &fakeObjectCopyProvider{}
	executor := newCopyExecutorForTest(store, provider)
	if _, err := executor.Execute(context.Background(), plan, fixture.requester); !errors.Is(err, ErrCopyIdentityMismatch) {
		t.Fatalf("error = %v, want ErrCopyIdentityMismatch", err)
	}
	if provider.inspectCalls != 0 || provider.listCalls != 0 || provider.copyCalls != 0 || store.operation != nil {
		t.Fatal("nil scope caused external or persistent side effects")
	}
}

func planTimestamp() (result time.Time) {
	return time.Unix(100, 0)
}
