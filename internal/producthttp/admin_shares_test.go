package producthttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nathanxiangang-web/panta/internal/adminops"
	"github.com/nathanxiangang-web/panta/internal/adminread"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

const shareRouteID = "99900000-0000-4000-8000-000000000001"

// shareDirectoryFake serves the additive ShareReads port. It panics on the
// legacy user/download reads so a mis-wired route cannot silently pass.
type shareDirectoryFake struct {
	calls  int
	filter adminread.ShareFilter
	page   adminread.ShareDirectory
	err    error
}

func (f *shareDirectoryFake) Users(context.Context, auth.Principal, int, string) (adminread.UserPage, error) {
	panic("share fake must not serve user reads")
}
func (f *shareDirectoryFake) Downloads(context.Context, auth.Principal, int, string, string) (adminread.DownloadPage, error) {
	panic("share fake must not serve download reads")
}
func (f *shareDirectoryFake) SearchShares(_ context.Context, p auth.Principal, filter adminread.ShareFilter) (adminread.ShareDirectory, error) {
	if p.Role != auth.RoleAdmin {
		panic("non-admin reached share port")
	}
	f.calls++
	f.filter = filter
	return f.page, f.err
}

// shareOperationsFake returns the caller-selected terminal result code so the
// three documented revoke outcomes can be asserted through the real handler.
type shareOperationsFake struct {
	calls        int
	status, code string
}

func (f *shareOperationsFake) Execute(_ context.Context, a adminops.Actor, c adminops.Command) (adminops.Result, error) {
	if a.SessionDigest != auth.TokenDigest(testToken) || a.Principal.Role != auth.RoleAdmin {
		panic("wrong actor")
	}
	f.calls++
	return adminops.Result{RequestID: c.RequestID, Action: c.Action, TargetID: c.TargetID, Status: f.status, Code: f.code}, nil
}
func (f *shareOperationsFake) Status(context.Context, adminops.Actor, string) (adminops.Result, error) {
	panic("unused status path")
}

func shareHandler(t *testing.T, a *fakeAccess, reads adminread.Reads, ops adminops.Operations) *WorkspaceHandler {
	t.Helper()
	h, err := NewWorkspaceHandler(a, "http://127.0.0.1:8180", &readAccessFake{}, []DisplayBinding{{ID: storage.BindingID(shareRouteID), Label: "Fixture"}}, 18, WithAdminReads(reads), WithAdminOperations(ops))
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestAdminShareRevokeHTTPBoundaryAndResultCodes(t *testing.T) {
	const origin = "http://127.0.0.1:8180"
	body := `{"request_id":"` + shareRouteID + `"}`
	cases := []struct {
		name, origin, token, accept, body string
		role                              auth.Role
		status, code                      string
		want, calls                       int
	}{
		{"ok", origin, testToken, "", body, auth.RoleAdmin, "SUCCEEDED", "SHARE_REVOKED", 200, 1},
		{"already-revoked", origin, testToken, "", body, auth.RoleAdmin, "REJECTED", "SHARE_ALREADY_REVOKED", 200, 1},
		{"not-found", origin, testToken, "", body, auth.RoleAdmin, "REJECTED", "SHARE_NOT_FOUND", 200, 1},
		{"anonymous", origin, "", "", body, auth.RoleAdmin, "", "", 401, 0},
		{"user-role", origin, testToken, "", body, auth.RoleUser, "", "", 403, 0},
		{"cross-origin", "https://evil.invalid", testToken, "", body, auth.RoleAdmin, "", "", 403, 0},
		{"no-origin", "", testToken, "", body, auth.RoleAdmin, "", "", 403, 0},
		{"bad-accept", origin, testToken, "text/html", body, auth.RoleAdmin, "", "", 406, 0},
		{"extra-field", origin, testToken, "", `{"request_id":"` + shareRouteID + `","login":"x"}`, auth.RoleAdmin, "", "", 400, 0},
		{"missing-request-id", origin, testToken, "", `{}`, auth.RoleAdmin, "", "", 400, 0},
		{"empty-body", origin, testToken, "", "", auth.RoleAdmin, "", "", 400, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ops := &shareOperationsFake{status: tc.status, code: tc.code}
			a := &fakeAccess{p: auth.Principal{UserID: shareRouteID, Login: "admin", Role: tc.role}}
			h := shareHandler(t, a, &shareDirectoryFake{}, ops)
			r := httptest.NewRequest("POST", origin+"/api/v1/admin/shares/"+shareRouteID+"/revoke", strings.NewReader(tc.body))
			r.Header.Set("Content-Type", "application/json")
			if tc.accept != "" {
				r.Header.Set("Accept", tc.accept)
			}
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			if tc.token != "" {
				r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: tc.token})
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.want || ops.calls != tc.calls {
				t.Fatal(w.Code, ops.calls, w.Body.String())
			}
			out := w.Body.String()
			for _, secret := range []string{"token_hash", "code_hash"} {
				if strings.Contains(out, secret) {
					t.Fatal("secret key leaked", secret)
				}
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("cacheable response")
			}
		})
	}
}

func TestAdminShareDirectoryHTTPGuardsQueryAndRedaction(t *testing.T) {
	const owner = "99900000-0000-4000-8000-0000000000aa"
	page := adminread.ShareDirectory{
		Items: []adminread.Share{{
			ShareID:       shareRouteID,
			OwnerID:       owner,
			BindingID:     shareRouteID,
			CanonicalPath: "/dir/file.txt",
			FileName:      "file.txt",
			SizeBytes:     4,
			CreatedAt:     time.Now(),
			ExpiresAt:     time.Now().Add(time.Hour),
			Active:        true,
		}},
		Total: 1, ReadAt: time.Now(), Scope: "FILTERED_PRODUCT_SHARES",
	}
	cases := []struct {
		name, query, accept string
		role                auth.Role
		token               string
		missingPort         bool
		want, calls         int
	}{
		{"anonymous", "", "", auth.RoleAdmin, "", false, 401, 0},
		{"user-role", "", "", auth.RoleUser, testToken, false, 403, 0},
		{"missing-port", "", "", auth.RoleAdmin, testToken, true, 503, 0},
		{"bad-accept", "", "text/html", auth.RoleAdmin, testToken, false, 406, 0},
		{"ok", "", "", auth.RoleAdmin, testToken, false, 200, 1},
		{"owner", "?owner=" + owner, "", auth.RoleAdmin, testToken, false, 200, 1},
		{"limit-zero", "?limit=0", "", auth.RoleAdmin, testToken, false, 400, 0},
		{"cursor-opaque", "?cursor=offset:1", "", auth.RoleAdmin, testToken, false, 400, 0},
		{"owner-invalid", "?owner=nope", "", auth.RoleAdmin, testToken, false, 400, 0},
		{"unknown-key", "?role=ADMIN", "", auth.RoleAdmin, testToken, false, 400, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &shareDirectoryFake{page: page}
			a := &fakeAccess{p: auth.Principal{UserID: shareRouteID, Login: "admin", Role: tc.role}}
			var reads adminread.Reads = fake
			if tc.missingPort {
				reads = nil
			}
			h := shareHandler(t, a, reads, &shareOperationsFake{})
			r := httptest.NewRequest("GET", "http://127.0.0.1:8180/api/v1/admin/shares"+tc.query, nil)
			if tc.accept != "" {
				r.Header.Set("Accept", tc.accept)
			}
			if tc.token != "" {
				r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: tc.token})
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.want || fake.calls != tc.calls {
				t.Fatal(w.Code, fake.calls, w.Body.String())
			}
			if tc.name == "ok" && fake.filter.Limit != 20 {
				t.Fatal("default limit", fake.filter.Limit)
			}
			if tc.name == "owner" && fake.filter.Owner != owner {
				t.Fatal("owner filter", fake.filter.Owner)
			}
			out := w.Body.String()
			for _, secret := range []string{"token_hash", "code_hash"} {
				if strings.Contains(out, secret) {
					t.Fatal("secret key leaked", secret)
				}
			}
		})
	}
}
