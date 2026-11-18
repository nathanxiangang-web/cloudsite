package acquisition

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/nathanxiangang-web/panta/internal/jobs"
	"github.com/nathanxiangang-web/panta/internal/providers/contracts"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

type CopyDonorCandidate struct {
	Manifest Manifest
	Job      jobs.Job
	Task     ProviderTask
	Evidence ResultObjectEvidence
}

type CopyDonorStore interface {
	FindCopyDonor(context.Context, string, string, storage.ConnectionID, ManifestID) (CopyDonorCandidate, error)
}

type CopyCoordinator struct {
	donors CopyDonorStore
	store  CopyStore
	now    func() time.Time
}

func NewCopyCoordinator(donors CopyDonorStore, store CopyStore, now func() time.Time) (*CopyCoordinator, error) {
	if donors == nil || store == nil || now == nil {
		return nil, ErrInvalidCopyOperation
	}
	return &CopyCoordinator{donors: donors, store: store, now: now}, nil
}

func (coordinator *CopyCoordinator) ExecuteCopy(ctx context.Context, requester Manifest, requesterJob jobs.Job,
	provider contracts.ObjectCopyProvider, owner string, expectedClaim int) (StepResult, bool, error) {
	candidate, err := coordinator.donors.FindCopyDonor(ctx, requester.SourceType, requester.SourceRef,
		*requester.TargetProviderConnectionID, requester.ID)
	if errors.Is(err, ErrCopyDonorUnavailable) {
		return StepResult{}, false, nil
	}
	if err != nil {
		return StepResult{}, true, err
	}
	if requesterJob.ID != *requester.JobID || requesterJob.Type != JobTypeAcquisition || requesterJob.State != jobs.StateRunning {
		return StepResult{}, true, ErrExecutionJobMismatch
	}
	baseline, err := coordinator.copyBaseline(ctx, requester, provider)
	if err != nil {
		return StepResult{}, true, err
	}
	operationID := copyOperationIDForManifest(requester.ID)
	plan, err := NewCopyPlan(requester, candidate.Manifest, candidate.Job, candidate.Task, candidate.Evidence,
		operationID, baseline, coordinator.now().UTC())
	if err != nil {
		return StepResult{}, true, err
	}
	plan.LeaseOwner = owner
	plan.ExpectedClaim = expectedClaim
	executor, err := NewCopyExecutor(coordinator.store, provider)
	if err != nil {
		return StepResult{}, true, err
	}
	result, err := executor.Execute(ctx, plan, requester)
	if err != nil || result.RequiresRecovery {
		if err == nil {
			err = ErrCopyProviderOutcomeUnknown
		}
		return StepResult{}, true, fmt.Errorf("%w: %w", ErrExecutionSideEffectUncertain, err)
	}
	if result.Reconciliation.Status == CopyReconciliationPending {
		return StepResult{Outcome: OutcomeProviderInProgress}, true, nil
	}
	if result.Operation.Status != CopyProviderConfirmed {
		return StepResult{}, true, ErrCopyProviderOutcomeUnknown
	}
	name := plan.Donor.ResultName
	return StepResult{Outcome: OutcomeProviderSucceeded, ResultName: &name}, true, nil
}
func (coordinator *CopyCoordinator) copyBaseline(ctx context.Context, requester Manifest, provider contracts.ObjectCopyProvider) ([]string, error) {
	stored, err := coordinator.store.GetCopyOperation(ctx, requester.ID)
	if err == nil {
		return append([]string(nil), stored.DestinationBefore...), nil
	}
	if !errors.Is(err, ErrCopyOperationNotFound) {
		return nil, err
	}
	current, err := provider.ListObjects(ctx, *requester.TargetProviderScope)
	if err != nil {
		return nil, err
	}
	baseline := make([]string, 0, len(current))
	for _, object := range current {
		baseline = append(baseline, object.Reference.Path)
	}
	sort.Strings(baseline)
	return baseline, nil
}

func copyOperationIDForManifest(manifestID ManifestID) CopyOperationID {
	sum := sha256.Sum256([]byte("panta-copy:" + string(manifestID)))
	sum[6] = (sum[6] & 0x0f) | 0x50
	sum[8] = (sum[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(sum[:16])
	return CopyOperationID(encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32])
}
