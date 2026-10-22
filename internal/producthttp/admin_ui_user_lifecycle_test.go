package producthttp

import (
	"strings"
	"testing"

	"github.com/nathanxiangang-web/panta/internal/auth"
)

// Guards the UI-3B PR-B wiring: the reviewed USER lifecycle commands and their
// server codes must remain reachable from the admin assets, and the admin page
// must expose the explicit confirmation controls. This is an asset-contract
// test, not a browser test.
func TestAdminUIUserLifecycleAssetsWireReviewedCommands(t *testing.T) {
	access := &fakeAccess{p: goodPrincipal()}
	access.p.Role = auth.RoleAdmin
	h, err := NewWorkspaceHandler(access, "http://127.0.0.1:8180", &readAccessFake{}, []DisplayBinding{workspaceBinding}, 17, WithAdminOperations(&operationsFake{}))
	if err != nil {
		t.Fatal(err)
	}
	js := request(h, "GET", "/assets/admin-operations.js", "", "", "")
	if js.Code != 200 {
		t.Fatalf("admin-operations.js status %d", js.Code)
	}
	body := js.Body.String()
	for _, want := range []string{
		"RENAME_USER", "RESET_USER_PASSWORD", "DELETE_USER", "RESTORE_USER",
		"USER_RENAMED", "USER_PASSWORD_RESET", "USER_RESTORED", "USER_DELETED",
		"USER_STATE_CONFLICT", "LOGIN_CONFLICT", "ACTIVE_DOWNLOADS", "DOWNLOAD_STATE_CONFLICT", "PASSWORD_UNCHANGED",
		"/rename", "/password", "/restore",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("admin-operations.js missing %q", want)
		}
	}
	page := request(h, "GET", "/admin/", "", "", "")
	if page.Code != 200 {
		t.Fatalf("admin page status %d", page.Code)
	}
	html := page.Body.String()
	for _, want := range []string{
		"rename-user-form", "reset-user-password-form", "delete-user-form", "restore-user-form",
		"confirm-user-disable", "confirm-user-reset", "confirm-user-delete",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("admin page missing %q", want)
		}
	}
}