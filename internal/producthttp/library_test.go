package producthttp

import (
	"strings"
	"testing"

	"github.com/nathanxiangang-web/panta/internal/auth"
)

func TestConsumerAndAdministratorSurfacesAreSeparate(t *testing.T) {
	access := &fakeAccess{p: goodPrincipal()}
	h, err := NewWorkspaceHandler(access, "http://127.0.0.1:8180", &readAccessFake{}, []DisplayBinding{workspaceBinding}, 13)
	if err != nil {
		t.Fatal(err)
	}
	front := request(h, "GET", "/", "", "", "")
	admin := request(h, "GET", "/admin/", "", "", "")
	if front.Code != 200 || admin.Code != 200 || !strings.Contains(front.Body.String(), "/assets/library.js") || !strings.Contains(admin.Body.String(), "/assets/app.js") {
		t.Fatal("separate UI shells missing")
	}
	for _, forbidden := range []string{"binding-select", "directory-back", "resolve-form", "Copy 投影", "Schema", "删除文件"} {
		if strings.Contains(front.Body.String(), forbidden) {
			t.Fatal("operational concept leaked to frontend", forbidden)
		}
	}
	for _, asset := range []string{"library.js", "file-catalog.js", "library.css"} {
		w := request(h, "GET", "/assets/"+asset, "", "", "")
		if w.Code != 200 {
			t.Fatal("asset unavailable", asset)
		}
		for _, unsafe := range []string{"innerHTML", "localStorage", "sessionStorage", "document.cookie", "https://", "iframe"} {
			if strings.Contains(w.Body.String(), unsafe) {
				t.Fatal("unsafe consumer asset", asset, unsafe)
			}
		}
	}
	if w := request(h, "GET", "/api/v1/admin/workspace", "", "", ""); w.Code != 401 {
		t.Fatal("anonymous admin", w.Code)
	}
	if w := request(h, "GET", "/api/v1/admin/workspace", "", testToken, ""); w.Code != 403 {
		t.Fatal("USER admin", w.Code)
	}
	access.p.Role = auth.RoleAdmin
	if w := request(h, "GET", "/api/v1/admin/workspace", "", testToken, ""); w.Code != 200 {
		t.Fatal("ADMIN denied", w.Code)
	}
	if w := request(h, "GET", "/api/v1/admin/workspace?unknown=x", "", testToken, ""); w.Code != 400 {
		t.Fatal("query not strict", w.Code)
	}
	if w := request(h, "POST", "/api/v1/admin/workspace", `{}`, testToken, "http://127.0.0.1:8180"); w.Code != 405 {
		t.Fatal("mutation route introduced", w.Code)
	}
}
