package productserver

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nathanxiangang-web/panta/internal/producthttp"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

func validConfig() Config {
	return Config{DatabaseURLFile: "/protected/database.url", ListenAddress: "127.0.0.1:8180", Origin: "http://127.0.0.1:8180", IndexCoreBaseURL: "http://127.0.0.1:8080", Bindings: []producthttp.DisplayBinding{{ID: storage.BindingID("97000000-0000-4000-8000-000000000001"), Label: "Test space"}}}
}
func TestConfigValidateAndProtectedInputs(t *testing.T) {
	cfg := validConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	cases := []func(*Config){func(c *Config) { c.ListenAddress = "0.0.0.0:8180" }, func(c *Config) { c.ListenAddress = "localhost:8180" }, func(c *Config) { c.ListenAddress = "127.0.0.1:0" }, func(c *Config) { c.Origin = "http://localhost:8180" }, func(c *Config) { c.Origin += "?secret=unsafe" }, func(c *Config) { c.IndexCoreBaseURL = "https://user:secret@example.com" }, func(c *Config) { c.Bindings = nil }, func(c *Config) { c.Bindings = append(c.Bindings, c.Bindings[0]) }, func(c *Config) { c.Bindings[0].Label = "bad\nlabel" }}
	for _, mutate := range cases {
		c := validConfig()
		mutate(&c)
		if c.Validate() == nil {
			t.Fatal("unsafe config admitted")
		}
	}
	if runtime.GOOS != "linux" {
		t.Skip("protected files intentionally Linux-only")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "config.json")
	data, _ := json.Marshal(cfg)
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(context.Background(), file); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(file, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(context.Background(), file); !errors.Is(err, ErrConfig) {
		t.Fatal("world-readable config admitted")
	}
	if err := os.Chmod(file, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(`{"login":"nathan","password":"test-only-password-123","role":"USER"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadProvision(context.Background(), file); err != nil {
		t.Fatal("valid provision refused", err)
	}
	for _, raw := range []string{`{"login":"nathan","password":"test-only-password-123","role":"USER","role":"ADMIN"}`, `{"login":"nathan","password":"test-only-password-123","role":"USER","cookie":"SECRET"}`, `{"login":"nathan","password":"test-only-password-123","role":"PREMIUM"}`, `{"login":"nathan","password":"short","role":"USER"}`} {
		if err := os.WriteFile(file, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadProvision(context.Background(), file); !errors.Is(err, ErrConfig) || strings.Contains(err.Error(), "SECRET") {
			t.Fatal("unsafe operator user request")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := LoadConfig(ctx, file); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled config read proceeded")
	}
}
func TestExplicitHTTPSProxyRetainsLoopbackAndRejectsInsecureOrigins(t *testing.T) {
	cfg := validConfig()
	cfg.HTTPSProxy = true
	cfg.Origin = "https://cloud.example.test"
	if cfg.Validate() != nil {
		t.Fatal("explicit HTTPS proxy refused")
	}
	for _, origin := range []string{"http://cloud.example.test", "https://", "https://user:secret@cloud.example.test", "https://cloud.example.test/path", "https://cloud.example.test?token=secret", "https://cloud.example.test#fragment"} {
		c := cfg
		c.Origin = origin
		if c.Validate() == nil {
			t.Fatal("unsafe proxy origin accepted")
		}
	}
	cfg.ListenAddress = "0.0.0.0:8180"
	if cfg.Validate() == nil {
		t.Fatal("proxy mode widened listener")
	}
	cfg = validConfig()
	cfg.Origin = "https://cloud.example.test"
	if cfg.Validate() == nil {
		t.Fatal("public origin implicitly enabled")
	}
}
func TestServeGracefulCancellation(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- serve(ctx, listener, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	}()
	client := &http.Client{Timeout: time.Second}
	response, err := client.Get("http://" + listener.Addr().String())
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	response.Body.Close()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("owned server did not stop")
	}
	if conn, err := net.DialTimeout("tcp", listener.Addr().String(), 100*time.Millisecond); err == nil {
		conn.Close()
		t.Fatal("listener survived shutdown")
	}
}
