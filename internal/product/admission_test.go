package product

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/nathanxiangang-web/panta/internal/acquisition"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/jobs"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

const admissionBinding storage.BindingID = "98000000-0000-4000-8000-000000000001"

func TestDefaultQuotaPolicyIs100WithUnchangedConcurrencyFence(t *testing.T) {
	if DefaultOperationLimit != 100 || DefaultConcurrencyLimit != 2 {
		t.Fatal("quota policy")
	}
}

func admissionPrincipal() auth.Principal {
	return auth.Principal{UserID: "98000000-0000-4000-8000-000000000002", Login: "test-user", Role: auth.RoleUser}
}

type admissionStoreStub struct {
	calls   int
	failure error
	created bool
	owner   auth.UserID
}

func (s *admissionStoreStub) Reserve(_ context.Context, p auth.Principal, r AdmissionRequest, _ Reservation) (Reservation, error) {
	s.calls++
	s.owner = p.UserID
	prepared, err := acquisition.PrepareSource(r.Source, acquisition.MagnetBTIHNameV1)
	if err != nil {
		return Reservation{}, ErrInvalid
	}
	return Reservation{ManifestID: "98000000-0000-4000-8000-000000000003", JobID: "98000000-0000-4000-8000-000000000004", MaxAttempts: AdmissionMaxAttempts, Created: s.created, Source: prepared.Reference.Value, Policy: prepared.Policy}, s.failure
}
func (s *admissionStoreStub) Usage(context.Context, auth.Principal) (UsageDTO, error) {
	return UsageDTO{Unit: "OPERATION", Price: "ZERO", OperationLimit: 10, Remaining: 10, ConcurrencyLimit: 2}, s.failure
}

type submitAdmissionStub struct {
	calls    int
	received acquisition.SubmitRequest
	pending  bool
	failure  error
}

func (s *submitAdmissionStub) Submit(_ context.Context, r acquisition.SubmitRequest) (acquisition.SubmitResult, error) {
	s.calls++
	s.received = r
	status := acquisition.SubmissionReplay
	err := s.failure
	if s.pending {
		status = acquisition.SubmissionPendingActivation
		err = acquisition.ErrActivationPending
	}
	return acquisition.SubmitResult{ManifestID: r.ManifestID, JobID: r.JobID, Status: status}, err
}
func TestAdmissionIdentityExactSourceReplayAndRedaction(t *testing.T) {
	store, submit := &admissionStoreStub{created: true}, &submitAdmissionStub{}
	allowed := []storage.BindingID{admissionBinding}
	s, err := NewAdmissionService(store, submit, allowed)
	if err != nil {
		t.Fatal(err)
	}
	allowed[0] = "other"
	input := AdmissionRequest{OperationKey: "key-1", Source: "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567&dn=private%20Name&tr=one&tr=two", BindingID: admissionBinding, TargetPath: "/"}
	prepared, _ := acquisition.PrepareSource(input.Source, acquisition.MagnetBTIHNameV1)
	got, err := s.Admit(context.Background(), admissionPrincipal(), input)
	if err != nil || got.Status != acquisition.SubmissionCreated || submit.received.Source != prepared.Reference.Value || submit.received.UserID == nil || string(*submit.received.UserID) != string(admissionPrincipal().UserID) || submit.received.MaxAttempts != 5 || submit.received.AssetID != nil {
		t.Fatal(got, err)
	}
	raw, _ := json.Marshal(got)
	if strings.Contains(string(raw), "private") || strings.Contains(string(raw), "source") || strings.Contains(string(raw), "reference") {
		t.Fatal("secret DTO")
	}
	store.created = false
	replay, err := s.Admit(context.Background(), admissionPrincipal(), input)
	if err != nil || replay.Status != acquisition.SubmissionReplay || replay.ManifestID != got.ManifestID || replay.JobID != jobs.JobID(got.JobID) {
		t.Fatal(replay, err)
	}
	submit.pending = true
	pending, err := s.Admit(context.Background(), admissionPrincipal(), input)
	if err != nil || pending.Status != acquisition.SubmissionPendingActivation {
		t.Fatal(pending, err)
	}
	before := store.calls
	if _, err = s.Admit(context.Background(), auth.Principal{}, input); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatal(err)
	}
	outside := input
	outside.BindingID = "98000000-0000-4000-8000-000000000099"
	if _, err = s.Admit(context.Background(), admissionPrincipal(), outside); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = s.Admit(canceled, admissionPrincipal(), input); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if store.calls != before {
		t.Fatal("unauthorized persistence")
	}
	submit.pending = false
	submit.failure = errors.New("private source token")
	if _, err = s.Admit(context.Background(), admissionPrincipal(), input); !errors.Is(err, ErrUnavailable) || strings.Contains(err.Error(), "token") {
		t.Fatal(err)
	}
}

func TestAdmissionPrivateEvidenceAndLimits(t *testing.T) {
	private := "https://example.invalid/SECRET?signature=keep"
	reservation := Reservation{Source: private, Policy: acquisition.SourceExactV1}
	raw, _ := json.Marshal(reservation)
	if strings.Contains(string(raw), "SECRET") || strings.Contains(reservation.String(), "SECRET") || strings.Contains((AdmissionRequest{Source: private}).String(), "SECRET") {
		t.Fatal("private evidence exposed")
	}
	input := AdmissionRequest{OperationKey: "k", Source: strings.Repeat("a", acquisition.MaxSourceInputLength+1), BindingID: admissionBinding, TargetPath: "/"}
	if ValidateAdmissionRequest(input) == nil {
		t.Fatal("oversized source accepted")
	}
	store, submit := &admissionStoreStub{}, &submitAdmissionStub{}
	s, _ := NewAdmissionService(store, submit, []storage.BindingID{admissionBinding})
	input.Source = private
	for _, known := range []error{ErrOperationConflict, ErrOperationLimit, ErrConcurrencyLimit, ErrAdmissionUncertain, auth.ErrUnauthorized} {
		store.failure = known
		if _, err := s.Admit(context.Background(), admissionPrincipal(), input); !errors.Is(err, known) {
			t.Fatal("error category lost")
		}
	}
	if submit.calls != 0 {
		t.Fatal("failed reservation activated")
	}
}
func TestAdmissionDigestAndValidation(t *testing.T) {
	input := AdmissionRequest{OperationKey: "k", Source: "HTTPS://example.invalid/a?x=%2f&x=%2F", BindingID: admissionBinding, TargetPath: "/a//b/"}
	first, err := AdmissionDigest(input)
	if err != nil {
		t.Fatal(err)
	}
	same := input
	same.TargetPath = "/a/b"
	same.OperationKey = "another"
	second, err := AdmissionDigest(same)
	if err != nil || first != second {
		t.Fatal("normalization/key identity", err)
	}
	for _, change := range []func(*AdmissionRequest){func(r *AdmissionRequest) { r.Source = strings.Replace(r.Source, "%2f", "%2F", 1) }, func(r *AdmissionRequest) { v := "a"; r.ExpectedName = &v }, func(r *AdmissionRequest) { r.BindingID = "98000000-0000-4000-8000-000000000099" }} {
		r := input
		change(&r)
		d, err := AdmissionDigest(r)
		if err != nil || d == first {
			t.Fatal("ambiguous intent", err)
		}
	}
	for _, bad := range []AdmissionRequest{{}, {OperationKey: "a/b", Source: input.Source, BindingID: admissionBinding, TargetPath: "/"}, {OperationKey: "k", Source: " https://example.invalid", BindingID: admissionBinding, TargetPath: "/"}, {OperationKey: "k", Source: input.Source, BindingID: admissionBinding, TargetPath: "\\bad"}} {
		if ValidateAdmissionRequest(bad) == nil {
			t.Fatal("invalid admitted")
		}
	}
	var typedNil *admissionStoreStub
	if _, err := NewAdmissionService(typedNil, &submitAdmissionStub{}, []storage.BindingID{admissionBinding}); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
}
