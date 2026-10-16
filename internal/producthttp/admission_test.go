package producthttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nathanxiangang-web/panta/internal/acquisition"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/product"
)

type admissionFake struct {
	calls  int
	input  product.AdmissionRequest
	owner  auth.Principal
	err    error
	status acquisition.SubmissionStatus
}

func (f *admissionFake) Admit(_ context.Context, p auth.Principal, r product.AdmissionRequest) (product.AdmissionDTO, error) {
	f.calls++
	f.input = r
	f.owner = p
	status := f.status
	if status == "" {
		status = acquisition.SubmissionCreated
	}
	return product.AdmissionDTO{ManifestID: "98000000-0000-4000-8000-000000000003", JobID: "98000000-0000-4000-8000-000000000004", Status: status}, f.err
}
func (f *admissionFake) Usage(_ context.Context, p auth.Principal) (product.UsageDTO, error) {
	f.calls++
	f.owner = p
	return product.UsageDTO{Unit: "OPERATION", Price: "ZERO", OperationLimit: 10, Remaining: 10, ConcurrencyLimit: 2}, f.err
}
func admissionBody() string {
	return `{"operation_key":"once","source":"https://example.invalid/SECRET?sig=keep","binding_id":"98000000-0000-4000-8000-000000000001","target_path":"/"}`
}
func TestAdmissionHTTPStrictInputAndIdentity(t *testing.T) {
	for _, tc := range []struct {
		name, body, token, origin, path string
		want, calls                     int
	}{
		{"valid", admissionBody(), testToken, "http://127.0.0.1:8180", "", 201, 1},
		{"anonymous", admissionBody(), "", "http://127.0.0.1:8180", "", 401, 0},
		{"foreign origin", admissionBody(), testToken, "https://evil.invalid", "", 403, 0},
		{"missing origin", admissionBody(), testToken, "", "", 403, 0},
		{"query injection", admissionBody(), testToken, "http://127.0.0.1:8180", "?owner=x", 400, 0},
		{"duplicate", strings.Replace(admissionBody(), `"operation_key":"once"`, `"operation_key":"once","operation_key":"twice"`, 1), testToken, "http://127.0.0.1:8180", "", 400, 0},
		{"unknown", strings.Replace(admissionBody(), `"operation_key":"once"`, `"owner":"attacker","operation_key":"once"`, 1), testToken, "http://127.0.0.1:8180", "", 400, 0},
		{"null", strings.Replace(admissionBody(), `"target_path":"/"`, `"target_path":null`, 1), testToken, "http://127.0.0.1:8180", "", 400, 0},
		{"type", strings.Replace(admissionBody(), `"target_path":"/"`, `"target_path":2`, 1), testToken, "http://127.0.0.1:8180", "", 400, 0},
		{"missing", strings.Replace(admissionBody(), `,"target_path":"/"`, "", 1), testToken, "http://127.0.0.1:8180", "", 400, 0},
		{"trailing", admissionBody() + "{}", testToken, "http://127.0.0.1:8180", "", 400, 0},
		{"oversize", strings.Repeat(" ", 512<<10) + admissionBody(), testToken, "http://127.0.0.1:8180", "", 400, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			backend := &admissionFake{}
			access := &fakeAccess{p: goodPrincipal()}
			h, err := NewAdmissionHandler(access, "http://127.0.0.1:8180", backend)
			if err != nil {
				t.Fatal(err)
			}
			w := request(h, "POST", "/api/v1/acquisitions"+tc.path, tc.body, tc.token, tc.origin)
			if w.Code != tc.want || backend.calls != tc.calls {
				t.Fatal(w.Code, backend.calls, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "SECRET") || strings.Contains(w.Body.String(), "source") {
				t.Fatal("source disclosure")
			}
			if backend.calls > 0 && (backend.owner != access.p || backend.input.Source != "https://example.invalid/SECRET?sig=keep") {
				t.Fatal("identity/source changed")
			}
		})
	}
}
func TestAdmissionHTTPBoundsStatusesAndUsage(t *testing.T) {
	backend := &admissionFake{}
	h, _ := NewAdmissionHandler(&fakeAccess{p: goodPrincipal()}, "http://127.0.0.1:8180", backend)
	// More than the old 8192-byte body, still below the decoded64KiB source cap.
	raw := product.AdmissionRequest{OperationKey: "long", Source: "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567&tr=" + strings.Repeat("x", 10000), BindingID: "98000000-0000-4000-8000-000000000001", TargetPath: "/"}
	body, _ := json.Marshal(raw)
	if w := request(h, "POST", "/api/v1/acquisitions", string(body), testToken, "http://127.0.0.1:8180"); w.Code != 201 {
		t.Fatal("bounded long source rejected", w.Code)
	}
	raw.Source = strings.Repeat("x", acquisition.MaxSourceInputLength+1)
	body, _ = json.Marshal(raw)
	if w := request(h, "POST", "/api/v1/acquisitions", string(body), testToken, "http://127.0.0.1:8180"); w.Code != 400 {
		t.Fatal("decoded source cap missing")
	}
	for _, tc := range []struct {
		err  error
		want int
	}{{product.ErrInvalid, 400}, {product.ErrNotFound, 404}, {product.ErrOperationConflict, 409}, {product.ErrOperationLimit, 429}, {product.ErrConcurrencyLimit, 429}, {product.ErrAdmissionUncertain, 503}, {errors.New("SECRET"), 503}} {
		backend.err = tc.err
		w := request(h, "POST", "/api/v1/acquisitions", admissionBody(), testToken, "http://127.0.0.1:8180")
		if w.Code != tc.want || strings.Contains(w.Body.String(), "SECRET") {
			t.Fatal("unsafe error", w.Code)
		}
	}
	backend.err = nil
	for _, tc := range []struct {
		status acquisition.SubmissionStatus
		code   int
	}{{acquisition.SubmissionReplay, 200}, {acquisition.SubmissionPendingActivation, 202}, {"invalid", 503}} {
		backend.status = tc.status
		if w := request(h, "POST", "/api/v1/acquisitions", admissionBody(), testToken, "http://127.0.0.1:8180"); w.Code != tc.code {
			t.Fatal("status mismatch")
		}
	}
	for _, tc := range []struct {
		path, token string
		code        int
	}{{"/api/v1/usage", testToken, 200}, {"/api/v1/usage", "", 401}, {"/api/v1/usage?user_id=other", testToken, 400}} {
		w := request(h, "GET", tc.path, "", tc.token, "")
		if w.Code != tc.code {
			t.Fatal(w.Code)
		}
	}
	r := httptest.NewRequest("POST", "http://127.0.0.1:8180/api/v1/acquisitions", strings.NewReader(admissionBody()))
	r.Header.Set("Origin", "http://127.0.0.1:8180")
	r.Header.Set("Content-Type", "text/plain")
	r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: testToken})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatal("content type")
	}
	var nilBackend *admissionFake
	if _, err := NewAdmissionHandler(nil, "http://127.0.0.1:8180", nilBackend); err == nil {
		t.Fatal(fmt.Sprint("typed nil accepted"))
	}
}
