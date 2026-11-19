package producthttp

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/nathanxiangang-web/panta/internal/fileaccess"
	"github.com/nathanxiangang-web/panta/internal/product"
)

type directContentSource struct {
	contentSource
	links int
	ua    string
}

func (s *directContentSource) ResolveLink(_ context.Context, r fileaccess.SourceRequest) (string, error) {
	s.links++
	s.ua = r.UserAgent
	return "https://cdn.example.test/a?temporary=opaque", nil
}
func TestDirectContentHTTP302AndPreviewRemainSeparate(t *testing.T) {
	path := "/docs/file.md"
	dir := false
	size := int64(4)
	reads := contentReads{resource: product.Resource{BindingID: workspaceBinding.ID, ResourceID: "file-one", Path: &path, Directory: &dir, SizeBytes: &size, Presence: "PRESENT"}}
	source := &directContentSource{}
	files, _ := fileaccess.New(reads, contentTopology{}, source)
	if files.EnableDirectLinks([]string{"cdn.example.test"}) != nil {
		t.Fatal("config")
	}
	previews, _ := fileaccess.NewPreviewService(files, previewCacheFake{}, pdfFake{})
	h, _ := NewWorkspaceHandler(&fakeAccess{p: goodPrincipal()}, "http://127.0.0.1:8180", reads, []DisplayBinding{workspaceBinding}, 14, WithFileAccess(files, previews))
	query := url.Values{"binding_id": {string(workspaceBinding.ID)}, "resource_id": {"file-one"}, "path": {path}}
	for _, method := range []string{"GET", "HEAD"} {
		for _, disposition := range []string{"inline", "attachment"} {
			query.Set("disposition", disposition)
			r := httptest.NewRequest(method, "http://127.0.0.1:8180/api/v1/files/content?"+query.Encode(), nil)
			r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: testToken})
			r.Header.Set("Range", "bytes=1-2")
			r.Header.Set("User-Agent", "Browser/3")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != 302 || w.Header().Get("Location") != "https://cdn.example.test/a?temporary=opaque" || w.Body.Len() != 0 || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Referrer-Policy") != "no-referrer" || source.calls != 0 || source.ua != "Browser/3" {
				t.Fatal("not byte-free302", w.Code)
			}
			if !strings.Contains(w.Header().Get("Content-Security-Policy"), "media-src 'self' https://cdn.example.test;") {
				t.Fatal("CSP blocks redirect media")
			}
		}
	}
	before := source.links
	query.Del("disposition")
	w := request(h, "GET", "/api/v1/files/content?"+query.Encode(), "", "", "")
	if w.Code != 401 || w.Header().Get("Location") != "" || source.links != before {
		t.Fatal("anonymous redirect")
	}
	w = request(h, "GET", "/api/v1/files/preview?"+query.Encode(), "", testToken, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"text":"text"`) || source.calls != 1 || source.links != before || w.Header().Get("Location") != "" {
		t.Fatal("preview stopped using private stream", w.Code)
	}
}

func TestInlineVideoHTTP302OnlyWithAuthenticatedPlayback(t *testing.T) {
	path := "/docs/_users/a/video.mp4"
	directory, size := false, int64(4)
	reads := contentReads{resource: product.Resource{BindingID: workspaceBinding.ID, ResourceID: "file-one", Path: &path, Directory: &directory, SizeBytes: &size, Presence: "PRESENT"}}
	source := &directContentSource{}
	files, _ := fileaccess.New(reads, contentTopology{}, source)
	if err := files.EnableDirectLinks([]string{"cdn.example.test"}); err != nil {
		t.Fatal(err)
	}
	h, err := NewWorkspaceHandler(&fakeAccess{p: goodPrincipal()}, "http://127.0.0.1:8180", reads, []DisplayBinding{workspaceBinding}, 22, WithFileAccess(files, nil))
	if err != nil {
		t.Fatal(err)
	}
	query := url.Values{"binding_id": {string(workspaceBinding.ID)}, "resource_id": {"file-one"}, "path": {path}, "disposition": {"inline"}}
	for _, tc := range []struct {
		token  string
		status int
	}{
		{"", http.StatusUnauthorized},
		{testToken, http.StatusFound},
	} {
		w := request(h, "GET", "/api/v1/files/content?"+query.Encode(), "", tc.token, "")
		if w.Code != tc.status {
			t.Fatalf("video response status = %d, want %d", w.Code, tc.status)
		}
		if tc.status == http.StatusFound {
			if w.Body.Len() != 0 || w.Header().Get("Location") == "" || w.Header().Get("Cache-Control") != "no-store" ||
				!strings.Contains(w.Header().Get("Content-Security-Policy"), "media-src 'self' https://cdn.example.test;") {
				t.Fatal("incorrect video redirect")
			}
		} else if w.Header().Get("Location") != "" {
			t.Fatal("anonymous redirect")
		}
	}
	if source.calls != 0 || source.links != 1 {
		t.Fatalf("metadata lookups = %d, byte opens = %d", source.links, source.calls)
	}
}

func TestWriteFileNeverStreamsByteBody(t *testing.T) {
	w := httptest.NewRecorder()
	body := io.NopCloser(strings.NewReader("secret-file-bytes"))
	writeFile(w, httptest.NewRequest("GET", "http://127.0.0.1:8180/content", nil), fileaccess.File{
		Content: fileaccess.Content{Body: body, Status: http.StatusOK, Length: 17},
	}, "attachment")
	if w.Code != http.StatusServiceUnavailable || strings.Contains(w.Body.String(), "secret-file-bytes") || w.Header().Get("Location") != "" {
		t.Fatal("content writer streamed bytes instead of failing closed", w.Code, w.Body.String())
	}
}
