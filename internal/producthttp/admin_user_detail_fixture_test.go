package producthttp

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/nathanxiangang-web/panta/internal/adminread"
	"github.com/nathanxiangang-web/panta/internal/auth"
)

// The Node behavior suite consumes this same fixture. Compare it with the
// actual server DTO so a self-consistent invented browser field cannot pass.
func TestAdminUserDetailBrowserFixtureMatchesServerDTO(t *testing.T) {
	stamp := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	detail := adminread.UserDetail{User: adminread.User{UserID: "99500000-0000-4000-8000-000000000002", Login: "test-user", Role: auth.RoleUser, Enabled: true, Version: 4, CreatedAt: stamp, UpdatedAt: stamp}, DownloadCount: 7, ActiveDownloads: 2}
	raw, e := json.Marshal(detail)
	if e != nil {
		t.Fatal(e)
	}
	fixture, e := os.ReadFile("../../test/fixtures/admin-user-detail.json")
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
		t.Fatalf("browser detail fixture diverged from server DTO: server=%s fixture=%s", raw, fixture)
	}
}
