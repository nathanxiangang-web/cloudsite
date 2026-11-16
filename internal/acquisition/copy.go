package acquisition

import (
	"errors"
	"time"

	"github.com/nathanxiangang-web/panta/internal/jobs"
	"github.com/nathanxiangang-web/panta/internal/providers/contracts"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

type CopyOperationID string
type CopyOperationStatus string
type ProviderErrorKind string

const (
	CopyReserved          CopyOperationStatus = "COPY_RESERVED"
	CopyProviderAccepted  CopyOperationStatus = "PROVIDER_ACCEPTED"
	CopyProviderConfirmed CopyOperationStatus = "PROVIDER_CONFIRMED"
	CopyProviderUnknown   CopyOperationStatus = "PROVIDER_UNKNOWN"
	CopyProviderFailed    CopyOperationStatus = "FAILED"
)

const (
	ProviderErrorAuthOrPermission ProviderErrorKind = "AUTH_OR_PERMISSION"
	ProviderErrorThrottled        ProviderErrorKind = "THROTTLED"
	ProviderErrorTransient        ProviderErrorKind = "TRANSIENT_PROVIDER"
	ProviderErrorScopeTooLarge    ProviderErrorKind = "SCOPE_TOO_LARGE"
	ProviderErrorInvalidScope     ProviderErrorKind = "INVALID_SCOPE"
	ProviderErrorConfigInvalid    ProviderErrorKind = "CONFIG_INVALID"
	ProviderErrorRootInactive     ProviderErrorKind = "ROOT_INACTIVE"
	ProviderErrorInternal         ProviderErrorKind = "INTERNAL"
)

var (
	ErrInvalidResultEvidence      = errors.New("invalid acquisition result object evidence")
	ErrInvalidCopyOperation       = errors.New("invalid acquisition copy operation")
	ErrCopyOperationNotFound      = errors.New("acquisition copy operation not found")
	ErrCopyOperationConflict      = errors.New("acquisition copy operation conflict")
	ErrCopyProviderOutcomeUnknown = errors.New("acquisition copy provider outcome is unknown")
)

type ResultObjectEvidence struct {
	ManifestID           ManifestID
	ProviderConnectionID storage.ConnectionID
	ProviderObjectRef    string
	ProviderParentScope  string
	ResultName           string
	SizeBytes            int64
	HashAlgorithm        string
	ContentHash          string
	ObservedAt           time.Time
}

type CopyOperation struct {
	ID                   CopyOperationID
	ManifestID           ManifestID
	JobID                jobs.JobID
	DonorManifestID      ManifestID
	ProviderConnectionID storage.ConnectionID
	RequestDigest        []byte
	DestinationBefore    []string
	Status               CopyOperationStatus
	ProviderResultRef    *string
	LastErrorKind        *ProviderErrorKind
	ProviderAcceptedAt   *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

func (status CopyOperationStatus) Valid() bool {
	switch status {
	case CopyReserved, CopyProviderAccepted, CopyProviderConfirmed, CopyProviderUnknown, CopyProviderFailed:
		return true
	default:
		return false
	}
}

func (kind ProviderErrorKind) Valid() bool {
	switch kind {
	case ProviderErrorAuthOrPermission, ProviderErrorThrottled, ProviderErrorTransient,
		ProviderErrorScopeTooLarge, ProviderErrorInvalidScope, ProviderErrorConfigInvalid,
		ProviderErrorRootInactive, ProviderErrorInternal:
		return true
	default:
		return false
	}
}

func (evidence ResultObjectEvidence) Validate() error {
	object := contracts.StorageObject{
		Reference: contracts.StorageObjectReference{Scope: evidence.ProviderParentScope, Path: evidence.ProviderObjectRef},
		Name:      evidence.ResultName, SizeBytes: evidence.SizeBytes,
		HashAlgorithm: evidence.HashAlgorithm, ContentHash: evidence.ContentHash,
	}
	if evidence.ManifestID == "" || evidence.ProviderConnectionID == "" || evidence.ObservedAt.IsZero() ||
		contracts.ValidateObjectEvidence(object) != nil {
		return ErrInvalidResultEvidence
	}
	return nil
}

func (operation CopyOperation) Validate() error {
	if operation.ID == "" || operation.ManifestID == "" || operation.JobID == "" ||
		operation.DonorManifestID == "" || operation.ProviderConnectionID == "" ||
		len(operation.RequestDigest) != 32 || operation.DestinationBefore == nil ||
		!operation.Status.Valid() || operation.CreatedAt.IsZero() || operation.UpdatedAt.IsZero() ||
		operation.UpdatedAt.Before(operation.CreatedAt) {
		return ErrInvalidCopyOperation
	}
	for _, objectRef := range operation.DestinationBefore {
		if contracts.ValidateObjectReference(contracts.StorageObjectReference{Scope: "scope", Path: objectRef}) != nil {
			return ErrInvalidCopyOperation
		}
	}
	if operation.LastErrorKind != nil && !operation.LastErrorKind.Valid() {
		return ErrInvalidCopyOperation
	}
	if operation.ProviderResultRef != nil && contracts.ValidateObjectReference(contracts.StorageObjectReference{Scope: "scope", Path: *operation.ProviderResultRef}) != nil {
		return ErrInvalidCopyOperation
	}
	if operation.Status == CopyProviderConfirmed && operation.ProviderResultRef == nil {
		return ErrInvalidCopyOperation
	}
	return nil
}
