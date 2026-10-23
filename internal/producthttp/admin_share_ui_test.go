package producthttp

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/nathanxiangang-web/panta/internal/adminread"
	"github.com/nathanxiangang-web/panta/internal/auth"
)

func TestAdminShareBrowserFixtureMatchesServerDTO(t *testing.T) {
	stamp := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	first := adminread.Share{ShareID: "99910000-0000-4000-8000-000000000001", OwnerID: "99910000-0000-4000-8000-000000000002", BindingID: string(workspaceBinding.ID), CanonicalPath: "/docs/guide.pdf", FileName: "guide.pdf", SizeBytes: 4096, CreatedAt: stamp, ExpiresAt: stamp.Add(7 * 24 * time.Hour), Active: true}
	second := first
	second.ShareID = "99910000-0000-4000-8000-000000000003"
	second.CanonicalPath = "/docs/old-notes.txt"
	second.FileName = "old-notes.txt"
	second.SizeBytes = 128
	second.ExpiresAt = stamp.Add(12 * time.Hour)
	second.Active = false
	page := adminread.ShareDirectory{Items: []adminread.Share{first, second}, Total: 2, ReadAt: stamp.Add(24 * time.Hour), Scope: "FILTERED_PRODUCT_SHARES"}
	raw, e := json.Marshal(page)
	if e != nil {
		t.Fatal(e)
	}
	fixture, e := os.ReadFile("../../test/fixtures/admin-share-directory.json")
	if e != nil {
		t.Fatal(e)
	}
	var actual, want map[string]any
	if e = json.Unmarshal(raw, &actual); e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(fixture, &want); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(actual, want) {
		t.Fatal("share browser fixture diverged from server DTO")
	}
}

func TestAdminShareUIAssetsUseOnlyReviewedSharePorts(t *testing.T) {
	access := &fakeAccess{p: goodPrincipal()}
	access.p.Role = auth.RoleAdmin
	h, e := NewWorkspaceHandler(access, "http://127.0.0.1:8180", &readAccessFake{}, []DisplayBinding{workspaceBinding}, 18)
	if e != nil {
		t.Fatal(e)
	}
	asset := request(h, "GET", "/assets/admin-shares.js", "", "", "")
	if asset.Code != 200 || !strings.Contains(asset.Header().Get("Content-Type"), "javascript") {
		t.Fatal("share asset not served", asset.Code)
	}
	for _, want := range []string{"/api/v1/admin/shares?", "/revoke", "/api/v1/admin/actions/", "REVOKE_SHARE", "SHARE_REVOKED", "SHARE_ALREADY_REVOKED", "SHARE_NOT_FOUND", "RESERVED_NO_CONFIRMED_RESULT"} {
		if !strings.Contains(asset.Body.String(), want) {
			t.Fatal("missing share port/receipt", want)
		}
	}
	page := request(h, "GET", "/admin/", "", "", "")
	for _, want := range []string{"/assets/admin-shares.js", "shares-panel", "share-revoke-confirm", "lookup-share-action"} {
		if !strings.Contains(page.Body.String(), want) {
			t.Fatal("missing share UI wiring", want)
		}
	}
}
