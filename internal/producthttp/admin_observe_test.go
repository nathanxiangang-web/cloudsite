package producthttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nathanxiangang-web/panta/internal/adminobserve"
	"github.com/nathanxiangang-web/panta/internal/auth"
)

type observeFake struct {
	calls int
	err   error
}

func (f *observeFake) Storage(context.Context, auth.Principal, int, string) (adminobserve.Page, error) {
	f.calls++
	return adminobserve.Page{Items: []adminobserve.Storage{}}, f.err
}
func (f *observeFake) Root(context.Context, auth.Principal, string) (adminobserve.Observation, error) {
	f.calls++
	return adminobserve.Observation{}, f.err
}
func observeHTTP(t *testing.T, a *fakeAccess, f adminobserve.Reads) *WorkspaceHandler {
	t.Helper()
	h, e := NewWorkspaceHandler(a, "http://127.0.0.1:8180", &readAccessFake{}, []DisplayBinding{workspaceBinding}, 15, WithAdminObservation(f))
	if e != nil {
		t.Fatal(e)
	}
	return h
}
func TestAdminObserveHTTPStrictAuthorizationAndInput(t *testing.T) {
	const id = "99000000-0000-4000-8000-000000000001"
	for _, tc := range []struct {
		path, accept string
		want         int
	}{
		{"storage", "", 200}, {"storage?limit=50&cursor=" + id, "application/json", 200}, {"indexcore?binding_id=" + id, "", 200},
		{"storage?limit=0", "", 400}, {"storage?limit=51", "", 400}, {"storage?cursor=", "", 400}, {"storage?cursor=bad", "", 400}, {"storage?limit=1&limit=2", "", 400},
		{"storage?binding_id=" + id, "", 400}, {"storage?cursor=%GG", "", 400}, {"indexcore", "", 400}, {"indexcore?binding_id=", "", 400}, {"indexcore?binding_id=bad", "", 400},
		{"indexcore?binding_id=" + id + "&binding_id=" + id, "", 400}, {"indexcore?binding_id=" + id + "&root_id=SECRET", "", 400}, {"indexcore?binding_id=" + id + "&base_url=https://SECRET", "", 400},
		{"storage?role=ADMIN", "", 400}, {"storage", "text/html", 406}, {"indexcore?binding_id=" + id, "application/json;q=0", 406},
	} {
		t.Run(tc.path+tc.accept, func(t *testing.T) {
			p := goodPrincipal()
			p.Role = auth.RoleAdmin
			a := &fakeAccess{p: p}
			f := &observeFake{}
			h := observeHTTP(t, a, f)
			r := httptest.NewRequest("GET", "http://127.0.0.1:8180/api/v1/admin/"+tc.path, nil)
			r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: testToken})
			r.Header.Set("X-Role", "ADMIN")
			if tc.accept != "" {
				r.Header.Set("Accept", tc.accept)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			wantCalls := 0
			if tc.want == 200 {
				wantCalls = 1
			}
			if w.Code != tc.want || f.calls != wantCalls || strings.Contains(w.Body.String(), "SECRET") || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal(w.Code, f.calls, w.Body.String())
			}
		})
	}
	for _, path := range []string{"storage", "indexcore?binding_id=" + id} {
		p := goodPrincipal()
		p.Role = auth.RoleAdmin
		a := &fakeAccess{p: p}
		f := &observeFake{}
		h := observeHTTP(t, a, f)
		for _, step := range []struct {
			token string
			role  auth.Role
			err   error
			want  int
		}{{"", auth.RoleAdmin, nil, 401}, {testToken, auth.RoleUser, nil, 403}, {testToken, auth.RoleAdmin, nil, 200}, {testToken, auth.RoleUser, nil, 403}, {testToken, auth.RoleAdmin, auth.ErrUnauthorized, 401}} {
			a.p.Role = step.role
			a.err = step.err
			before := f.calls
			w := request(h, "GET", "/api/v1/admin/"+path, "", step.token, "")
			if w.Code != step.want || step.want != 200 && before != f.calls {
				t.Fatal(w.Code)
			}
		}
		a.err = nil
		a.p.Role = auth.RoleAdmin
		for _, method := range []string{"POST", "PUT", "DELETE", "PATCH", "HEAD"} {
			before := f.calls
			if w := request(h, method, "/api/v1/admin/"+path, "", testToken, "http://127.0.0.1:8180"); w.Code != 405 || f.calls != before {
				t.Fatal(method, w.Code)
			}
		}
		if w := request(observeHTTP(t, a, nil), "GET", "/api/v1/admin/"+path, "", testToken, ""); w.Code != 503 {
			t.Fatal("missing port")
		}
	}
}
func TestAdminObserveHTTPStableRedactedFailures(t *testing.T) {
	p := goodPrincipal()
	p.Role = auth.RoleAdmin
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{{adminobserve.ErrNotConnected, 503, "NOT_CONNECTED"}, {adminobserve.ErrNotReady, 503, "ROOT_NOT_READY"}, {adminobserve.ErrNotFound, 404, "NOT_FOUND"}, {adminobserve.ErrRootNotFound, 404, "ROOT_NOT_FOUND"}, {adminobserve.ErrBusy, 429, "OBSERVATION_BUSY"}, {context.DeadlineExceeded, 503, "OBSERVATION_UNKNOWN"}, {errors.New("credential_ref=SECRET provider_scope=SECRET mount=SECRET cookie=SECRET https://SECRET"), 503, "OBSERVATION_UNKNOWN"}} {
		h := observeHTTP(t, &fakeAccess{p: p}, &observeFake{err: tc.err})
		w := request(h, "GET", "/api/v1/admin/indexcore?binding_id="+string(workspaceBinding.ID), "", testToken, "")
		if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.code) || strings.Contains(w.Body.String(), "SECRET") {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}
