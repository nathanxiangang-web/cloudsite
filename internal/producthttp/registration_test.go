package producthttp

import (
	"context"
	"errors"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"strings"
	"testing"
)

type signupAccess struct {
	fakeAccess
	registrations int
	registerErr   error
}

func (f *signupAccess) Register(_ context.Context, login, password string) (auth.Principal, error) {
	f.registrations++
	p := goodPrincipal()
	p.Login = login
	return p, f.registerErr
}

func TestRegistrationExplicitCapabilityStrictInputAndNoSession(t *testing.T) {
	f := &signupAccess{}
	h, err := NewWorkspaceHandler(f, "http://127.0.0.1:8180", &readAccessFake{}, []DisplayBinding{workspaceBinding}, 14)
	if err != nil {
		t.Fatal(err)
	}
	if w := request(h, "GET", "/api/v1/auth/capabilities", "", "", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"registration_enabled":false`) {
		t.Fatal("default opened")
	}
	body := `{"login":"New.User","password":"test-only-password-123"}`
	if w := request(h, "POST", "/api/v1/auth/register", body, "", "http://127.0.0.1:8180"); w.Code != 404 || f.registrations != 0 {
		t.Fatal("closed registration admitted")
	}
	if _, err = NewWorkspaceHandler(&fakeAccess{}, "http://127.0.0.1:8180", &readAccessFake{}, []DisplayBinding{workspaceBinding}, 14, WithRegistration(true)); !errors.Is(err, auth.ErrUnavailable) {
		t.Fatal("missing registrar admitted")
	}
	h, err = NewWorkspaceHandler(f, "http://127.0.0.1:8180", &readAccessFake{}, []DisplayBinding{workspaceBinding}, 14, WithRegistration(true))
	if err != nil {
		t.Fatal(err)
	}
	if w := request(h, "POST", "/api/v1/auth/register", body, "", "https://evil.example"); w.Code != 403 || f.registrations != 0 {
		t.Fatal("cross origin signup")
	}
	for _, bad := range []string{
		`{"login":"new.user","password":"test-only-password-123","role":"ADMIN"}`,
		`{"login":"first","login":"second","password":"test-only-password-123"}`,
		`{"login":"new.user","password":"test-only-password-123","enabled":true}`,
		`{"login":"new.user","password":null}`, body + `{}`, `{}`, `{"login":"bad name","password":"test-only-password-123"}`,
	} {
		if w := request(h, "POST", "/api/v1/auth/register", bad, "", "http://127.0.0.1:8180"); w.Code != 400 || f.registrations != 0 {
			t.Fatal("invalid request admitted", w.Code)
		}
	}
	h, _ = NewWorkspaceHandler(f, "http://127.0.0.1:8180", &readAccessFake{}, []DisplayBinding{workspaceBinding}, 14, WithRegistration(true))
	w := request(h, "POST", "/api/v1/auth/register", body, "", "http://127.0.0.1:8180")
	if w.Code != 201 || f.registrations != 1 || len(w.Result().Cookies()) != 0 || strings.Contains(w.Body.String(), "password") || !strings.Contains(w.Body.String(), `"role":"USER"`) || !strings.Contains(w.Body.String(), `"login":"new.user"`) {
		t.Fatal("unsafe signup", w.Code)
	}
	f.registerErr = auth.ErrConflict
	if w = request(h, "POST", "/api/v1/auth/register", body, "", "http://127.0.0.1:8180"); w.Code != 409 || strings.Contains(w.Body.String(), "test-only") {
		t.Fatal("duplicate contract")
	}
	for i := 0; i < 6; i++ {
		request(h, "POST", "/api/v1/auth/register", body, "", "http://127.0.0.1:8180")
	}
	count := f.registrations
	if w = request(h, "POST", "/api/v1/auth/register", body, "", "http://127.0.0.1:8180"); w.Code != 429 || f.registrations != count {
		t.Fatal("unbounded signup")
	}
	if w = request(h, "POST", "/api/v1/auth/login", body, "", "http://127.0.0.1:8180"); w.Code != 429 {
		t.Fatal("split auth budget bypass")
	}
}
