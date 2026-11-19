package acquisition

import (
	"bytes"
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/nathanxiangang-web/panta/internal/providers/contracts"
)

type CopyStore interface {
	CreateCopyOperation(context.Context, CopyOperation) error
	GetCopyOperation(context.Context, ManifestID) (CopyOperation, error)
	UpdateCopyOperation(context.Context, CopyOperation, time.Time) error
}

type CopyLeaseStore interface {
	CreateCopyOperationWithLease(context.Context, CopyOperation, string, int) error
}

const copyOutcomeWriteTimeout = 5 * time.Second

type CopyExecutionResult struct {
	Operation        CopyOperation
	Reconciliation   CopyReconciliation
	CopyAttempted    bool
	RequiresRecovery bool
}

type CopyExecutor struct {
	store    CopyStore
	provider contracts.ObjectCopyProvider
}

func NewCopyExecutor(store CopyStore, provider contracts.ObjectCopyProvider) (*CopyExecutor, error) {
	if store == nil || provider == nil {
		return nil, ErrInvalidCopyOperation
	}
	return &CopyExecutor{store: store, provider: provider}, nil
}

func (executor *CopyExecutor) Execute(ctx context.Context, plan CopyPlan, requester Manifest) (CopyExecutionResult, error) {
	if err := plan.Operation.Validate(); err != nil {
		return CopyExecutionResult{}, err
	}
	if err := plan.Donor.Validate(); err != nil {
		return CopyExecutionResult{}, err
	}
	if _, err := plan.DestinationScope(requester); err != nil {
		return CopyExecutionResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return CopyExecutionResult{}, err
	}
	stored, err := executor.store.GetCopyOperation(ctx, plan.Operation.ManifestID)
	isNew := errors.Is(err, ErrCopyOperationNotFound)
	if err != nil && !isNew {
		return CopyExecutionResult{}, err
	}
	if !isNew {
		if !sameCopyOperation(stored, plan.Operation) {
			return CopyExecutionResult{}, ErrCopyOperationConflict
		}
		if stored.Status == CopyProviderConfirmed {
			return CopyExecutionResult{Operation: stored,
				Reconciliation: CopyReconciliation{Status: CopyReconciliationConfirmed, ProviderResultRef: *stored.ProviderResultRef}}, nil
		}
		if stored.Status == CopyProviderFailed {
			return CopyExecutionResult{Operation: stored, RequiresRecovery: true}, ErrCopyOperationConflict
		}
		result := CopyExecutionResult{Operation: stored,
			RequiresRecovery: stored.Status == CopyReserved || stored.Status == CopyProviderUnknown}
		return executor.reconcile(ctx, plan, requester, stored, result)
	}
	donor, err := executor.provider.InspectObject(ctx, plan.Source)
	if err != nil || !sameEvidence(plan.Donor, donor) {
		return CopyExecutionResult{}, ErrCopyDonorUnavailable
	}
	current, err := executor.provider.ListObjects(ctx, *requester.TargetProviderScope)
	if err != nil {
		return CopyExecutionResult{}, err
	}
	if !sameBaseline(current, plan.Operation.DestinationBefore) {
		return CopyExecutionResult{}, ErrCopyDestinationConflict
	}
	if err := CheckCopyDestinationUnused(plan, requester, current); err != nil {
		return CopyExecutionResult{}, err
	}
	if plan.LeaseOwner != "" {
		leaseStore, ok := executor.store.(CopyLeaseStore)
		if !ok {
			return CopyExecutionResult{}, ErrInvalidCopyOperation
		}
		if err := leaseStore.CreateCopyOperationWithLease(ctx, plan.Operation, plan.LeaseOwner, plan.ExpectedClaim); err != nil {
			return CopyExecutionResult{}, err
		}
	} else if err := executor.store.CreateCopyOperation(ctx, plan.Operation); err != nil {
		return CopyExecutionResult{}, err
	}
	stored = plan.Operation
	result := CopyExecutionResult{Operation: stored, CopyAttempted: true}
	previous := stored.UpdatedAt
	copyErr := executor.provider.CopyObject(ctx, plan.Source, *requester.TargetProviderScope)
	// A provider mutation can complete during shutdown. Give the outcome write
	// its own bounded lifetime so recovery retains evidence of the attempt.
	writeCtx, cancelWrite := context.WithTimeout(context.WithoutCancel(ctx), copyOutcomeWriteTimeout)
	defer cancelWrite()
	if copyErr != nil {
		changedAt := time.Now().UTC()
		stored.UpdatedAt = changedAt
		kind := ProviderErrorInternal
		stored.LastErrorKind = &kind
		if errors.Is(copyErr, contracts.ErrInvalidStorageObject) || errors.Is(copyErr, contracts.ErrUnsupportedCapability) {
			stored.Status = CopyProviderFailed
			updateErr := executor.store.UpdateCopyOperation(writeCtx, stored, previous)
			result.Operation = stored
			result.RequiresRecovery = updateErr != nil
			return result, errors.Join(copyErr, updateErr)
		}
		kind = ProviderErrorTransient
		stored.Status = CopyProviderUnknown
		stored.LastErrorKind = &kind
		updateErr := executor.store.UpdateCopyOperation(writeCtx, stored, previous)
		result.Operation = stored
		result.RequiresRecovery = true
		return result, errors.Join(ErrCopyProviderOutcomeUnknown, updateErr)
	}
	acceptedAt := time.Now().UTC()
	stored.Status = CopyProviderAccepted
	stored.ProviderAcceptedAt = &acceptedAt
	stored.UpdatedAt = acceptedAt
	if err := executor.store.UpdateCopyOperation(writeCtx, stored, previous); err != nil {
		result.Operation = stored
		result.RequiresRecovery = true
		return result, err
	}
	result.Operation = stored
	return executor.reconcile(ctx, plan, requester, stored, result)
}

func (executor *CopyExecutor) reconcile(ctx context.Context, plan CopyPlan, requester Manifest,
	operation CopyOperation, result CopyExecutionResult) (CopyExecutionResult, error) {
	current, err := executor.provider.ListObjects(ctx, *requester.TargetProviderScope)
	result.Operation = operation
	if err != nil {
		return result, err
	}
	reconciliation, err := ReconcileCopyPlan(plan, requester, current)
	result.Reconciliation = reconciliation
	if err != nil {
		result.RequiresRecovery = true
		return result, err
	}
	if reconciliation.Status == CopyReconciliationPending {
		return result, nil
	}
	candidate, err := executor.provider.InspectObject(ctx, contracts.StorageObjectReference{
		Scope: *requester.TargetProviderScope, Path: reconciliation.ProviderResultRef,
	})
	if err != nil || candidate.Reference.Scope != *requester.TargetProviderScope || !sameCopiedEvidence(plan.Donor, candidate) {
		result.RequiresRecovery = true
		if err != nil {
			return result, err
		}
		return result, ErrCopyIdentityMismatch
	}
	previous := operation.UpdatedAt
	operation.Status = CopyProviderConfirmed
	operation.ProviderResultRef = &reconciliation.ProviderResultRef
	operation.UpdatedAt = time.Now().UTC()
	if err := executor.store.UpdateCopyOperation(ctx, operation, previous); err != nil {
		result.RequiresRecovery = true
		return result, err
	}
	result.Operation = operation
	result.RequiresRecovery = false
	return result, nil
}

func sameEvidence(expected ResultObjectEvidence, actual contracts.StorageObject) bool {
	return expected.ProviderParentScope == actual.Reference.Scope && expected.ProviderObjectRef == actual.Reference.Path &&
		expected.ResultName == actual.Name && expected.SizeBytes == actual.SizeBytes &&
		expected.HashAlgorithm == actual.HashAlgorithm && strings.EqualFold(expected.ContentHash, actual.ContentHash)
}

func sameCopiedEvidence(expected ResultObjectEvidence, actual contracts.StorageObject) bool {
	return expected.ResultName == actual.Name && expected.SizeBytes == actual.SizeBytes &&
		expected.HashAlgorithm == actual.HashAlgorithm && strings.EqualFold(expected.ContentHash, actual.ContentHash)
}

func sameBaseline(current []contracts.StorageObject, baseline []string) bool {
	ids := make([]string, 0, len(current))
	for _, object := range current {
		if contracts.ValidateObjectEvidence(object) != nil {
			return false
		}
		ids = append(ids, object.Reference.Path)
	}
	sort.Strings(ids)
	expected := append([]string(nil), baseline...)
	sort.Strings(expected)
	if len(ids) != len(expected) {
		return false
	}
	for index := range ids {
		if ids[index] != expected[index] {
			return false
		}
	}
	return true
}

func sameCopyOperation(left, right CopyOperation) bool {
	if left.ID != right.ID || left.ManifestID != right.ManifestID || left.JobID != right.JobID ||
		left.DonorManifestID != right.DonorManifestID || left.ProviderConnectionID != right.ProviderConnectionID ||
		!bytes.Equal(left.RequestDigest, right.RequestDigest) || len(left.DestinationBefore) != len(right.DestinationBefore) {
		return false
	}
	for index := range left.DestinationBefore {
		if left.DestinationBefore[index] != right.DestinationBefore[index] {
			return false
		}
	}
	return true
}
