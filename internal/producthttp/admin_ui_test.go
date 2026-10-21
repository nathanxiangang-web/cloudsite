package producthttp

import (
	"strings"
	"testing"

	"github.com/nathanxiangang-web/panta/internal/auth"
)

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
	for _, name := range []string{"overview", "users", "resources", "downloads", "storage", "indexcore", "shares", "audit", "settings"} {
		if !strings.Contains(page.Body.String(), "id=\"nav-"+name+"\"") {
			t.Fatal("navigation absent", name)
		}
	}
	for _, entry := range []string{"/", "/downloads/"} {
		body := request(h, "GET", entry, "", "", "").Body.String()
		if strings.Contains(body, "/assets/app.css") || strings.Contains(body, "/assets/app.js") {
			t.Fatal("admin assets leaked into consumer", entry)
		}
	}
	for _, name := range []string{"users", "overview", "downloads", "storage", "indexcore", "shares", "audit", "commands/reset"} {
		for _, method := range []string{"GET", "POST", "DELETE"} {
			w := request(h, method, "/api/v1/admin/"+name, "{}", testToken, "http://127.0.0.1:8180")
			if w.Code != 404 && w.Code != 405 {
				t.Fatal("future admin route unexpectedly implemented", method, name, w.Code)
			}
		}
	}
}
