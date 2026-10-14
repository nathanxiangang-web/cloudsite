package producthttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nathanxiangang-web/panta/internal/auth"
)

const testToken = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

type fakeAccess struct {
	p      auth.Principal
	err    error
	calls  int
	logout int
}

func (f *fakeAccess) Login(context.Context, string, string) (auth.LoginResult, error) {
	f.calls++
	return auth.LoginResult{Principal: f.p, Token: testToken, ExpiresAt: time.Now().Add(time.Hour)}, f.err
}
func (f *fakeAccess) Authenticate(context.Context, string) (auth.Principal, error) {
	f.calls++
	return f.p, f.err
}
func (f *fakeAccess) Logout(context.Context, string) error { f.logout++; return f.err }
func goodPrincipal() auth.Principal {
	return auth.Principal{UserID: "90000000-0000-4000-8000-000000000001", Login: "nathan", Role: auth.RoleUser}
}
func request(h http.Handler, method, path, body, token, origin string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://127.0.0.1:8180"+path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	if token != "" {
		r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: token})
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func TestAuthHTTPContractCookieAndRedaction(t *testing.T) {
	f := &fakeAccess{p: goodPrincipal()}
	h, err := NewAuthHandler(f, "http://127.0.0.1:8180")
	if err != nil {
		t.Fatal(err)
	}
	w := request(h, "POST", "/api/v1/auth/login", `{"login":"nathan","password":"test-only-password-123"}`, "", "http://127.0.0.1:8180")
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	if strings.Contains(w.Body.String(), testToken) || strings.Contains(w.Body.String(), "password") {
		t.Fatal("secret response")
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatal("missing cookie")
	}
	c := cookies[0]
	if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Domain != "" || c.Path != "/" || c.MaxAge != 3600 || c.Secure {
		t.Fatal("unsafe development cookie")
	}
	w = request(h, "GET", "/api/v1/auth/me", "", testToken, "")
	var data map[string]any
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &data) != nil || len(data) != 3 || data["user_id"] != string(f.p.UserID) {
		t.Fatal("unsafe DTO")
	}
	w = request(h, "POST", "/api/v1/auth/logout", "", testToken, "http://127.0.0.1:8180")
	if w.Code != 204 || f.logout != 1 || w.Result().Cookies()[0].MaxAge != -1 {
		t.Fatal("logout failed")
	}
	https, err := NewAuthHandler(f, "https://product.example")
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "https://product.example/api/v1/auth/login", strings.NewReader(`{"login":"nathan","password":"test-only-password-123"}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "https://product.example")
	w = httptest.NewRecorder()
	https.ServeHTTP(w, r)
	if w.Code != 200 || !w.Result().Cookies()[0].Secure {
		t.Fatal("HTTPS cookie not Secure")
	}
}
func TestAuthHTTPRefusalMatrix(t *testing.T) {
	cases := []struct {
		name, method, body, token, origin string
		err                               error
		p                                 auth.Principal
		want                              int
	}{
		{"missing", "GET", "", "", "", nil, goodPrincipal(), 401},
		{"bad token", "GET", "", "opaque-provider-ref", "", nil, goodPrincipal(), 401},
		{"unavailable", "GET", "", testToken, "", errors.New("UID=SECRET provider/raw/url"), goodPrincipal(), 503},
		{"expired", "GET", "", testToken, "", auth.ErrUnauthorized, goodPrincipal(), 401},
		{"invalid principal", "GET", "", testToken, "", nil, auth.Principal{UserID: "provider", Login: "nathan", Role: auth.RoleAdmin}, 503},
		{"origin absent", "POST", `{}`, "", "", nil, goodPrincipal(), 403},
		{"origin cross site", "POST", `{}`, "", "https://attacker.example", nil, goodPrincipal(), 403},
		{"invalid json", "POST", `{broken`, "", "http://127.0.0.1:8180", nil, goodPrincipal(), 400},
		{"client role", "POST", `{"login":"nathan","password":"test-only-password-123","role":"ADMIN"}`, "", "http://127.0.0.1:8180", nil, goodPrincipal(), 400},
		{"duplicate login", "POST", `{"login":"nathan","login":"other","password":"test-only-password-123"}`, "", "http://127.0.0.1:8180", nil, goodPrincipal(), 400},
		{"oversize", "POST", strings.Repeat(" ", 8193), "", "http://127.0.0.1:8180", nil, goodPrincipal(), 400},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeAccess{p: tc.p, err: tc.err}
			h, _ := NewAuthHandler(f, "http://127.0.0.1:8180")
			path := "/api/v1/auth/me"
			if tc.method == "POST" {
				path = "/api/v1/auth/login"
			}
			w := request(h, tc.method, path, tc.body, tc.token, tc.origin)
			if w.Code != tc.want {
				t.Fatalf("status %d want %d", w.Code, tc.want)
			}
			if strings.Contains(w.Body.String(), "SECRET") || len(w.Result().Cookies()) > 0 {
				t.Fatal("failure leaked a secret/cookie")
			}
			if tc.want == 400 || tc.want == 403 || (tc.want == 401 && tc.token != testToken) {
				if f.calls != 0 {
					t.Fatal("refusal called authentication backend")
				}
			}
		})
	}
}
func TestUnwiredTypedNilAndClientIdentityFailClosed(t *testing.T) {
	for _, a := range []auth.Access{nil, (*auth.Service)(nil)} {
		h, _ := NewAuthHandler(a, "http://127.0.0.1:8180")
		w := request(h, "GET", "/api/v1/auth/me", "", testToken, "")
		if w.Code != 503 {
			t.Fatal(w.Code)
		}
	}
	f := &fakeAccess{p: goodPrincipal()}
	h, _ := NewAuthHandler(f, "http://127.0.0.1:8180")
	r := httptest.NewRequest("GET", "http://127.0.0.1:8180/api/v1/auth/me", nil)
	r.Header.Set("X-User-ID", string(f.p.UserID))
	r.Header.Set("X-Role", "ADMIN")
	r.Header.Set("Authorization", "Bearer "+testToken)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 401 || f.calls != 0 {
		t.Fatal("client asserted identity")
	}
	r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: testToken})
	r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: testToken})
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("duplicate credential admitted")
	}
	for _, origin := range []string{"http://0.0.0.0:8080", "http://example.com", "https://user:password@example.com", "https://example.com?token=secret"} {
		if _, err := NewAuthHandler(f, origin); err == nil {
			t.Fatal("unsafe origin")
		}
	}
}
