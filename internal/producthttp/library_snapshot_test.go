package producthttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/libraryview"
	"github.com/nathanxiangang-web/panta/internal/product"
)

type libraryFake struct {
	calls int
	err   error
}

func (f *libraryFake) Library(context.Context, auth.Principal, product.LibraryRequest) (product.LibraryPage, error) {
	f.calls++
	return product.LibraryPage{Items: []product.Resource{}, Counts: &libraryview.Counts{}, Ready: true, Revision: strings.Repeat("a", 64), Limit: 50}, f.err
}
func TestLibraryHTTPAuthenticationBeforeConditionalRead(t *testing.T) {
	a := &fakeAccess{p: goodPrincipal()}
	f := &libraryFake{}
	h, err := NewWorkspaceHandler(a, "http://127.0.0.1:8180", &readAccessFake{}, []DisplayBinding{workspaceBinding}, 21, WithLibraryReads(f))
	if err != nil {
		t.Fatal(err)
	}
	if w := request(h, "GET", "/api/v1/library", "", "", ""); w.Code != 401 || f.calls != 0 {
		t.Fatal(w.Code, f.calls)
	}
	w := request(h, "GET", "/api/v1/library", "", testToken, "")
	tag := w.Header().Get("ETag")
	if w.Code != 200 || tag == "" || w.Header().Get("Cache-Control") != "private, no-cache" || w.Header().Get("Vary") != "Cookie" {
		t.Fatal(w.Code, w.Header())
	}
	conditional := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "http://127.0.0.1:8180/api/v1/library", nil)
		r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: testToken})
		r.Header.Set("If-None-Match", tag)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	before := a.calls
	w = conditional()
	if w.Code != 304 || w.Body.Len() != 0 || a.calls != before+1 {
		t.Fatal(w.Code, a.calls)
	}
	a.err = auth.ErrUnauthorized
	before = f.calls
	w = conditional()
	if w.Code != 401 || f.calls != before || w.Header().Get("ETag") != "" {
		t.Fatal("conditional authentication bypass", w.Code)
	}
}
func TestLibraryHTTPStrictScopesErrorsAndCapability(t *testing.T) {
	a := &fakeAccess{p: goodPrincipal()}
	f := &libraryFake{}
	h, _ := NewWorkspaceHandler(a, "http://127.0.0.1:8180", &readAccessFake{}, []DisplayBinding{workspaceBinding}, 21, WithLibraryReads(f))
	if w := request(h, "GET", "/api/v1/workspace", "", testToken, ""); !strings.Contains(w.Body.String(), `"library_snapshot_enabled":true`) {
		t.Fatal(w.Body.String())
	}
	for _, q := range []string{"?root_id=other", "?binding_id=other", "?surface=library&surface=downloads", "?page=-1", "?page=2000", "?page=1&cursor=other"} {
		if w := request(h, "GET", "/api/v1/library"+q, "", testToken, ""); w.Code != 400 {
			t.Fatal(w.Code)
		}
	}
	for _, tc := range []struct {
		err    error
		status int
	}{{libraryview.ErrInvalid, 400}, {libraryview.ErrStale, 409}, {libraryview.ErrUnavailable, 503}} {
		f.err = tc.err
		if w := request(h, "GET", "/api/v1/library", "", testToken, ""); w.Code != tc.status || w.Header().Get("ETag") != "" {
			t.Fatal(w.Code)
		}
	}
}
