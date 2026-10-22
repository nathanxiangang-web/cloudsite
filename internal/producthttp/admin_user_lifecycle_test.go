package producthttp

import (
	"github.com/nathanxiangang-web/panta/internal/adminops"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/storage"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAdminUserLifecycleHTTPRoutes(t *testing.T) {
	const id = "99500000-0000-4000-8000-000000000001"
	for _, tc := range []struct{ method, suffix, fields, action string }{
		{"POST", "/rename", `,"login":"renamed"`, adminops.RenameUser},
		{"POST", "/password", `,"password":"new-password"`, adminops.ResetPassword},
		{"DELETE", "", "", adminops.DeleteUser},
		{"POST", "/restore", `,"password":"new-password"`, adminops.RestoreUser},
	} {
		for _, scenario := range []string{"allowed", "anonymous", "user", "cross-origin", "duplicate-key", "extra-field", "null-version"} {
			t.Run(tc.action+"/"+scenario, func(t *testing.T) {
				a := &fakeAccess{p: auth.Principal{UserID: id, Login: "admin", Role: auth.RoleAdmin}}
				f := &operationsFake{}
				token, origin := testToken, "http://127.0.0.1:8180"
				want := 200
				body := `{"request_id":"` + id + `","expected_version":0` + tc.fields + `}`
				switch scenario {
				case "anonymous":
					token = ""
					want = 401
				case "user":
					a.p.Role = auth.RoleUser
					want = 403
				case "cross-origin":
					origin = "https://evil.invalid"
					want = 403
				case "duplicate-key":
					body = strings.TrimSuffix(body, "}") + `,"expected_version":1}`
					want = 400
				case "extra-field":
					body = strings.TrimSuffix(body, "}") + `,"role":"ADMIN"}`
					want = 400
				case "null-version":
					body = strings.Replace(body, `"expected_version":0`, `"expected_version":null`, 1)
					want = 400
				}
				h, e := NewWorkspaceHandler(a, "http://127.0.0.1:8180", &readAccessFake{}, []DisplayBinding{{ID: storage.BindingID(id), Label: "Fixture"}}, 17, WithAdminOperations(f))
				if e != nil {
					t.Fatal(e)
				}
				req := httptest.NewRequest(tc.method, "http://127.0.0.1:8180/api/v1/admin/users/"+id+tc.suffix, strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Origin", origin)
				if token != "" {
					req.Header.Set("Cookie", SessionCookieName+"="+token)
				}
				w := httptest.NewRecorder()
				h.ServeHTTP(w, req)
				if w.Code != want {
					t.Fatal(w.Code, w.Body.String())
				}
				if want == 200 {
					if f.calls != 1 || f.command.Action != tc.action || !f.command.Valid() {
						t.Fatal("wrong command", f.command.Action)
					}
				} else if f.calls != 0 {
					t.Fatal("denied request reached command")
				}
				if strings.Contains(w.Body.String(), "new-password") {
					t.Fatal("password echo")
				}
			})
		}
	}
}
