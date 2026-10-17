package product

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nathanxiangang-web/panta/internal/acquisition"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/jobs"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

var (
	ErrOperationConflict  = errors.New("product operation conflict")
	ErrOperationLimit     = errors.New("product operation limit reached")
	ErrConcurrencyLimit   = errors.New("product concurrency limit reached")
	ErrAdmissionUncertain = errors.New("product admission uncertain")
)

const (
	DefaultOperationLimit   = 100
	DefaultConcurrencyLimit = 2
	AdmissionMaxAttempts    = 5
	AdmissionTimeout        = 30 * time.Second
)

var operationKeyPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

type AdmissionRequest struct {
	OperationKey string            `json:"operation_key"`
	Source       string            `json:"source"`
	BindingID    storage.BindingID `json:"binding_id"`
	TargetPath   string            `json:"target_path"`
	ExpectedName *string           `json:"expected_name,omitempty"`
}

func (AdmissionRequest) String() string { return "product admission (redacted)" }

type Reservation struct {
	ManifestID  acquisition.ManifestID
	JobID       jobs.JobID
	MaxAttempts int
	Created     bool
	// Immutable private execution evidence returned by the durable reservation.
	Source string                   `json:"-"`
	Policy acquisition.SourcePolicy `json:"-"`
}

func (Reservation) String() string { return "product reservation (redacted)" }

type AdmissionDTO struct {
	ManifestID acquisition.ManifestID       `json:"manifest_id"`
	JobID      jobs.JobID                   `json:"job_id"`
	Status     acquisition.SubmissionStatus `json:"status"`
}
type UsageDTO struct {
	Unit             string `json:"unit"`
	Price            string `json:"price"`
	OperationLimit   int    `json:"operation_limit"`
	Used             int    `json:"used"`
	Remaining        int    `json:"remaining"`
	ConcurrencyLimit int    `json:"concurrency_limit"`
	InFlight         int    `json:"in_flight"`
}
type AdmissionStore interface {
	Reserve(context.Context, auth.Principal, AdmissionRequest, Reservation) (Reservation, error)
	Usage(context.Context, auth.Principal) (UsageDTO, error)
}
type AcquisitionSubmitter interface {
	Submit(context.Context, acquisition.SubmitRequest) (acquisition.SubmitResult, error)
}
type Admissions interface {
	Admit(context.Context, auth.Principal, AdmissionRequest) (AdmissionDTO, error)
	Usage(context.Context, auth.Principal) (UsageDTO, error)
}
type AdmissionService struct {
	store     AdmissionStore
	submitter AcquisitionSubmitter
	allowed   map[storage.BindingID]bool
}

func NewAdmissionService(store AdmissionStore, submitter AcquisitionSubmitter, allowed []storage.BindingID) (*AdmissionService, error) {
	if missing(store) || missing(submitter) || len(allowed) == 0 {
		return nil, ErrUnavailable
	}
	s := &AdmissionService{store: store, submitter: submitter, allowed: make(map[storage.BindingID]bool)}
	for _, id := range allowed {
		if !ValidID(string(id)) || s.allowed[id] {
			return nil, ErrInvalid
		}
		s.allowed[id] = true
	}
	return s, nil
}

// ValidateAdmissionRequest checks raw intent only. Policy execution happens on
// NEW durable reservations, never while replaying an accepted operation.
func ValidateAdmissionRequest(r AdmissionRequest) error {
	if !operationKeyPattern.MatchString(r.OperationKey) || !ValidID(string(r.BindingID)) {
		return ErrInvalid
	}
	if r.Source == "" || len(r.Source) > acquisition.MaxSourceInputLength || !utf8.ValidString(r.Source) || strings.TrimSpace(r.Source) != r.Source || strings.ContainsAny(r.Source, "\x00\r\n") {
		return ErrInvalid
	}
	if _, err := acquisition.NormalizeTargetPath(r.TargetPath); err != nil {
		return ErrInvalid
	}
	if r.ExpectedName != nil && (len(*r.ExpectedName) > acquisition.MaxExpectedNameLength || strings.TrimSpace(*r.ExpectedName) == "" || !utf8.ValidString(*r.ExpectedName) || strings.ContainsRune(*r.ExpectedName, 0)) {
		return ErrInvalid
	}
	return nil
}

// AdmissionDigest uses unambiguous length frames, including optional presence.
// It is internal durable evidence and is never a client DTO.
func AdmissionDigest(r AdmissionRequest) ([32]byte, error) {
	if err := ValidateAdmissionRequest(r); err != nil {
		return [32]byte{}, err
	}
	path, _ := acquisition.NormalizeTargetPath(r.TargetPath)
	h := sha256.New()
	values := []string{"panta-admission-v2", r.Source, string(r.BindingID), path, "absent"}
	if r.ExpectedName != nil {
		values[4] = "present"
		values = append(values, *r.ExpectedName)
	}
	for _, value := range values {
		var size [8]byte
		binary.BigEndian.PutUint64(size[:], uint64(len(value)))
		h.Write(size[:])
		h.Write([]byte(value))
	}
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum, nil
}
func (s *AdmissionService) Admit(ctx context.Context, p auth.Principal, r AdmissionRequest) (AdmissionDTO, error) {
	if s == nil {
		return AdmissionDTO{}, ErrUnavailable
	}
	if !p.Valid() {
		return AdmissionDTO{}, auth.ErrUnauthorized
	}
	if err := ValidateAdmissionRequest(r); err != nil {
		return AdmissionDTO{}, err
	}
	if !s.allowed[r.BindingID] {
		return AdmissionDTO{}, ErrNotFound
	}
	if err := ctx.Err(); err != nil {
		return AdmissionDTO{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, AdmissionTimeout)
	defer cancel()
	mid, err := auth.NewUserID()
	if err != nil {
		return AdmissionDTO{}, ErrUnavailable
	}
	jid, err := auth.NewUserID()
	if err != nil {
		return AdmissionDTO{}, ErrUnavailable
	}
	reserved, err := s.store.Reserve(ctx, p, r, Reservation{ManifestID: acquisition.ManifestID(mid), JobID: jobs.JobID(jid), MaxAttempts: AdmissionMaxAttempts})
	if err != nil {
		return AdmissionDTO{}, admissionError(err)
	}
	if !ValidID(string(reserved.ManifestID)) || !ValidID(string(reserved.JobID)) || reserved.MaxAttempts != AdmissionMaxAttempts || !ValidAdmissionExecution(reserved.Source, reserved.Policy) {
		return AdmissionDTO{}, ErrAdmissionUncertain
	}
	result := AdmissionDTO{ManifestID: reserved.ManifestID, JobID: reserved.JobID}
	if err := ctx.Err(); err != nil {
		return AdmissionDTO{}, err
	}
	owner := acquisition.UserID(p.UserID)
	activated, err := s.submitter.Submit(ctx, acquisition.SubmitRequest{ManifestID: reserved.ManifestID, JobID: reserved.JobID, MaxAttempts: reserved.MaxAttempts, Source: reserved.Source, UserID: &owner, ExpectedName: r.ExpectedName, TargetStorageBindingID: r.BindingID, TargetPath: r.TargetPath})
	if activated.ManifestID != reserved.ManifestID || activated.JobID != reserved.JobID {
		return AdmissionDTO{}, ErrAdmissionUncertain
	}
	if errors.Is(err, acquisition.ErrActivationPending) && activated.Status == acquisition.SubmissionPendingActivation {
		result.Status = acquisition.SubmissionPendingActivation
		return result, nil
	}
	if err != nil {
		return AdmissionDTO{}, admissionError(err)
	}
	if activated.Status != acquisition.SubmissionCreated && activated.Status != acquisition.SubmissionReplay {
		return AdmissionDTO{}, ErrAdmissionUncertain
	}
	result.Status = acquisition.SubmissionReplay
	if reserved.Created {
		result.Status = acquisition.SubmissionCreated
	}
	return result, nil
}

// Validate stored execution without re-running a potentially newer preparation
// policy. A future policy must be explicitly supported before admitting it.
func ValidAdmissionExecution(source string, policy acquisition.SourcePolicy) bool {
	if policy != acquisition.SourceExactV1 && policy != acquisition.MagnetBTIHNameV1 {
		return false
	}
	_, err := acquisition.ResolveSource(source)
	return err == nil
}
func (s *AdmissionService) Usage(ctx context.Context, p auth.Principal) (UsageDTO, error) {
	if s == nil {
		return UsageDTO{}, ErrUnavailable
	}
	if !p.Valid() {
		return UsageDTO{}, auth.ErrUnauthorized
	}
	ctx, cancel := context.WithTimeout(ctx, ReadTimeout)
	defer cancel()
	result, err := s.store.Usage(ctx, p)
	if err != nil {
		return UsageDTO{}, admissionError(err)
	}
	if result.Unit != "OPERATION" || result.Price != "ZERO" || result.OperationLimit < 0 || result.Used < 0 || result.Remaining != max(0, result.OperationLimit-result.Used) || result.ConcurrencyLimit < 1 || result.InFlight < 0 || result.InFlight > result.Used {
		return UsageDTO{}, ErrUnavailable
	}
	return result, nil
}
func admissionError(err error) error {
	for _, known := range []error{context.Canceled, context.DeadlineExceeded, auth.ErrUnauthorized, ErrInvalid, ErrNotFound, ErrOperationConflict, ErrOperationLimit, ErrConcurrencyLimit, ErrAdmissionUncertain} {
		if errors.Is(err, known) {
			return known
		}
	}
	if errors.Is(err, acquisition.ErrSubmissionConflict) || errors.Is(err, acquisition.ErrSubmissionUncertain) {
		return ErrAdmissionUncertain
	}
	return ErrUnavailable
}
