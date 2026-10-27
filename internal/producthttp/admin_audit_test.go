package producthttp

import (
	"context"
	"errors"
	"github.com/nathanxiangang-web/panta/internal/adminaudit"
	"github.com/nathanxiangang-web/panta/internal/adminops"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"net/http/httptest"
	"strings"
	"testing"
)

type auditHTTPFake struct {
	calls  int
	filter adminaudit.Filter
	actor  adminops.Actor
	err    error
}

func (f *auditHTTPFake) Search(_ context.Context, a adminops.Actor, q adminaudit.Filter) (adminaudit.Page, error) {
	f.calls++
	f.actor = a
	f.filter = q
	return adminaudit.Page{Items: []adminaudit.Item{}}, f.err
}
func (f *auditHTTPFake) Detail(_ context.Context, a adminops.Actor, actor, id string) (adminaudit.Detail, error) {
	f.calls++
	f.actor = a
	return adminaudit.Detail{}, f.err
}
func TestAdminAuditHTTPBoundaries(t *testing.T) {
	const path = "/api/v1/admin/audit/actions"
	const other = "99500000-0000-4000-8000-000000000099"
	for _, tc := range []struct {
		name, method, query, token, accept string
		role                               auth.Role
		missing                            bool
		err                                error
		want, calls                        int
	}{
		{name: "anonymous", want: 401}, {name: "user", token: testToken, role: auth.RoleUser, want: 403}, {name: "read", token: testToken, want: 200, calls: 1}, {name: "head", method: "HEAD", token: testToken, want: 405}, {name: "post", method: "POST", token: testToken, want: 405},
		{name: "accept", token: testToken, accept: "text/html", want: 406}, {name: "missing", token: testToken, missing: true, want: 503}, {name: "fault", token: testToken, err: errors.New("SECRET SQL"), want: 503, calls: 1},
		{name: "unknown-status", token: testToken, query: "?status=RECOVERY_REQUIRED", want: 400}, {name: "unknown-action", token: testToken, query: "?action=SQL", want: 400}, {name: "duplicate", token: testToken, query: "?status=UNKNOWN&status=UNKNOWN", want: 400}, {name: "unknown-field", token: testToken, query: "?outcome=raw", want: 400}, {name: "bad-time", token: testToken, query: "?from=never", want: 400}, {name: "micro", token: testToken, query: "?from=2026-10-27T00:00:00.123456789Z", want: 400}, {name: "empty-cursor-and-time", token: testToken, query: "?cursor=&from=2026-10-27T00:00:00Z", want: 400},
		{name: "foreign-actor", token: testToken, query: "?actor_id=" + other, want: 403}, {name: "foreign-detail", token: testToken, query: "/" + other + "/" + other, want: 404},
		{name: "filters", token: testToken, query: "?action=CREATE_USER&status=UNKNOWN&from=2026-10-26T00:00:00Z&before=2026-10-28T00:00:00Z", want: 200, calls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := goodPrincipal()
			p.Role = auth.RoleAdmin
			if tc.role != "" {
				p.Role = tc.role
			}
			f := &auditHTTPFake{err: tc.err}
			var port adminaudit.Reads = f
			if tc.missing {
				port = nil
			}
			h, e := NewWorkspaceHandler(&fakeAccess{p: p}, "http://127.0.0.1:8180", &readAccessFake{}, []DisplayBinding{{ID: "98000000-0000-4000-8000-000000000002", Label: "Fixture"}}, 19, WithAdminAuditReads(port))
			if e != nil {
				t.Fatal(e)
			}
			method := tc.method
			if method == "" {
				method = "GET"
			}
			req := httptest.NewRequest(method, "http://127.0.0.1:8180"+path+tc.query, nil)
			if tc.token != "" {
				req.Header.Set("Cookie", SessionCookieName+"="+tc.token)
			}
			if tc.accept != "" {
				req.Header.Set("Accept", tc.accept)
			}
			req.Header.Set("Origin", "http://127.0.0.1:8180")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			if w.Code != tc.want || f.calls != tc.calls || strings.Contains(w.Body.String(), "SECRET") {
				t.Fatal(w.Code, w.Body.String(), f.calls)
			}
			if method == "GET" && w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("cacheable audit")
			}
		})
	}
}
