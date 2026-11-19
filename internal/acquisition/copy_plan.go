package acquisition

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nathanxiangang-web/panta/internal/jobs"
	"github.com/nathanxiangang-web/panta/internal/providers/contracts"
)

var (
	ErrCopyDonorUnavailable    = errors.New("acquisition copy donor is not eligible")
	ErrCopyDestinationConflict = errors.New("acquisition copy destination is not empty")
	ErrCopyResultAmbiguous     = errors.New("acquisition copy result is ambiguous")
	ErrCopyIdentityMismatch    = errors.New("acquisition copy identity mismatch")
)

type CopyPlan struct {
	Operation     CopyOperation
	Donor         ResultObjectEvidence
	Source        contracts.StorageObjectReference
	LeaseOwner    string
	ExpectedClaim int
}

type CopyReconciliationStatus string

const (
	CopyReconciliationPending   CopyReconciliationStatus = "PENDING"
	CopyReconciliationConfirmed CopyReconciliationStatus = "CONFIRMED"
)

type CopyReconciliation struct {
	Status            CopyReconciliationStatus
	ProviderResultRef string
}

func NewCopyPlan(requester Manifest, donor Manifest, donorJob jobs.Job, donorTask ProviderTask,
	evidence ResultObjectEvidence, operationID CopyOperationID, destinationBefore []string, now time.Time) (CopyPlan, error) {
	if requester.State != StateActive || requester.JobID == nil || requester.UserSpaceID == nil ||
		requester.TargetProviderConnectionID == nil || requester.TargetProviderScope == nil ||
		donor.State != StateReady || donor.ResultCopyID == nil || donor.JobID == nil || donorJob.ID != *donor.JobID ||
		donorJob.Type != "ACQUISITION" || donorJob.State != jobs.StateSucceeded || donorTask.ManifestID != donor.ID ||
		donorTask.JobID != donorJob.ID || donorTask.ProviderID != "115" || !donorTask.ReferenceKnown() ||
		requester.SourceType != donor.SourceType || requester.SourceRef != donor.SourceRef ||
		donor.TargetProviderConnectionID == nil || donor.TargetProviderScope == nil ||
		*donor.TargetProviderConnectionID != *requester.TargetProviderConnectionID ||
		*donor.TargetProviderScope != evidence.ProviderParentScope ||
		evidence.ManifestID != donor.ID || evidence.ProviderConnectionID != *requester.TargetProviderConnectionID ||
		requester.ID == donor.ID || *requester.UserSpaceID == "" || donor.UserSpaceID == nil ||
		*requester.UserSpaceID == *donor.UserSpaceID || evidence.ProviderParentScope == *requester.TargetProviderScope ||
		evidence.Validate() != nil || ValidateProviderTask(donorTask) != nil {
		return CopyPlan{}, ErrCopyDonorUnavailable
	}
	if donor.ResultName != nil && *donor.ResultName != evidence.ResultName ||
		donor.ResultName == nil && donor.ExpectedName != nil && *donor.ExpectedName != evidence.ResultName {
		return CopyPlan{}, ErrCopyDonorUnavailable
	}
	baseline := make([]string, len(destinationBefore))
	copy(baseline, destinationBefore)
	sort.Strings(baseline)
	operation := CopyOperation{
		ID: operationID, ManifestID: requester.ID, JobID: *requester.JobID, DonorManifestID: donor.ID,
		ProviderConnectionID: *requester.TargetProviderConnectionID,
		RequestDigest:        copyRequestDigest(requester, donorTask, evidence, baseline),
		DestinationBefore:    baseline, Status: CopyReserved, CreatedAt: now.UTC(), UpdatedAt: now.UTC(),
	}
	if err := operation.Validate(); err != nil {
		return CopyPlan{}, err
	}
	plan := CopyPlan{Operation: operation, Donor: evidence,
		Source: contracts.StorageObjectReference{Scope: evidence.ProviderParentScope, Path: evidence.ProviderObjectRef}}
	return plan, nil
}

func (plan CopyPlan) DestinationScope(requester Manifest) (string, error) {
	if requester.ID != plan.Operation.ManifestID || requester.TargetProviderScope == nil {
		return "", ErrCopyIdentityMismatch
	}
	return *requester.TargetProviderScope, nil
}

func CheckCopyDestinationUnused(plan CopyPlan, requester Manifest, current []contracts.StorageObject) error {
	if _, err := plan.DestinationScope(requester); err != nil {
		return err
	}
	for _, object := range current {
		if object.Name == plan.Donor.ResultName {
			return ErrCopyDestinationConflict
		}
	}
	return nil
}

func ReconcileCopyPlan(plan CopyPlan, requester Manifest, current []contracts.StorageObject) (CopyReconciliation, error) {
	destination, err := plan.DestinationScope(requester)
	if err != nil {
		return CopyReconciliation{}, err
	}
	before := make(map[string]struct{}, len(plan.Operation.DestinationBefore))
	for _, objectRef := range plan.Operation.DestinationBefore {
		before[objectRef] = struct{}{}
	}
	seen := make(map[string]struct{}, len(current))
	candidates := make([]contracts.StorageObject, 0, 1)
	for _, object := range current {
		if contracts.ValidateObjectEvidence(object) != nil || object.Reference.Scope != destination {
			return CopyReconciliation{}, ErrCopyIdentityMismatch
		}
		if _, exists := seen[object.Reference.Path]; exists {
			return CopyReconciliation{}, ErrCopyResultAmbiguous
		}
		seen[object.Reference.Path] = struct{}{}
		if _, exists := before[object.Reference.Path]; exists {
			continue
		}
		if object.Name == plan.Donor.ResultName && object.SizeBytes == plan.Donor.SizeBytes &&
			object.HashAlgorithm == plan.Donor.HashAlgorithm && strings.EqualFold(object.ContentHash, plan.Donor.ContentHash) {
			candidates = append(candidates, object)
		}
	}
	if len(candidates) == 0 {
		return CopyReconciliation{Status: CopyReconciliationPending}, nil
	}
	if len(candidates) != 1 {
		return CopyReconciliation{}, ErrCopyResultAmbiguous
	}
	resultRef := candidates[0].Reference.Path
	if plan.Operation.ProviderResultRef != nil && *plan.Operation.ProviderResultRef != resultRef {
		return CopyReconciliation{}, ErrCopyIdentityMismatch
	}
	return CopyReconciliation{Status: CopyReconciliationConfirmed, ProviderResultRef: resultRef}, nil
}

func copyRequestDigest(requester Manifest, donorTask ProviderTask, evidence ResultObjectEvidence, baseline []string) []byte {
	digest := sha256.New()
	for _, value := range []string{
		"panta-native-copy-v1", string(requester.ID), string(*requester.JobID), string(donorTask.ManifestID),
		donorTask.ProviderTaskRef, string(evidence.ProviderConnectionID), evidence.ProviderObjectRef,
		evidence.ProviderParentScope, evidence.ResultName, strconv.FormatInt(evidence.SizeBytes, 10),
		evidence.HashAlgorithm, strings.ToLower(evidence.ContentHash), *requester.TargetProviderScope, requester.TargetPath,
	} {
		writeCopyDigestField(digest, value)
	}
	for _, objectRef := range baseline {
		writeCopyDigestField(digest, objectRef)
	}
	return digest.Sum(nil)
}

func writeCopyDigestField(digest interface{ Write([]byte) (int, error) }, value string) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = digest.Write(length[:])
	_, _ = digest.Write([]byte(value))
}
