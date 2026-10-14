package acquisitionruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/nathanxiangang-web/panta/internal/platform/productserver"
	"github.com/nathanxiangang-web/panta/internal/producthttp"
	"github.com/nathanxiangang-web/panta/internal/store/postgres"
)

func TestPostgresOperatorProductReadOnlyCLI(t *testing.T) {
	if goruntime.GOOS != "linux" {
		t.Skip("protected config is Linux-only")
	}
	fixture, _, jobID := runtimeFixture(t)
	before := readJob(t, fixture.ctx, fixture.pool, jobID)
	var queryCalls atomic.Int64
	read := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queryCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if r.Method != "GET" {
			t.Error("readonly process used a mutation")
			http.Error(w, "bad", 405)
			return
		}
		if r.URL.Path != "/v1/roots/canonical-root/resources" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `{"items":[{"resource_id":"ui-fixture-resource","root_id":"canonical-root","canonical_path":"/README-ui-fixture.md","parent_resource_id":null,"name":"README-ui-fixture.md","is_dir":false,"size":42,"mtime":"2026-09-28T01:02:03Z","resource_presence":"PRESENT","introduced_at_generation":1,"last_confirmed_generation":1}],"next_cursor":""}`)
	}))
	defer read.Close()
	private := t.TempDir()
	if err := os.Chmod(private, 0700); err != nil {
		t.Fatal(err)
	}
	database := filepath.Join(private, "database.url")
	if err := os.WriteFile(database, []byte(os.Getenv("PANTA_TEST_DATABASE_URL")), 0600); err != nil {
		t.Fatal(err)
	}
	reservation, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := reservation.Addr().String()
	reservation.Close()
	origin := "http://" + address
	cfg := productserver.Config{DatabaseURLFile: database, ListenAddress: address, Origin: origin, IndexCoreBaseURL: read.URL, Bindings: []producthttp.DisplayBinding{{ID: fixture.bindingID, Label: "CONTROLLED TEST FIXTURE - not real 115"}}}
	configFile := filepath.Join(private, "product.json")
	data, _ := json.Marshal(cfg)
	if err = os.WriteFile(configFile, data, 0600); err != nil {
		t.Fatal(err)
	}
	userFile := filepath.Join(private, "user.json")
	if err = os.WriteFile(userFile, []byte(`{"login":"ui-test","password":"test-only-password-123","role":"USER"}`), 0600); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(private, "panta")
	build := exec.Command(filepath.Join(goruntime.GOROOT(), "bin", "go"), "build", "-o", binary, "../../../cmd/panta")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build product CLI: %v %s", err, output)
	}
	env := append(os.Environ(), "PANTA_ACQUISITION_WORKER_ENABLED=false", "PANTA_PRODUCT_CONFIG_FILE="+configFile, "PANTA_PRODUCT_USER_FILE="+userFile)
	provision := exec.Command(binary, productserver.ProvisionAction)
	provision.Env = env
	if output, err := provision.CombinedOutput(); err != nil {
		t.Fatalf("provision failed: %v %s", err, output)
	}
	duplicate := exec.Command(binary, productserver.ProvisionAction)
	duplicate.Env = env
	if output, err := duplicate.CombinedOutput(); err == nil || strings.Contains(string(output), "test-only-password") {
		t.Fatal("duplicate silently reset or leaked password")
	}
	if queryCalls.Load() != 0 {
		t.Fatal("provision called IndexCore")
	}
	processCtx, cancel := context.WithTimeout(fixture.ctx, 20*time.Second)
	defer cancel()
	command := exec.CommandContext(processCtx, binary, productserver.ServeAction)
	command.Env = env
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	stopped := false
	t.Cleanup(func() {
		if !stopped {
			_ = command.Process.Signal(syscall.SIGTERM)
			_ = command.Wait()
		}
	})
	client := &http.Client{Timeout: time.Second}
	ready := false
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		response, err := client.Get(origin + "/")
		if err == nil {
			response.Body.Close()
			if response.StatusCode == 200 {
				ready = true
				break
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !ready {
		t.Fatal("product listener never became ready")
	}
	request := func(method, path, body string, cookie *http.Cookie) (int, []byte, *http.Cookie) {
		t.Helper()
		r, _ := http.NewRequest(method, origin+path, strings.NewReader(body))
		if method == "POST" {
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Origin", origin)
		}
		if cookie != nil {
			r.AddCookie(cookie)
		}
		response, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		bodyBytes, _ := io.ReadAll(response.Body)
		var resultCookie *http.Cookie
		if cookies := response.Cookies(); len(cookies) > 0 {
			resultCookie = cookies[0]
		}
		return response.StatusCode, bodyBytes, resultCookie
	}
	if code, _, _ := request("GET", "/api/v1/tasks", "", nil); code != 401 {
		t.Fatal("unverified reads")
	}
	code, body, cookie := request("POST", "/api/v1/auth/login", `{"login":"ui-test","password":"test-only-password-123"}`, nil)
	if code != 200 || cookie == nil || strings.Contains(string(body), "password") {
		t.Fatal("login failed", code, string(body))
	}
	if code, body, _ = request("GET", "/api/v1/workspace", "", cookie); code != 200 || !bytes.Contains(body, []byte("READ_ONLY")) {
		t.Fatal("workspace missing")
	}
	if code, body, _ = request("GET", "/api/v1/resources?binding_id="+string(fixture.bindingID), "", cookie); code != 200 || !bytes.Contains(body, []byte("README-ui-fixture.md")) {
		t.Fatal("real adapter read failed", code, string(body))
	}
	if code, body, _ = request("GET", "/api/v1/tasks", "", cookie); code != 200 || bytes.Contains(body, []byte(string(jobID))) {
		t.Fatal("legacy owner was adopted")
	}
	if code, _, _ = request("POST", "/api/v1/auth/logout", "", cookie); code != 204 {
		t.Fatal("logout failed")
	}
	if code, _, _ = request("GET", "/api/v1/tasks", "", cookie); code != 401 {
		t.Fatal("logout did not revoke")
	}
	if err = command.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if err = command.Wait(); err != nil {
		t.Fatal("unclean shutdown", err)
	}
	stopped = true
	if strings.Contains(output.String(), "test-only-password") || strings.Contains(output.String(), "postgres://") {
		t.Fatal("secret output")
	}
	after := readJob(t, fixture.ctx, fixture.pool, jobID)
	if after.State != before.State || after.ClaimAttempts != before.ClaimAttempts {
		t.Fatal("readonly server claimed a job")
	}
	var tasks, copies int
	if err = fixture.pool.QueryRow(fixture.ctx, `SELECT count(*) FROM acquisition_provider_tasks`).Scan(&tasks); err != nil || tasks != 0 {
		t.Fatal("provider action persisted")
	}
	if err = fixture.pool.QueryRow(fixture.ctx, `SELECT count(*) FROM copies`).Scan(&copies); err != nil || copies != 0 {
		t.Fatal("readonly path wrote Copy")
	}
	// Migration metadata still checksum guarded; no startup auto-repair of v12.
	if _, err = fixture.pool.Exec(fixture.ctx, `DELETE FROM schema_migrations WHERE version=$1`, postgres.CurrentSchemaVersion); err != nil {
		t.Fatal(err)
	}
	denied := exec.Command(binary, productserver.ServeAction)
	denied.Env = env
	if _, err = denied.CombinedOutput(); err == nil {
		t.Fatal("unmigrated history admitted")
	}
}
