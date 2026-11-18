package product

import (
	"context"
	"errors"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/sessionhealth"
	"github.com/nathanxiangang-web/panta/internal/storage"
	"testing"
	"time"
)

type healthReadFunc func(context.Context) sessionhealth.View

func (f healthReadFunc) Read(ctx context.Context) sessionhealth.View { return f(ctx) }

type checkedStoreStub struct {
	admissionStoreStub
	gate   error
	replay bool
}

func (s *checkedStoreStub) ReserveChecked(ctx context.Context, p auth.Principal, r AdmissionRequest, c Reservation, check AdmissionCheck) (Reservation, error) {
	gate := check.Rejection()
	s.gate = gate
	if !s.replay && gate != nil {
		return Reservation{}, gate
	}
	return s.Reserve(ctx, p, r, c)
}
func TestAdmissionSessionGateAndReplay(t *testing.T) {
	for _, tc := range []struct {
		status  sessionhealth.Status
		expired bool
		want    error
	}{
		{sessionhealth.SessionAuthenticated, false, nil},
		{sessionhealth.SessionAuthenticated, true, ErrProviderSessionUnknown},
		{sessionhealth.SessionLoginRequired, false, ErrProviderLoginRequired},
		{sessionhealth.SessionUnknown, false, ErrProviderSessionUnknown},
	} {
		for _, replay := range []bool{false, true} {
			store := &checkedStoreStub{replay: replay}
			submit := &submitAdmissionStub{}
			health := healthReadFunc(func(context.Context) sessionhealth.View {
				until := time.Now().Add(time.Minute)
				if tc.expired {
					until = time.Now().Add(-time.Second)
				}
				return sessionhealth.View{Status: tc.status, ValidUntil: until}
			})
			service, err := NewAdmissionService(store, submit, []storage.BindingID{admissionBinding}, WithAdmissionHealth(health))
			if err != nil {
				t.Fatal(err)
			}
			_, err = service.Admit(context.Background(), admissionPrincipal(), AdmissionRequest{OperationKey: "stable", Source: "https://example.invalid/synthetic", BindingID: admissionBinding, TargetPath: "/"})
			if !errors.Is(store.gate, tc.want) {
				t.Fatal("gate", store.gate, tc.want)
			}
			if tc.want != nil && !replay {
				if !errors.Is(err, tc.want) || store.calls != 0 || submit.calls != 0 {
					t.Fatal(err, store.calls, submit.calls)
				}
			} else if err != nil || submit.calls != 1 {
				t.Fatal(err, submit.calls)
			}
		}
	}
	if _, err := NewAdmissionService(&admissionStoreStub{}, &submitAdmissionStub{}, []storage.BindingID{admissionBinding}, WithAdmissionHealth(healthReadFunc(func(context.Context) sessionhealth.View { return sessionhealth.View{} }))); err == nil {
		t.Fatal("unguarded store accepted")
	}
}
