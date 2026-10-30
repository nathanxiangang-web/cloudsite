package producthttp

import (
	"context"
	"strings"
	"testing"

	"github.com/nathanxiangang-web/panta/internal/auth"
)

// Test-only auth port: failed revocation leaves the same session usable.
type adminLogoutRetryAccess struct {
	fakeAccess
	fail    bool
	revoked bool
}

func (a *adminLogoutRetryAccess) Logout(context.Context, string) error {
	if a.fail {
		return auth.ErrUnavailable
	}
	a.revoked = true
	return nil
}
func (a *adminLogoutRetryAccess) Authenticate(ctx context.Context, token string) (auth.Principal, error) {
	if a.revoked {
		return auth.Principal{}, auth.ErrUnauthorized
	}
	return a.fakeAccess.Authenticate(ctx, token)
}

func TestAdminUI1FailedLogoutDoesNotConfirmRevocationAndExistingRouteCanRetry(t *testing.T) {
	access := &adminLogoutRetryAccess{fakeAccess: fakeAccess{p: goodPrincipal()}, fail: true}
	access.p.Role = auth.RoleAdmin
	h, err := NewWorkspaceHandler(access, "http://127.0.0.1:8180", &readAccessFake{}, []DisplayBinding{workspaceBinding}, 15)
	if err != nil {
		t.Fatal(err)
	}
	w := request(h, "POST", "/api/v1/auth/logout", "", testToken, "http://127.0.0.1:8180")
	if w.Code != 503 || len(w.Result().Cookies()) != 0 || access.revoked {
		t.Fatal("failed logout incorrectly confirmed/cleared cookie", w.Code)
	}
	if me := request(h, "GET", "/api/v1/auth/me", "", testToken, ""); me.Code != 200 {
		t.Fatal("test must retain the original valid server session", me.Code)
	}
	access.fail = false
	w = request(h, "POST", "/api/v1/auth/logout", "", testToken, "http://127.0.0.1:8180")
	if w.Code != 204 || len(w.Result().Cookies()) != 1 || w.Result().Cookies()[0].MaxAge != -1 {
		t.Fatal("retry not confirmed", w.Code)
	}
	if me := request(h, "GET", "/api/v1/auth/me", "", testToken, ""); me.Code != 401 {
		t.Fatal("revoked server session restored on reload", me.Code)
	}
}

func TestAdminUI1DescriptorRechecksCurrentRoleAndRevokedSession(t *testing.T) {
	access := &fakeAccess{p: goodPrincipal()}
	access.p.Role = auth.RoleAdmin
	h, err := NewWorkspaceHandler(access, "http://127.0.0.1:8180", &readAccessFake{}, []DisplayBinding{workspaceBinding}, 15)
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct {
		role   auth.Role
		err    error
		status int
	}{
		{auth.RoleAdmin, nil, 200},
		{auth.RoleUser, nil, 403},
		{auth.RoleAdmin, nil, 200},
		{auth.RoleAdmin, auth.ErrUnauthorized, 401},
	} {
		access.p.Role, access.err = step.role, step.err
		w := request(h, "GET", "/api/v1/admin/workspace", "", testToken, "")
		if w.Code != step.status {
			t.Fatalf("fresh role/session: got %d want %d", w.Code, step.status)
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("admin response cacheable")
		}
		if step.status != 200 && strings.Contains(w.Body.String(), string(workspaceBinding.ID)) {
			t.Fatal("descriptor leaked after authorization loss")
		}
	}
}

func TestAdminUI1ShellIsIsolatedAndFutureAdminRoutesRemainAbsent(t *testing.T) {
	access := &fakeAccess{p: goodPrincipal()}
	access.p.Role = auth.RoleAdmin
	h, err := NewWorkspaceHandler(access, "http://127.0.0.1:8180", &readAccessFake{}, []DisplayBinding{workspaceBinding}, 15)
	if err != nil {
		t.Fatal(err)
	}
	page := request(h, "GET", "/admin/", "", "", "")
	if page.Code != 200 {
		t.Fatal(page.Code)
	} // Static shell carries no private DTO.
	for _, name := range []string{"overview", "users", "resources", "downloads", "storage", "indexcore", "shares", "audit"} {
		if !strings.Contains(page.Body.String(), "id=\"nav-"+name+"\"") {
			t.Fatal("navigation absent", name)
		}
	}
	if strings.Contains(page.Body.String(), `id="nav-settings"`) {
		t.Fatal("unfinished settings entry must be absent")
	}
	for _, entry := range []string{"/", "/downloads/"} {
		body := request(h, "GET", entry, "", "", "").Body.String()
		if strings.Contains(body, "/assets/app.css") || strings.Contains(body, "/assets/app.js") {
			t.Fatal("admin assets leaked into consumer", entry)
		}
	}
	// Shares and overview have protected read routes with their own tests.
	// The remaining names stay unimplemented.
	for _, name := range []string{"audit", "commands/reset"} {
		for _, method := range []string{"GET", "POST", "DELETE"} {
			w := request(h, method, "/api/v1/admin/"+name, "{}", testToken, "http://127.0.0.1:8180")
			if w.Code != 404 && w.Code != 405 {
				t.Fatal("future admin route unexpectedly implemented", method, name, w.Code)
			}
		}
	}
}
