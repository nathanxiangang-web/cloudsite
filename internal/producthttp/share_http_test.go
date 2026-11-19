package producthttp

import (
	"context"
	"encoding/json"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/fileaccess"
	"github.com/nathanxiangang-web/panta/internal/fileshare"
	"github.com/nathanxiangang-web/panta/internal/product"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type shareMemory struct {
	record  fileshare.Record
	grant   []byte
	revoked bool
}

func (m *shareMemory) Create(_ context.Context, r fileshare.Record, _ time.Duration) (fileshare.Record, error) {
	r.ExpiresAt = time.Now().Add(time.Hour)
	r.Active = true
	m.record = r
	return r, nil
}
func (m *shareMemory) List(_ context.Context, owner auth.UserID) ([]fileshare.Record, error) {
	if owner != m.record.Owner {
		return []fileshare.Record{}, nil
	}
	return []fileshare.Record{m.record}, nil
}
func (m *shareMemory) Revoke(_ context.Context, owner auth.UserID, id string) error {
	if owner != m.record.Owner || id != m.record.ID {
		return fileshare.ErrDenied
	}
	m.revoked = true
	return nil
}
func (m *shareMemory) Secret(_ context.Context, hash []byte) (fileshare.Record, error) {
	if m.revoked || string(hash) != string(m.record.TokenHash) {
		return fileshare.Record{}, fileshare.ErrDenied
	}
	return m.record, nil
}
func (m *shareMemory) IssueGrant(_ context.Context, id string, hash []byte) error {
	if m.revoked || id != m.record.ID {
		return fileshare.ErrDenied
	}
	m.grant = hash
	return nil
}
func (m *shareMemory) Granted(_ context.Context, id string, hash []byte) (fileshare.Record, auth.Principal, error) {
	if m.revoked || id != m.record.ID || string(hash) != string(m.grant) {
		return fileshare.Record{}, auth.Principal{}, fileshare.ErrDenied
	}
	return m.record, goodPrincipal(), nil
}
func TestShareHTTPAnonymousGrantRangeHeadAndRevoke(t *testing.T) {
	path := "/docs/name.md"
	directory := false
	size := int64(4)
	reads := contentReads{resource: product.Resource{BindingID: workspaceBinding.ID, ResourceID: "file-one", Path: &path, Directory: &directory, SizeBytes: &size, Presence: "PRESENT"}}
	source := &contentSource{target: "https://cdn.example.test/file?signature=opaque"}
	files, _ := fileaccess.New(reads, contentTopology{}, source)
	if err := files.EnableDirectLinks([]string{"cdn.example.test"}); err != nil {
		t.Fatal(err)
	}
	store := &shareMemory{}
	shares, _ := fileshare.New(store, files)
	h, _ := NewWorkspaceHandler(&fakeAccess{p: goodPrincipal()}, "http://127.0.0.1:8180", reads, []DisplayBinding{workspaceBinding}, 14, WithFileAccess(files), WithSharing(shares))
	call := func(method, path, body string, owner bool, cookie *http.Cookie, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://127.0.0.1:8180"+path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		if owner {
			r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: testToken})
		}
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	origin := "http://127.0.0.1:8180"
	body := `{"binding_id":"` + string(workspaceBinding.ID) + `","resource_id":"file-one","path":"/docs/name.md","duration":"1h","code":"aB39"}`
	if w := call("POST", "/api/v1/shares", body, false, nil, origin); w.Code != 401 {
		t.Fatal("anonymous create", w.Code)
	}
	if w := call("POST", "/api/v1/shares", body, true, nil, "http://evil.invalid"); w.Code != 403 {
		t.Fatal("cross-origin create")
	}
	if w := call("POST", "/api/v1/shares", `{"duration":"1h","duration":"7d"}`, true, nil, origin); w.Code != 400 {
		t.Fatal("duplicate fields")
	}
	w := call("POST", "/api/v1/shares", body, true, nil, origin)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var created struct{ ID, URL string }
	json.Unmarshal(w.Body.Bytes(), &created)
	token := strings.Split(created.URL, "#")[1]
	list := call("GET", "/api/v1/shares", "", true, nil, "")
	if strings.Contains(list.Body.String(), token) || strings.Contains(list.Body.String(), "/docs/") {
		t.Fatal("owner list leaked secrets or path")
	}
	redeem := func(code string) *httptest.ResponseRecorder {
		return call("POST", "/api/v1/shares/redeem", `{"token":"`+token+`","code":"`+code+`"}`, false, nil, origin)
	}
	if w = redeem("wrong-code"); w.Code != 404 {
		t.Fatal("code bypass")
	}
	w = redeem("aB39")
	if w.Code != 200 || len(w.Result().Cookies()) != 1 {
		t.Fatal("grant", w.Code)
	}
	cookie := w.Result().Cookies()[0]
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Value == token {
		t.Fatal("grant cookie unsafe")
	}
	var info struct {
		Download string `json:"download_url"`
	}
	json.Unmarshal(w.Body.Bytes(), &info)
	if w = call("GET", info.Download, "", false, nil, ""); w.Code != 404 {
		t.Fatal("missing grant")
	}
	forged := *cookie
	forged.Value = token
	if w = call("GET", info.Download, "", false, &forged, ""); w.Code != 404 {
		t.Fatal("link token forged code grant")
	}
	w = call("GET", info.Download, "", false, cookie, "")
	if w.Code != http.StatusFound || w.Body.Len() != 0 || w.Header().Get("Location") != source.target || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatal("guest download must be an empty-body 302", w.Code, w.Header())
	}
	w = call("HEAD", info.Download, "", false, cookie, "")
	if w.Code != http.StatusFound || w.Body.Len() != 0 || w.Header().Get("Location") != source.target {
		t.Fatal("HEAD must be an empty-body 302")
	}
	r := httptest.NewRequest("GET", origin+info.Download, nil)
	r.AddCookie(cookie)
	r.Header.Set("Range", "bytes=1-2")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusFound || w.Body.Len() != 0 || w.Header().Get("Location") != source.target {
		t.Fatal("guest Range must redirect without file bytes", w.Code)
	}
	if w = call("POST", "/api/v1/shares/"+created.ID+"/revoke", `{}`, true, nil, origin); w.Code != 204 {
		t.Fatal("revoke", w.Code)
	}
	before := source.links
	if w = call("GET", info.Download, "", false, cookie, ""); w.Code != 404 || source.links != before {
		t.Fatal("revoked grant reached bytes")
	}
}
