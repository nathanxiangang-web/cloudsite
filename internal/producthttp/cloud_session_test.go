package producthttp

import (
	"context"
	"errors"
	"github.com/nathanxiangang-web/panta/internal/product"
	"github.com/nathanxiangang-web/panta/internal/sessionhealth"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type sessionReadFake struct{ calls int }

func (f *sessionReadFake) Read(context.Context) sessionhealth.View {
	f.calls++
	now := time.Now().UTC()
	return sessionhealth.View{Status: sessionhealth.SessionLoginRequired, CheckedAt: now, ValidUntil: now.Add(time.Second)}
}
func TestCloudSessionProtectedReadOnlyAndSeparateFromWebsiteLogin(t *testing.T) {
	health := &sessionReadFake{}
	ops := &operationFake{}
	admit := &admissionFake{}
	h, err := NewWorkspaceHandler(&fakeAccess{p: goodPrincipal()}, "http://127.0.0.1:8180", &readAccessFake{}, []DisplayBinding{workspaceBinding}, 21, WithCloudDownload(cloudTarget(), admit, ops, health))
	if err != nil {
		t.Fatal(err)
	}
	if health.calls != 0 {
		t.Fatal("constructor performed I/O")
	}
	for _, tc := range []struct {
		path, token string
		code        int
	}{
		{"/api/v1/cloud-downloads/session", "", 401},
		{"/api/v1/cloud-downloads/session?connection=private", testToken, 400},
		{"/api/v1/cloud-downloads/session", testToken, 200},
	} {
		w := request(h, "GET", tc.path, "", tc.token, "")
		if w.Code != tc.code {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if health.calls != 1 || admit.calls != 0 {
		t.Fatal(health.calls, admit.calls)
	}
	w := request(h, "GET", "/api/v1/cloud-downloads/operations/web-once", "", testToken, "")
	if w.Code != 200 || ops.calls != 1 || health.calls != 1 {
		t.Fatal(w.Code, ops.calls, health.calls)
	}
	for _, e := range []error{product.ErrProviderLoginRequired, product.ErrProviderSessionUnknown} {
		w := httptest.NewRecorder()
		admissionFailure(w, e)
		if w.Code != 503 || !strings.Contains(w.Body.String(), "PROVIDER_") || errors.Is(e, product.ErrAdmissionUncertain) {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}
