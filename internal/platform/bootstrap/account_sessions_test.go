package bootstrap

import (
	"context"
	"github.com/nathanxiangang-web/panta/internal/providers/contracts"
	"os"
	"path/filepath"
	"testing"
)

func TestAccountSessionBootstrapIsLazyProtectedAndDoesNotRequireHint(t *testing.T) {
	path, mount := protectedFixture(t)
	if e := os.Remove(filepath.Join(mount, "hint.token")); e != nil {
		t.Fatal(e)
	}
	resolver, sessions, e := LoadAccountSessions(context.Background(), path)
	if e != nil || len(sessions) != 1 {
		t.Fatal(e)
	}
	if _, e = resolver.ResolveSecret(context.Background(), contracts.CredentialRef("not-approved")); e == nil {
		t.Fatal("unbound secret")
	}
	if e = os.Chmod(filepath.Join(mount, "connection.cookie"), 0644); e != nil {
		t.Fatal(e)
	}
	if _, e = resolver.ResolveSecret(context.Background(), contracts.CredentialRef("opaque-ref")); e == nil {
		t.Fatal("unsafe secret")
	}
	if e = os.Chmod(path, 0644); e != nil {
		t.Fatal(e)
	}
	if _, _, e = LoadAccountSessions(context.Background(), path); e == nil {
		t.Fatal("unprotected config")
	}
}
