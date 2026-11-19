package producthttp

import (
	"context"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/fileaccess"
	"github.com/nathanxiangang-web/panta/internal/product"
	"github.com/nathanxiangang-web/panta/internal/storage"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

type contentReads struct {
	product.Reads
	resource product.Resource
}

func (f contentReads) Resolve(context.Context, auth.Principal, storage.BindingID, string) (product.Resource, error) {
	return f.resource, nil
}

type contentTopology struct{}

func (contentTopology) GetBinding(context.Context, storage.BindingID) (storage.Binding, error) {
	return storage.Binding{ID: workspaceBinding.ID, OpenListMountPath: "/docs"}, nil
}

type contentSource struct {
	calls   int
	links   int
	target  string
	linkErr error
}
type memoryPreview struct{ *strings.Reader }

func (m memoryPreview) Close() error { return nil }

type previewCacheFake struct{}

func (previewCacheFake) Open(ctx context.Context, _ string, _ int64, load func(context.Context) (io.ReadCloser, error)) (io.ReadSeekCloser, error) {
	body, err := load(ctx)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	raw, err := io.ReadAll(body)
	return memoryPreview{strings.NewReader(string(raw))}, err
}

type pdfFake struct{}

func (pdfFake) PDF(context.Context, io.ReadSeeker, int) ([]byte, int, error) {
	return []byte{137, 80, 78, 71, 13, 10, 26, 10}, 1, nil
}
func (s *contentSource) ResolveLink(_ context.Context, _ fileaccess.SourceRequest) (string, error) {
	s.links++
	return s.target, s.linkErr
}

func (s *contentSource) OpenPreview(_ context.Context, r fileaccess.SourceRequest) (fileaccess.Content, error) {
	s.calls++
	start, end, partial, _ := fileaccess.ByteRange(r.Range, 4)
	body := "text"
	status := 200
	cr := ""
	if partial {
		body = body[start : end+1]
		status = 206
		cr = "bytes " + strconv.FormatInt(start, 10) + "-" + strconv.FormatInt(end, 10) + "/4"
	}
	return fileaccess.Content{Body: io.NopCloser(strings.NewReader(body)), Status: status, Length: int64(len(body)), ContentRange: cr}, nil
}
func TestContentFailsClosedWithoutDirectLinkConfigAndPreviewRemainsAvailable(t *testing.T) {
	path := "/docs/name.md"
	directory := false
	size := int64(4)
	reads := contentReads{resource: product.Resource{BindingID: workspaceBinding.ID, ResourceID: "file-one", Path: &path, Directory: &directory, SizeBytes: &size, Presence: "PRESENT"}}
	source := &contentSource{}
	service, _ := fileaccess.New(reads, contentTopology{}, source)
	previews, _ := fileaccess.NewPreviewService(service, previewCacheFake{}, pdfFake{})
	h, _ := NewWorkspaceHandler(&fakeAccess{p: goodPrincipal()}, "http://127.0.0.1:8180", reads, []DisplayBinding{workspaceBinding}, 13, WithFileAccess(service, previews))
	query := url.Values{"binding_id": {string(workspaceBinding.ID)}, "resource_id": {"file-one"}, "path": {path}}
	for _, c := range []struct {
		method, mode, token, extra, byteRange string
		status                                int
		body                                  string
	}{{"GET", "content", "", "", "", 401, ""}, {"GET", "content", testToken, "", "", 503, "CONTENT_UNAVAILABLE"}, {"GET", "content", testToken, "", "bytes=1-2", 503, "CONTENT_UNAVAILABLE"}, {"HEAD", "content", testToken, "", "", 503, ""}, {"GET", "content", testToken, "", "bytes=4-5", 416, "RANGE_NOT_SATISFIABLE"}, {"GET", "preview", testToken, "", "", 200, `"text":"text"`}, {"GET", "content", testToken, "&url=http://127.0.0.1", "", 400, ""}, {"GET", "content", testToken, "&disposition=unsafe", "", 400, ""}, {"GET", "content", testToken, "&resource_id=wrong", "", 400, ""}} {
		r := httptest.NewRequest(c.method, "http://127.0.0.1:8180/api/v1/files/"+c.mode+"?"+query.Encode()+c.extra, nil)
		if c.token != "" {
			r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: c.token})
		}
		if c.byteRange != "" {
			r.Header.Set("Range", c.byteRange)
		}
		beforeCalls := source.calls
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != c.status || c.body != "" && !strings.Contains(w.Body.String(), c.body) {
			t.Fatal(c, w.Code, w.Body.String())
		}
		if c.mode == "content" && source.calls != beforeCalls {
			t.Fatal("content request opened source bytes")
		}
	}
	if source.calls != 1 {
		t.Fatal("explicit preview did not use its preview-only source", source.calls)
	}
	w := request(h, "GET", "/api/v1/workspace", "", testToken, "")
	if !strings.Contains(w.Body.String(), `"content_access_enabled":true`) {
		t.Fatal("missing capability")
	}
}
func TestStreamConcurrencyGate(t *testing.T) {
	g := &streamGate{users: map[auth.UserID]int{}}
	for n := 0; n < 4; n++ {
		if !g.enter("user") {
			t.Fatal(n)
		}
	}
	if g.enter("user") {
		t.Fatal("per-user cap")
	}
	g.leave("user")
	if !g.enter("user") {
		t.Fatal("release")
	}
}

func TestPDFPreviewReturnsServerImageNotSourceURL(t *testing.T) {
	path := "/docs/file.pdf"
	directory := false
	size := int64(4)
	reads := contentReads{resource: product.Resource{BindingID: workspaceBinding.ID, ResourceID: "file-one", Path: &path, Directory: &directory, SizeBytes: &size, Presence: "PRESENT"}}
	source := &contentSource{}
	files, _ := fileaccess.New(reads, contentTopology{}, source)
	previews, _ := fileaccess.NewPreviewService(files, previewCacheFake{}, pdfFake{})
	h, _ := NewWorkspaceHandler(&fakeAccess{p: goodPrincipal()}, "http://127.0.0.1:8180", reads, []DisplayBinding{workspaceBinding}, 13, WithFileAccess(files, previews))
	query := url.Values{"binding_id": {string(workspaceBinding.ID)}, "resource_id": {"file-one"}, "path": {path}, "page": {"1"}}
	w := request(h, "GET", "/api/v1/files/preview?"+query.Encode(), "", testToken, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"image_base64":"iVBORw0KGgo="`) || !strings.Contains(w.Body.String(), `"pages":1`) {
		t.Fatal(w.Code, w.Body.String())
	}
	before := source.calls
	w = request(h, "HEAD", "/api/v1/files/preview?"+query.Encode(), "", testToken, "")
	if w.Code != 405 || source.calls != before {
		t.Fatal("HEAD preview reached source", w.Code)
	}
}
