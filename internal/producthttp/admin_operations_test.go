package producthttp

import (
	"context"
	"errors"
	"github.com/nathanxiangang-web/panta/internal/adminops"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/storage"
	"net/http/httptest"
	"strings"
	"testing"
)

type operationsFake struct {
	calls   int
	command adminops.Command
	err     error
}

func (f *operationsFake) Execute(_ context.Context, a adminops.Actor, c adminops.Command) (adminops.Result, error) {
	if a.SessionDigest != auth.TokenDigest(testToken) || a.Principal.Role != auth.RoleAdmin {
		panic("wrong actor")
	}
	f.calls++
	f.command = c
	return adminops.Result{RequestID: c.RequestID, Status: "SUCCEEDED"}, f.err
}
func (f *operationsFake) Status(ctx context.Context, a adminops.Actor, id string) (adminops.Result, error) {
	return f.Execute(ctx, a, adminops.Command{RequestID: id})
}
func TestAdminOperationsHTTPStrictMutationBoundary(t *testing.T) {
	const id = "99500000-0000-4000-8000-000000000001"
	body := `{"request_id":"` + id + `","login":"new-user","password":"secret-password"}`
	cases := []struct {
		name, method, path, body, origin, token, accept string
		role                                            auth.Role
		want, calls                                     int
	}{
		{"create", "POST", "/api/v1/admin/users", body, "http://127.0.0.1:8180", testToken, "", auth.RoleAdmin, 200, 1},
		{"no-origin", "POST", "/api/v1/admin/users", body, "", testToken, "", auth.RoleAdmin, 403, 0},
		{"cross-origin", "POST", "/api/v1/admin/users", body, "https://evil.invalid", testToken, "", auth.RoleAdmin, 403, 0},
		{"no-cookie", "POST", "/api/v1/admin/users", body, "http://127.0.0.1:8180", "", "", auth.RoleAdmin, 401, 0},
		{"role", "POST", "/api/v1/admin/users", body, "http://127.0.0.1:8180", testToken, "", auth.RoleUser, 403, 0},
		{"forged-role", "POST", "/api/v1/admin/users", strings.TrimSuffix(body, "}") + `,"role":"ADMIN"}`, "http://127.0.0.1:8180", testToken, "", auth.RoleAdmin, 400, 0},
		{"duplicate", "POST", "/api/v1/admin/users", strings.TrimSuffix(body, "}") + `,"login":"other"}`, "http://127.0.0.1:8180", testToken, "", auth.RoleAdmin, 400, 0},
		{"null", "POST", "/api/v1/admin/users", `{"request_id":null,"login":"abc","password":"secret-password"}`, "http://127.0.0.1:8180", testToken, "", auth.RoleAdmin, 400, 0},
		{"extra", "POST", "/api/v1/admin/users", body + `{}`, "http://127.0.0.1:8180", testToken, "", auth.RoleAdmin, 400, 0},
		{"scope", "POST", "/api/v1/admin/storage/" + id + "/probe", `{"request_id":"` + id + `","cookie":"SECRET"}`, "http://127.0.0.1:8180", testToken, "", auth.RoleAdmin, 400, 0},
		{"probe", "POST", "/api/v1/admin/storage/" + id + "/probe", `{"request_id":"` + id + `"}`, "http://127.0.0.1:8180", testToken, "", auth.RoleAdmin, 200, 1},
		{"enable", "PATCH", "/api/v1/admin/users/" + id, `{"request_id":"` + id + `","enabled":false,"expected_version":0}`, "http://127.0.0.1:8180", testToken, "", auth.RoleAdmin, 200, 1},
		{"null-state", "PATCH", "/api/v1/admin/users/" + id, `{"request_id":"` + id + `","enabled":null,"expected_version":0}`, "http://127.0.0.1:8180", testToken, "", auth.RoleAdmin, 400, 0},
		{"accept", "POST", "/api/v1/admin/users", body, "http://127.0.0.1:8180", testToken, "text/html", auth.RoleAdmin, 406, 0},
		{"query", "POST", "/api/v1/admin/users?role=ADMIN", body, "http://127.0.0.1:8180", testToken, "", auth.RoleAdmin, 400, 0},
		{"oversize", "POST", "/api/v1/admin/users", strings.Repeat("x", 4097), "http://127.0.0.1:8180", testToken, "", auth.RoleAdmin, 400, 0},
		{"status", "GET", "/api/v1/admin/actions/" + id, "", "", testToken, "", auth.RoleAdmin, 200, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := &fakeAccess{p: auth.Principal{UserID: auth.UserID(id), Login: "admin", Role: tc.role}}
			f := &operationsFake{}
			h, e := NewWorkspaceHandler(a, "http://127.0.0.1:8180", &readAccessFake{}, []DisplayBinding{{ID: storage.BindingID(id), Label: "Test"}}, 16, WithAdminOperations(f))
			if e != nil {
				t.Fatal(e)
			}
			req := httptest.NewRequest(tc.method, "http://127.0.0.1:8180"+tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			if tc.accept != "" {
				req.Header.Set("Accept", tc.accept)
			}
			req.Header.Set("X-Role", "ADMIN")
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if tc.token != "" {
				req.Header.Set("Cookie", SessionCookieName+"="+tc.token)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			if w.Code != tc.want || f.calls != tc.calls {
				t.Fatal(w.Code, f.calls, w.Body.String())
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("cacheable")
			}
			if strings.Contains(w.Body.String(), "secret-password") {
				t.Fatal("secret echoed")
			}
		})
	}
}
func TestAdminOperationsErrorResponsesNeverExposeUpstream(t *testing.T) {
	f := &operationsFake{err: errors.New("Cookie=SECRET https://SECRET.invalid")}
	a := &fakeAccess{p: auth.Principal{UserID: "99500000-0000-4000-8000-000000000001", Login: "admin", Role: auth.RoleAdmin}}
	h, _ := NewWorkspaceHandler(a, "http://127.0.0.1:8180", &readAccessFake{}, []DisplayBinding{{ID: "99500000-0000-4000-8000-000000000001", Label: "Test"}}, 16, WithAdminOperations(f))
	w := request(h, "GET", "/api/v1/admin/actions/99500000-0000-4000-8000-000000000001", "", testToken, "")
	if w.Code != 503 || strings.Contains(w.Body.String(), "SECRET") {
		t.Fatal(w.Code, w.Body.String())
	}
}
