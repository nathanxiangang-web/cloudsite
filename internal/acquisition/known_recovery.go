package acquisition

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/nathanxiangang-web/panta/internal/jobs"
	"github.com/nathanxiangang-web/panta/internal/providers/contracts"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

var ErrKnownRecoveryConflict = errors.New("known acquisition recovery scope or state conflict")

// KnownPollScope is immutable operator evidence, not permission to start a
// provider task. Digests bind exact UTF-8 bytes without publishing the values.
type KnownPollScope struct {
	ManifestID      ManifestID           `json:"manifest_id"`
	JobID           jobs.JobID           `json:"job_id"`
	BindingID       storage.BindingID    `json:"binding_id"`
	ConnectionID    storage.ConnectionID `json:"connection_id"`
	ProviderID      contracts.ProviderID `json:"provider_id"`
	RootID          string               `json:"root_id"`
	ReferenceSHA256 string               `json:"reference_sha256"`
	SourceSHA256    string               `json:"source_sha256"`
}

func (scope KnownPollScope) Valid() bool {
	return scope.ManifestID != "" && scope.JobID != "" && scope.BindingID != "" &&
		scope.ConnectionID != "" && scope.ProviderID.Valid() && scope.RootID != "" &&
		validDigest(scope.ReferenceSHA256) && validDigest(scope.SourceSHA256)
}

func validDigest(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && hex.EncodeToString(decoded) == value
}

func EvidenceSHA256(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func (scope KnownPollScope) Matches(manifest Manifest, task ProviderTask) bool {
	return scope.Valid() && manifest.ID == scope.ManifestID && manifest.JobID != nil &&
		*manifest.JobID == scope.JobID && manifest.TargetStorageBindingID == scope.BindingID &&
		EvidenceSHA256(manifest.SourceRef) == scope.SourceSHA256 && task.ManifestID == scope.ManifestID &&
		task.JobID == scope.JobID && task.ProviderID == scope.ProviderID && task.ReferenceKnown() &&
		ValidProviderTaskRef(task.ProviderTaskRef) && EvidenceSHA256(task.ProviderTaskRef) == scope.ReferenceSHA256
}

// The two timestamps are CAS tokens from a read-only snapshot. Resume preserves
// the failure budget and generation; only a later ordinary claim increments it.
type KnownRecoveryRequest struct {
	Scope             KnownPollScope `json:"scope"`
	ExpectedClaim     int            `json:"expected_claim"`
	ManifestUpdatedAt time.Time      `json:"manifest_updated_at"`
	JobUpdatedAt      time.Time      `json:"job_updated_at"`
	RetryDelay        time.Duration  `json:"retry_delay_ns"`
}

func (request KnownRecoveryRequest) Valid() bool {
	return request.Scope.Valid() && request.ExpectedClaim > 0 && !request.ManifestUpdatedAt.IsZero() &&
		!request.JobUpdatedAt.IsZero() && request.RetryDelay >= time.Second && request.RetryDelay <= 5*time.Minute
}

// WithKnownPollOnly makes the missing-reference/start path unreachable, even if
// the durable row disappears after an operator resumes the pair.
func WithKnownPollOnly(scope KnownPollScope) ExecutionOption {
	return func(service *ExecutionStepService) error {
		if !scope.Valid() {
			return ErrKnownRecoveryConflict
		}
		service.pollScope = &scope
		return nil
	}
}
