package producthttp

import (
	"context"
	"github.com/nathanxiangang-web/panta/internal/fileaccess"
	"github.com/nathanxiangang-web/panta/internal/fileshare"
	"github.com/nathanxiangang-web/panta/internal/product"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGuestHTTPRedirectRevalidatesGrantAndCanonicalIdentity(t *testing.T) {
	path := "/docs/file.md"
	dir := false
	size := int64(4)
	reads := contentReads{resource: product.Resource{BindingID: workspaceBinding.ID, ResourceID: "file-one", Path: &path, Directory: &dir, SizeBytes: &size, Presence: "PRESENT"}}
	source := &directContentSource{}
	files, _ := fileaccess.New(reads, contentTopology{}, source)
	files.EnableDirectLinks([]string{"cdn.example.test"})
	store := &shareMemory{}
	shares, _ := fileshare.New(store, files)
	h, _ := NewWorkspaceHandler(&fakeAccess{p: goodPrincipal()}, "http://127.0.0.1:8180", reads, []DisplayBinding{workspaceBinding}, 14, WithFileAccess(files), WithSharing(shares))
	record, token, err := shares.Create(context.Background(), goodPrincipal(), fileaccess.Request{BindingID: workspaceBinding.ID, ResourceID: "file-one", Path: path}, "1h", "aB39")
	if err != nil {
		t.Fatal(err)
	}
	grant, err := shares.Redeem(context.Background(), token, "aB39")
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"GET", "HEAD"} {
		r := httptest.NewRequest(method, "http://127.0.0.1:8180/api/v1/shared/"+record.ID+"/content", nil)
		r.AddCookie(&http.Cookie{Name: "panta_share", Value: grant.Token})
		r.Header.Set("User-Agent", "Guest/1")
		r.Header.Set("Range", "bytes=1-2")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 302 || w.Header().Get("Location") == "" || w.Body.Len() != 0 || source.ua != "Guest/1" || source.calls != 0 {
			t.Fatal("guest delivery not redirected", w.Code)
		}
	}
	before := source.links
	store.revoked = true
	r := httptest.NewRequest("GET", "http://127.0.0.1:8180/api/v1/shared/"+record.ID+"/content", nil)
	r.AddCookie(&http.Cookie{Name: "panta_share", Value: grant.Token})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 404 || w.Header().Get("Location") != "" || source.links != before {
		t.Fatal("revoked guest got redirect")
	}
}
