package producthttp

import (
	"strings"
	"testing"
)

func TestResourcePresentationServesSafeAssetsWithoutWritableRoutes(t *testing.T) {
	h, err := NewWorkspaceHandler(&fakeAccess{p: goodPrincipal()}, "http://127.0.0.1:8180", &readAccessFake{}, []DisplayBinding{workspaceBinding}, 13)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/assets/resource-display.js", "/assets/app.js"} {
		w := request(h, "GET", path, "", "", "")
		if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Type"), "text/javascript") {
			t.Fatal(path, w.Code)
		}
		for _, unsafe := range []string{"innerHTML", "localStorage", "sessionStorage", "document.cookie", "https://", "iframe"} {
			if strings.Contains(w.Body.String(), unsafe) {
				t.Fatal("unsafe asset", path, unsafe)
			}
		}
	}
	page := request(h, "GET", "/", "", "", "").Body.String()
	for _, id := range []string{"category-software", "category-video", "category-document", "resource-search", "view-cards", "file-detail", "file-note"} {
		if !strings.Contains(page, `id="`+id+`"`) {
			t.Fatal("missing presentation", id)
		}
	}
	if !strings.Contains(page, "不是整个网盘") || !strings.Contains(page, "不包含文件正文") {
		t.Fatal("scope/content incorrectly implied")
	}
	if strings.Contains(page, "删除文件") {
		t.Fatal("file deletion is excluded by the human's scope")
	}
	for _, path := range []string{"/api/v1/files/97000000-0000-4000-8000-000000000001", "/api/v1/resources/97000000-0000-4000-8000-000000000001"} {
		w := request(h, "DELETE", path, "", testToken, "http://127.0.0.1:8180")
		if w.Code != 404 && w.Code != 405 {
			t.Fatal("file-delete API unexpectedly available", path, w.Code)
		}
	}
	for _, path := range []string{"/api/v1/acquisitions", "/api/v1/files/access", "/api/v1/shares"} {
		w := request(h, "POST", path, `{}`, testToken, "http://127.0.0.1:8180")
		if w.Code != 404 && w.Code != 405 {
			t.Fatal("readonly router mutation", path, w.Code)
		}
	}
	w := request(h, "GET", "/api/v1/resources", "", "", "")
	if w.Code != 401 {
		t.Fatal("private file metadata without auth", w.Code)
	}
}
