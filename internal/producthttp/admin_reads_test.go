package producthttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nathanxiangang-web/panta/internal/adminread"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

type adminReadsFake struct {
	calls, limit  int
	owner, cursor string
	err           error
}

func (f *adminReadsFake) Users(_ context.Context, p auth.Principal, n int, c string) (adminread.UserPage, error) {
	if p.Role != auth.RoleAdmin {
		panic("non-admin port call")
	}
	f.calls++
	f.limit = n
	f.cursor = c
	return adminread.UserPage{Items: []adminread.User{}}, f.err
}
func (f *adminReadsFake) Downloads(ctx context.Context, p auth.Principal, n int, c, u string) (adminread.DownloadPage, error) {
	_, err := f.Users(ctx, p, n, c)
	f.owner = u
	return adminread.DownloadPage{Items: []adminread.Download{}}, err
}
func adminHTTP(t *testing.T, a *fakeAccess, reads adminread.Reads) *WorkspaceHandler {
	t.Helper()
	h, err := NewWorkspaceHandler(a, "http://127.0.0.1:8180", &readAccessFake{}, []DisplayBinding{{ID: storage.BindingID("98000000-0000-4000-8000-000000000002"), Label: "Fixture"}}, 15, WithAdminReads(reads))
	if err != nil {
		t.Fatal(err)
	}
	return h
}
func TestAdminHTTPReadAuthorizationQueryAndRedaction(t *testing.T) {
	const uuid = "98000000-0000-4000-8000-000000000003"
	for _, route := range []string{"users", "downloads"} {
		for _, tc := range []struct {
			query, accept string
			role          auth.Role
			token         string
			backend       error
			want, calls   int
		}{
			{"", "", auth.RoleAdmin, "", nil, 401, 0}, {"", "", auth.RoleUser, testToken, nil, 403, 0},
			{"", "", auth.RoleAdmin, testToken, nil, 200, 1}, {"?limit=50&cursor=" + uuid, "application/json", auth.RoleAdmin, testToken, nil, 200, 1},
			{"?limit=0", "", auth.RoleAdmin, testToken, nil, 400, 0}, {"?limit=51", "", auth.RoleAdmin, testToken, nil, 400, 0},
			{"?limit=", "", auth.RoleAdmin, testToken, nil, 400, 0}, {"?limit=1&limit=2", "", auth.RoleAdmin, testToken, nil, 400, 0},
			{"?cursor=", "", auth.RoleAdmin, testToken, nil, 400, 0}, {"?cursor=offset:20", "", auth.RoleAdmin, testToken, nil, 400, 0},
			{"?cursor=" + uuid + "&cursor=" + uuid, "", auth.RoleAdmin, testToken, nil, 400, 0}, {"?cursor=%GG", "", auth.RoleAdmin, testToken, nil, 400, 0},
			{"?role=ADMIN", "", auth.RoleAdmin, testToken, nil, 400, 0}, {"?binding_id=" + uuid, "", auth.RoleAdmin, testToken, nil, 400, 0},
			{"?source_ref=secret", "", auth.RoleAdmin, testToken, nil, 400, 0}, {"?include_unowned=true", "", auth.RoleAdmin, testToken, nil, 400, 0},
			{"", "text/html", auth.RoleAdmin, testToken, nil, 406, 0}, {"", "application/json;q=0,*/*;q=1", auth.RoleAdmin, testToken, nil, 406, 0},
			{"", "application/*;q=0.5", auth.RoleAdmin, testToken, nil, 200, 1}, {"", "bad accept", auth.RoleAdmin, testToken, nil, 406, 0},
			{"", "", auth.RoleAdmin, testToken, errors.New("Cookie=SECRET token=SECRET raw-source provider_ref"), 503, 1},
			{"", "", auth.RoleAdmin, testToken, adminread.ErrInvalid, 400, 1},
		} {
			t.Run(route+tc.query+tc.accept+string(tc.role)+tc.token, func(t *testing.T) {
				p := goodPrincipal()
				p.Role = tc.role
				a := &fakeAccess{p: p}
				f := &adminReadsFake{err: tc.backend}
				h := adminHTTP(t, a, f)
				r := httptest.NewRequest("GET", "http://127.0.0.1:8180/api/v1/admin/"+route+tc.query, nil)
				r.Header.Set("X-Role", "ADMIN")
				r.Header.Set("X-User-ID", uuid)
				if tc.accept != "" {
					r.Header.Set("Accept", tc.accept)
				}
				if tc.token != "" {
					r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: tc.token})
				}
				w := httptest.NewRecorder()
				h.ServeHTTP(w, r)
				if w.Code != tc.want || f.calls != tc.calls {
					t.Fatal(w.Code, f.calls, w.Body.String())
				}
				if strings.Contains(w.Body.String(), "SECRET") || strings.Contains(w.Body.String(), "provider_ref") || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Security-Policy") == "" {
					t.Fatal("unsafe response")
				}
			})
		}
	}
}
func TestAdminHTTPFreshRoleDisabledMethodsAndOptionalPort(t *testing.T) {
	p := goodPrincipal()
	p.Role = auth.RoleAdmin
	a := &fakeAccess{p: p}
	f := &adminReadsFake{}
	h := adminHTTP(t, a, f)
	for _, route := range []string{"users", "downloads"} {
		a.p.Role = auth.RoleAdmin
		a.err = nil
		if w := request(h, "GET", "/api/v1/admin/"+route, "", testToken, ""); w.Code != 200 {
			t.Fatal(w.Code)
		}
		a.p.Role = auth.RoleUser
		before := f.calls
		if w := request(h, "GET", "/api/v1/admin/"+route, "", testToken, ""); w.Code != 403 || f.calls != before {
			t.Fatal("role downgrade bypass")
		}
		a.p.Role = auth.RoleAdmin
		a.err = auth.ErrUnauthorized
		if w := request(h, "GET", "/api/v1/admin/"+route, "", testToken, ""); w.Code != 401 || f.calls != before {
			t.Fatal("disabled session bypass")
		}
		a.err = nil
		for _, method := range []string{"POST", "PUT", "PATCH", "DELETE", "HEAD"} {
			w := request(h, method, "/api/v1/admin/"+route, "", testToken, "http://127.0.0.1:8180")
			if w.Code != 405 || f.calls != before {
				t.Fatal(method, w.Code)
			}
		}
		if w := request(adminHTTP(t, a, nil), "GET", "/api/v1/admin/"+route, "", testToken, ""); w.Code != 503 {
			t.Fatal("missing port", w.Code)
		}
	}
	const owner = "98000000-0000-4000-8000-000000000009"
	w := request(h, "GET", "/api/v1/admin/downloads?user_id="+owner, "", testToken, "")
	if w.Code != 200 || f.owner != owner || f.limit != 20 {
		t.Fatal(w.Code, f.owner)
	}
	for _, path := range []string{"/api/v1/admin/users?user_id=" + owner, "/api/v1/admin/downloads?user_id=", "/api/v1/admin/downloads?user_id=other", "/api/v1/admin/downloads?user_id=" + owner + "&user_id=" + owner, "/api/v1/tasks?user_id=" + owner} {
		if w := request(h, "GET", path, "", testToken, ""); w.Code != 400 {
			t.Fatal(path, w.Code)
		}
	}
	// A valid admin role in a forged header never changes cookie authentication.
	r := httptest.NewRequest("GET", "http://attacker.invalid/api/v1/admin/users", nil)
	r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: testToken})
	before := a.calls
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 || a.calls != before {
		t.Fatal("host boundary")
	}
	r = httptest.NewRequest("GET", "http://127.0.0.1:8180/api/v1/admin/users", nil)
	r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: testToken})
	r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: testToken})
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("duplicate cookie", w.Code)
	}
}
