package producthttp

import (
	"context"
	"errors"
	"github.com/nathanxiangang-web/panta/internal/adminread"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"net/http"
	"net/http/httptest"
	"testing"
)

type downloadHTTPFake struct {
	adminReadsFake
	filter adminread.DownloadFilter
}

func (f *downloadHTTPFake) SearchDownloads(ctx context.Context, p auth.Principal, filter adminread.DownloadFilter) (adminread.DownloadPage, error) {
	f.calls++
	f.filter = filter
	return adminread.DownloadPage{Items: []adminread.Download{}}, f.err
}
func (f *downloadHTTPFake) DownloadDetail(ctx context.Context, p auth.Principal, id string) (adminread.DownloadDetail, error) {
	f.calls++
	return adminread.DownloadDetail{}, f.err
}
func TestAdminDownloadDiagnosticHTTPContract(t *testing.T) {
	const testUserID = "98000000-0000-4000-8000-000000000003"
	for _, tc := range []struct {
		method, path, token string
		role                auth.Role
		err                 error
		want, calls         int
	}{
		{"GET", "/api/v1/admin/downloads/" + testUserID, "", auth.RoleAdmin, nil, 401, 0},
		{"GET", "/api/v1/admin/downloads/" + testUserID, testToken, auth.RoleUser, nil, 403, 0},
		{"HEAD", "/api/v1/admin/downloads/" + testUserID, testToken, auth.RoleAdmin, nil, 405, 0},
		{"POST", "/api/v1/admin/downloads/" + testUserID, testToken, auth.RoleAdmin, nil, 405, 0},
		{"GET", "/api/v1/admin/downloads/bad", testToken, auth.RoleAdmin, nil, 400, 0},
		{"GET", "/api/v1/admin/downloads/" + testUserID + "?secret=x", testToken, auth.RoleAdmin, nil, 400, 0},
		{"GET", "/api/v1/admin/downloads/" + testUserID, testToken, auth.RoleAdmin, nil, 200, 1},
		{"GET", "/api/v1/admin/downloads/" + testUserID, testToken, auth.RoleAdmin, adminread.ErrDownloadNotFound, 404, 1},
		{"GET", "/api/v1/admin/downloads/" + testUserID, testToken, auth.RoleAdmin, errors.New("SECRET"), 503, 1},
		{"GET", "/api/v1/admin/downloads?status=FAILED&user_id=" + testUserID, testToken, auth.RoleAdmin, nil, 200, 1},
		{"GET", "/api/v1/admin/downloads?status=UNKNOWN", testToken, auth.RoleAdmin, nil, 400, 0},
		{"GET", "/api/v1/admin/downloads?status=", testToken, auth.RoleAdmin, nil, 400, 0},
		{"GET", "/api/v1/admin/downloads?status=READY&status=FAILED", testToken, auth.RoleAdmin, nil, 400, 0},
		{"GET", "/api/v1/admin/downloads?offset=20", testToken, auth.RoleAdmin, nil, 400, 0},
	} {
		t.Run(tc.method+tc.path+string(tc.role)+tc.token, func(t *testing.T) {
			p := goodPrincipal()
			p.Role = tc.role
			f := &downloadHTTPFake{adminReadsFake: adminReadsFake{err: tc.err}}
			h := adminHTTP(t, &fakeAccess{p: p}, f)
			w := request(h, tc.method, tc.path, "", tc.token, "http://127.0.0.1:8180")
			if w.Code != tc.want || f.calls != tc.calls {
				t.Fatal(w.Code, f.calls, w.Body.String())
			}
			if w.Code != 405 && w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("cacheable")
			}
		})
	}
	p := goodPrincipal()
	p.Role = auth.RoleAdmin
	f := &downloadHTTPFake{}
	h := adminHTTP(t, &fakeAccess{p: p}, f)
	r := httptest.NewRequest("GET", "http://127.0.0.1:8180/api/v1/admin/downloads/"+testUserID, nil)
	r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: testToken})
	r.Header.Set("Accept", "text/html")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 406 {
		t.Fatal(w.Code)
	}
}
