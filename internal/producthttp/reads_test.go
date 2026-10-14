package producthttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nathanxiangang-web/panta/internal/acquisition"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/product"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

type readAccessFake struct {
	calls     int
	principal auth.Principal
	err       error
}

func (f *readAccessFake) invoked(p auth.Principal) error { f.calls++; f.principal = p; return f.err }
func (f *readAccessFake) Tasks(_ context.Context, p auth.Principal, _ int, _ string) (product.TaskPage, error) {
	return product.TaskPage{Items: []product.Task{}}, f.invoked(p)
}
func (f *readAccessFake) Task(_ context.Context, p auth.Principal, _ acquisition.ManifestID) (product.Task, error) {
	return product.Task{}, f.invoked(p)
}
func (f *readAccessFake) Browse(_ context.Context, p auth.Principal, _ product.BrowseRequest) (product.ResourcePage, error) {
	return product.ResourcePage{Items: []product.Resource{}}, f.invoked(p)
}
func (f *readAccessFake) Resolve(_ context.Context, p auth.Principal, _ storage.BindingID, _ string) (product.Resource, error) {
	return product.Resource{}, f.invoked(p)
}
func TestReadHTTPAuthQueryPolicyErrors(t *testing.T) {
	cases := []struct {
		name, path, token string
		backend           error
		want              int
		calls             int
	}{
		{"no cookie", "/api/v1/tasks", "", nil, 401, 0},
		{"bad credential", "/api/v1/tasks", "provider-cookie", nil, 401, 0},
		{"list", "/api/v1/tasks", testToken, nil, 200, 1},
		{"detail", "/api/v1/tasks/95000000-0000-4000-8000-000000000010", testToken, nil, 200, 1},
		{"unknown owner", "/api/v1/tasks?user_id=other", testToken, nil, 400, 0},
		{"client role", "/api/v1/tasks?role=ADMIN", testToken, nil, 400, 0},
		{"duplicate query", "/api/v1/tasks?limit=1&limit=2", testToken, nil, 400, 0},
		{"oversize page", "/api/v1/tasks?limit=51", testToken, nil, 400, 0},
		{"empty page", "/api/v1/tasks?limit=", testToken, nil, 400, 0},
		{"bad encoding", "/api/v1/tasks?cursor=%GG", testToken, nil, 400, 0},
		{"detail query", "/api/v1/tasks/95000000-0000-4000-8000-000000000010?raw=1", testToken, nil, 400, 0},
		{"unowned", "/api/v1/tasks/95000000-0000-4000-8000-000000000010", testToken, product.ErrNotFound, 404, 1},
		{"ambiguous", "/api/v1/resources/resolve?binding_id=example&path=/file", testToken, product.ErrAmbiguous, 409, 1},
		{"backend redacted", "/api/v1/tasks", testToken, errors.New("Cookie=SECRET/raw-source"), 503, 1},
		{"visibility injection", "/api/v1/resources?binding_id=example&include_removed=true", testToken, nil, 400, 0},
		{"arbitrary root", "/api/v1/resources?root_id=other", testToken, nil, 400, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := &fakeAccess{p: goodPrincipal()}
			f := &readAccessFake{err: tc.backend}
			h, err := NewReadHandler(a, "http://127.0.0.1:8180", f)
			if err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest("GET", "http://127.0.0.1:8180"+tc.path, nil)
			r.Header.Set("X-User-ID", "attacker")
			r.Header.Set("X-Role", "ADMIN")
			if tc.token != "" {
				r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: tc.token})
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.want || f.calls != tc.calls {
				t.Fatal(w.Code, f.calls, w.Body.String())
			}
			if f.calls > 0 && f.principal != a.p {
				t.Fatal("client identity forwarded")
			}
			if strings.Contains(w.Body.String(), "SECRET") || strings.Contains(w.Body.String(), "attacker") || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("unsafe response")
			}
		})
	}
}
func TestReadHTTPSharedHostAuthBoundary(t *testing.T) {
	a := &fakeAccess{p: goodPrincipal()}
	f := &readAccessFake{}
	h, _ := NewReadHandler(a, "http://127.0.0.1:8180", f)
	r := httptest.NewRequest("GET", "http://attacker.example/api/v1/tasks", nil)
	r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: testToken})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 || a.calls != 0 || f.calls != 0 {
		t.Fatal("host bypass")
	}
	a.err = auth.ErrUnauthorized
	w = request(h, "GET", "/api/v1/tasks", "", testToken, "")
	if w.Code != 401 || f.calls != 0 {
		t.Fatal("revoked identity accessed read store")
	}
	if _, err := NewReadHandler(a, "http://127.0.0.1:8180", nil); err == nil {
		t.Fatal("unwired read port")
	}
}
