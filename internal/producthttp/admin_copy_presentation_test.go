package producthttp

import (
	"context"
	"errors"
	"github.com/nathanxiangang-web/panta/internal/adminops"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/copypresentation"
	"net/http/httptest"
	"strings"
	"testing"
)

type copyReadHTTPFake struct{ operationsFake }

func (f *copyReadHTTPFake) CopyPresentation(_ context.Context, a adminops.Actor, id, binding string) (copypresentation.Detail, error) {
	f.calls++
	return copypresentation.Detail{}, f.err
}
func TestCopyPresentationHTTPStrictBoundary(t *testing.T) {
	const id = "99500000-0000-4000-8000-000000000001"
	base := "/api/v1/admin/copies/" + id + "/presentation"
	body := `{"request_id":"` + id + `","expected_version":0,"binding_id":"` + id + `","display_name":"Label","category":"AUTO","hidden":false}`
	for _, tc := range []struct {
		name, method, path, body, origin, token, accept string
		role                                            auth.Role
		missing                                         bool
		err                                             error
		want, calls                                     int
	}{
		{name: "read", method: "GET", path: base + "?binding_id=" + id, token: testToken, want: 200, calls: 1},
		{name: "write", method: "PUT", body: body, token: testToken, origin: "http://127.0.0.1:8180", want: 200, calls: 1},
		{name: "anonymous", method: "PUT", body: body, want: 401},
		{name: "user", method: "PUT", body: body, token: testToken, role: auth.RoleUser, want: 403},
		{name: "no-origin", method: "PUT", body: body, token: testToken, want: 403},
		{name: "foreign-origin", method: "PUT", body: body, token: testToken, origin: "https://evil.invalid", want: 403},
		{name: "delete", method: "DELETE", token: testToken, want: 405},
		{name: "head", method: "HEAD", token: testToken, want: 405},
		{name: "post", method: "POST", token: testToken, want: 405},
		{name: "missing-port", method: "GET", path: base + "?binding_id=" + id, token: testToken, missing: true, want: 503},
		{name: "accept", method: "GET", path: base + "?binding_id=" + id, token: testToken, accept: "text/html", want: 406},
		{name: "missing-binding", method: "GET", token: testToken, want: 400},
		{name: "duplicate-query", method: "GET", path: base + "?binding_id=" + id + "&binding_id=" + id, token: testToken, want: 400},
		{name: "forged-owner", method: "PUT", body: strings.Replace(body, `"hidden":false`, `"hidden":false,"owner_id":"other"`, 1), token: testToken, origin: "http://127.0.0.1:8180", want: 400},
		{name: "null", method: "PUT", body: strings.Replace(body, `"hidden":false`, `"hidden":null`, 1), token: testToken, origin: "http://127.0.0.1:8180", want: 400},
		{name: "html", method: "PUT", body: strings.Replace(body, "Label", "<script>", 1), token: testToken, origin: "http://127.0.0.1:8180", want: 400},
		{name: "bad-utf8", method: "PUT", body: strings.Replace(body, "Label", string([]byte{255}), 1), token: testToken, origin: "http://127.0.0.1:8180", want: 400},
		{name: "surrogate", method: "PUT", body: strings.Replace(body, "Label", `\ud800`, 1), token: testToken, origin: "http://127.0.0.1:8180", want: 400},
		{name: "controls", method: "PUT", body: strings.Replace(body, "Label", `a\nname`, 1), token: testToken, origin: "http://127.0.0.1:8180", want: 400},
		{name: "version", method: "PUT", body: strings.Replace(body, `"expected_version":0`, `"expected_version":-1`, 1), token: testToken, origin: "http://127.0.0.1:8180", want: 400},
		{name: "fraction", method: "PUT", body: strings.Replace(body, `"expected_version":0`, `"expected_version":1.5`, 1), token: testToken, origin: "http://127.0.0.1:8180", want: 400},
		{name: "duplicate-field", method: "PUT", body: strings.Replace(body, `"hidden":false`, `"hidden":false,"hidden":true`, 1), token: testToken, origin: "http://127.0.0.1:8180", want: 400},
		{name: "oversize", method: "PUT", body: strings.Repeat("a", 4097), token: testToken, origin: "http://127.0.0.1:8180", want: 400},
		{name: "unavailable", method: "PUT", body: body, token: testToken, origin: "http://127.0.0.1:8180", err: errors.New("SECRET SQL"), want: 503, calls: 1},
		{name: "not-found", method: "GET", path: base + "?binding_id=" + id, token: testToken, err: adminops.ErrNotFound, want: 404, calls: 1},
		{name: "conflict", method: "PUT", body: body, token: testToken, origin: "http://127.0.0.1:8180", err: adminops.ErrConflict, want: 409, calls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			role := tc.role
			if role == "" {
				role = auth.RoleAdmin
			}
			f := &copyReadHTTPFake{}
			f.err = tc.err
			var port adminops.Operations = f
			if tc.missing {
				port = nil
			}
			h, e := NewWorkspaceHandler(&fakeAccess{p: auth.Principal{UserID: auth.UserID(id), Login: "admin", Role: role}}, "http://127.0.0.1:8180", &readAccessFake{}, []DisplayBinding{workspaceBinding}, 20, WithAdminOperations(port))
			if e != nil {
				t.Fatal(e)
			}
			path := tc.path
			if path == "" {
				path = base
			}
			req := httptest.NewRequest(tc.method, "http://127.0.0.1:8180"+path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			origin := tc.origin
			if origin == "" && tc.name != "no-origin" {
				origin = "http://127.0.0.1:8180"
			}
			if origin != "" {
				req.Header.Set("Origin", origin)
			}
			if tc.accept != "" {
				req.Header.Set("Accept", tc.accept)
			}
			if tc.token != "" {
				req.Header.Set("Cookie", SessionCookieName+"="+tc.token)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			if w.Code != tc.want || f.calls != tc.calls || strings.Contains(w.Body.String(), "SECRET") {
				t.Fatal(w.Code, w.Body.String(), f.calls)
			}
			if f.command.Action != "" && (f.command.Presentation == nil || f.command.Action != adminops.SetCopyPresentation) {
				t.Fatal(f.command)
			}
		})
	}
}
