package producthttp

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/product"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

var workspaceBinding = DisplayBinding{ID: storage.BindingID("97000000-0000-4000-8000-000000000001"), Label: "Test workspace"}

func TestWorkspaceAssetsIdentityMetadataAndAdmission(t *testing.T) {
	access := &fakeAccess{p: goodPrincipal()}
	reads := &readAccessFake{}
	h, err := NewWorkspaceHandler(access, "http://127.0.0.1:8180", reads, []DisplayBinding{workspaceBinding}, 13)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path, token string
		want        int
	}{{"/", "", 200}, {"/assets/app.css", "", 200}, {"/assets/app.js", "", 200}, {"/assets/../workspace.go", "", 404}, {"/api/v1/workspace", "", 401}, {"/api/v1/workspace", testToken, 200}, {"/api/v1/workspace?root_id=secret", testToken, 400}} {
		// Path cleanup is Go mux behavior. The traversal must not produce source.
		w := request(h, "GET", tc.path, "", tc.token, "")
		if tc.want == 404 && w.Code >= 300 && w.Code < 400 {
			if w.Header().Get("Location") != "/workspace.go" {
				t.Fatal("unsafe traversal redirect")
			}
			w = request(h, "GET", "/workspace.go", "", tc.token, "")
		}
		if w.Code != tc.want {
			t.Fatal(tc.path, w.Code)
		}
		if !strings.Contains(w.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") || w.Header().Get("X-Frame-Options") != "DENY" || w.Header().Get("Referrer-Policy") != "no-referrer" || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("missing security headers")
		}
		if tc.path == "/api/v1/workspace" && tc.want == 200 {
			body := w.Body.String()
			if !strings.Contains(body, `"mode":"READ_ONLY"`) || strings.Contains(body, "root_id") || strings.Contains(body, "provider") || strings.Contains(body, "database_url") {
				t.Fatal("unsafe workspace config", body)
			}
		}
	}
	now := time.Now()
	h.gate.now = func() time.Time { return now }
	body := `{"login":"nathan","password":"test-only-password-123"}`
	for n := 0; n < 8; n++ {
		if w := request(h, "POST", "/api/v1/auth/login", body, "", "http://127.0.0.1:8180"); w.Code != 200 {
			t.Fatal("admission refused early", n, w.Code)
		}
	}
	calls := access.calls
	w := request(h, "POST", "/api/v1/auth/login", body, "", "http://127.0.0.1:8180")
	if w.Code != 429 || access.calls != calls || w.Header().Get("Retry-After") != "60" {
		t.Fatal("login budget bypass")
	}
	now = now.Add(time.Minute)
	if w = request(h, "POST", "/api/v1/auth/login", body, "", "http://127.0.0.1:8180"); w.Code != 200 {
		t.Fatal("budget did not reset")
	}
	h.Wait()
	w = request(h, "GET", "/api/v1/workspace", "", testToken, "")
	if w.Code != 503 {
		t.Fatal("late request admitted after stop")
	}
	var nilReads *product.ReadService
	if _, err = NewWorkspaceHandler(access, "http://127.0.0.1:8180", nilReads, []DisplayBinding{workspaceBinding}, 13); err == nil {
		t.Fatal("nil read fallback")
	}
}

type blockedLogin struct {
	entered chan struct{}
	release chan struct{}
}

func (b *blockedLogin) Login(ctx context.Context, _, _ string) (auth.LoginResult, error) {
	b.entered <- struct{}{}
	select {
	case <-b.release:
		return auth.LoginResult{Principal: goodPrincipal(), Token: testToken, ExpiresAt: time.Now().Add(time.Hour)}, nil
	case <-ctx.Done():
		return auth.LoginResult{}, ctx.Err()
	}
}
func (b *blockedLogin) Authenticate(context.Context, string) (auth.Principal, error) {
	return goodPrincipal(), nil
}
func (b *blockedLogin) Logout(context.Context, string) error { return nil }
func TestWorkspaceBoundedConcurrentLoginAndWait(t *testing.T) {
	access := &blockedLogin{entered: make(chan struct{}, 2), release: make(chan struct{})}
	h, _ := NewWorkspaceHandler(access, "http://127.0.0.1:8180", &readAccessFake{}, []DisplayBinding{workspaceBinding}, 13)
	body := `{"login":"nathan","password":"test-only-password-123"}`
	done := make(chan int, 2)
	for i := 0; i < 2; i++ {
		go func() { done <- request(h, "POST", "/api/v1/auth/login", body, "", "http://127.0.0.1:8180").Code }()
	}
	for i := 0; i < 2; i++ {
		select {
		case <-access.entered:
		case <-time.After(time.Second):
			t.Fatal("login did not enter")
		}
	}
	w := request(h, "POST", "/api/v1/auth/login", body, "", "http://127.0.0.1:8180")
	if w.Code != 429 {
		t.Fatal("third concurrent bcrypt admitted")
	}
	stopped := make(chan struct{})
	go func() { h.Wait(); close(stopped) }()
	select {
	case <-stopped:
		t.Fatal("teardown did not join active login")
	case <-time.After(20 * time.Millisecond):
	}
	close(access.release)
	for i := 0; i < 2; i++ {
		if <-done != 200 {
			t.Fatal("active login lost")
		}
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("teardown hung")
	}
}
func TestWorkspaceStaticJSContainsNoTokenStorageOrHTMLInjection(t *testing.T) {
	data, err := workspaceFiles.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, forbidden := range []string{"localStorage", "sessionStorage", "innerHTML", "document.cookie", "https://"} {
		if strings.Contains(source, forbidden) {
			t.Fatal("unsafe UI primitive", forbidden)
		}
	}
	data, _ = workspaceFiles.ReadFile("web/index.html")
	if !strings.Contains(string(data), "系统与诊断") || !strings.Contains(string(data), "nav-users") || strings.Contains(string(data), "nav-settings") || strings.Contains(string(data), "file-primary-action") {
		t.Fatal("admin information architecture contains an unfinished affordance")
	}
}
