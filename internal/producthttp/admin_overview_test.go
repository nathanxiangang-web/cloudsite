package producthttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nathanxiangang-web/panta/internal/adminread"
	"github.com/nathanxiangang-web/panta/internal/auth"
)

type overviewHTTPFake struct{ adminReadsFake }

func (f *overviewHTTPFake) Overview(_ context.Context, p auth.Principal, owner string) (adminread.Overview, error) {
	if p.Role != auth.RoleAdmin {
		panic("non-admin overview")
	}
	f.calls++
	f.owner = owner
	return adminread.Overview{Source: adminread.OverviewSource, Scope: adminread.OverviewScope, OwnerUserID: owner, ReadAt: time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC)}, f.err
}
func TestAdminOverviewAuthorizationQueryAndSafeDTO(t *testing.T) {
	const owner = "98000000-0000-4000-8000-000000000003"
	for _, tc := range []struct {
		query, accept, token string
		role                 auth.Role
		err                  error
		status, calls        int
	}{
		{"", "", "", auth.RoleAdmin, nil, 401, 0}, {"", "", testToken, auth.RoleUser, nil, 403, 0},
		{"", "application/json", testToken, auth.RoleAdmin, nil, 200, 1},
		{"?user_id=" + owner, "", testToken, auth.RoleAdmin, nil, 200, 1},
		{"?user_id=", "", testToken, auth.RoleAdmin, nil, 400, 0}, {"?user_id=bad", "", testToken, auth.RoleAdmin, nil, 400, 0},
		{"?user_id=" + owner + "&user_id=" + owner, "", testToken, auth.RoleAdmin, nil, 400, 0},
		{"?limit=20", "", testToken, auth.RoleAdmin, nil, 400, 0}, {"?binding_id=" + owner, "", testToken, auth.RoleAdmin, nil, 400, 0},
		{"?source_ref=SECRET", "", testToken, auth.RoleAdmin, nil, 400, 0}, {"?user_id=%GG", "", testToken, auth.RoleAdmin, nil, 400, 0},
		{"", "text/html", testToken, auth.RoleAdmin, nil, 406, 0}, {"", "application/json;q=0,*/*;q=1", testToken, auth.RoleAdmin, nil, 406, 0},
		{"", "bad accept", testToken, auth.RoleAdmin, nil, 406, 0},
		{"", "", testToken, auth.RoleAdmin, errors.New("Cookie=SECRET source_ref=SECRET"), 503, 1},
	} {
		t.Run(tc.query+tc.accept+string(tc.role)+tc.token, func(t *testing.T) {
			p := goodPrincipal()
			p.Role = tc.role
			a := &fakeAccess{p: p}
			f := &overviewHTTPFake{adminReadsFake: adminReadsFake{err: tc.err}}
			h := adminHTTP(t, a, f)
			r := httptest.NewRequest("GET", "http://127.0.0.1:8180/api/v1/admin/overview"+tc.query, nil)
			r.Header.Set("X-Role", "ADMIN")
			r.Header.Set("X-User-ID", owner)
			if tc.token != "" {
				r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: tc.token})
			}
			if tc.accept != "" {
				r.Header.Set("Accept", tc.accept)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.status || f.calls != tc.calls {
				t.Fatal(w.Code, w.Body.String(), f.calls)
			}
			if w.Header().Get("Cache-Control") != "no-store" || strings.Contains(w.Body.String(), "SECRET") {
				t.Fatal("unsafe response")
			}
			if w.Code == 200 {
				var dto map[string]json.RawMessage
				if err := json.Unmarshal(w.Body.Bytes(), &dto); err != nil {
					t.Fatal(err)
				}
				for k := range dto {
					switch k {
					case "source", "scope", "owner_user_id", "read_at", "manifests", "jobs":
					default:
						t.Fatal("unapproved DTO field", k)
					}
				}
				if string(dto["manifests"]) == "null" || !strings.Contains(string(dto["manifests"]), `"total":0`) {
					t.Fatal("empty must be observed zero")
				}
				if tc.query != "" && f.owner != owner {
					t.Fatal("owner not forwarded")
				}
			}
		})
	}
}
func TestAdminOverviewFreshAuthMethodsAndUnavailable(t *testing.T) {
	p := goodPrincipal()
	p.Role = auth.RoleAdmin
	a := &fakeAccess{p: p}
	f := &overviewHTTPFake{}
	h := adminHTTP(t, a, f)
	if w := request(h, "GET", "/api/v1/admin/overview", "", testToken, ""); w.Code != 200 {
		t.Fatal(w.Code)
	}
	a.p.Role = auth.RoleUser
	if w := request(h, "GET", "/api/v1/admin/overview", "", testToken, ""); w.Code != 403 || f.calls != 1 {
		t.Fatal(w.Code)
	}
	a.p.Role = auth.RoleAdmin
	a.err = auth.ErrUnauthorized
	if w := request(h, "GET", "/api/v1/admin/overview", "", testToken, ""); w.Code != 401 || f.calls != 1 {
		t.Fatal(w.Code)
	}
	a.err = nil
	for _, method := range []string{"POST", "PATCH", "PUT", "DELETE", "HEAD"} {
		if w := request(h, method, "/api/v1/admin/overview", "", testToken, "http://127.0.0.1:8180"); w.Code != 405 || f.calls != 1 {
			t.Fatal(method, w.Code)
		}
	}
	for _, reads := range []adminread.Reads{nil, &adminReadsFake{}, (*overviewHTTPFake)(nil)} {
		if w := request(adminHTTP(t, a, reads), "GET", "/api/v1/admin/overview", "", testToken, ""); w.Code != 503 {
			t.Fatal("missing port fabricated zero", w.Code)
		}
	}
}
