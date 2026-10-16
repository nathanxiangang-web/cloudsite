package acquisitionruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/nathanxiangang-web/panta/internal/platform/productserver"
	"github.com/nathanxiangang-web/panta/internal/producthttp"
)

func TestPostgresOperatorConsumerCloudCLI(t *testing.T) {
	if goruntime.GOOS != "linux" {
		t.Skip("protected files and signals require Linux")
	}
	f, manifestID, jobID := runtimeFixture(t)
	query := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("preflight/idle server contacted query API") }))
	defer query.Close()
	hint := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("preflight/idle server sent Hint") }))
	defer hint.Close()
	bootstrapFile := operatorProtectedConfig(t, f, hint.URL)
	cookieFile := filepath.Join(filepath.Dir(bootstrapFile), "secrets", "provider.cookie")
	cookie := []byte("UID=synthetic-uid;CID=synthetic-cid;SEID=synthetic-seid;KID=synthetic-kid")
	if err := os.WriteFile(cookieFile, cookie, 0600); err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	databaseFile := filepath.Join(directory, "database.url")
	if err := os.WriteFile(databaseFile, []byte(os.Getenv("PANTA_TEST_DATABASE_URL")), 0600); err != nil {
		t.Fatal(err)
	}
	// Hold the actual port during preflight: it must not try to listen or claim.
	reserved := httptest.NewUnstartedServer(nil)
	defer reserved.Close()
	address := reserved.Listener.Addr().String()
	origin := "http://" + address
	cfg := productserver.Config{DatabaseURLFile: databaseFile, ListenAddress: address, Origin: origin, IndexCoreBaseURL: query.URL, Bindings: []producthttp.DisplayBinding{{ID: f.bindingID, Label: "Controlled fixture"}}, CloudDownload: &productserver.CloudDownloadConfig{Target: producthttp.CloudDownloadTarget{BindingID: f.bindingID, TargetPath: "/downloads/item", Label: "Test downloads"}, BootstrapFile: bootstrapFile}}
	configFile := filepath.Join(directory, "product.json")
	save := func() {
		t.Helper()
		data, _ := json.Marshal(cfg)
		if err := os.WriteFile(configFile, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	save()
	binary := filepath.Join(directory, "panta")
	build := exec.Command(filepath.Join(goruntime.GOROOT(), "bin", "go"), "build", "-o", binary, "../../../cmd/panta")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	env := append(os.Environ(), "PANTA_ACQUISITION_WORKER_ENABLED=true", "PANTA_PRODUCT_CONFIG_FILE="+configFile)
	execute := func(action string, success bool) {
		t.Helper()
		ctx, cancel := context.WithTimeout(f.ctx, 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, action)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if (err == nil) != success || bytes.Contains(out, cookie) || bytes.Contains(out, []byte("postgres://")) || bytes.Contains(out, []byte(runtimeHintToken)) {
			t.Fatalf("unexpected CLI result success=%t err=%v", success, err)
		}
	}
	execute(productserver.CloudPreflightAction, true)
	execute(productserver.CloudAction, false) // occupied port: no Worker may run.
	if job := readJob(t, f.ctx, f.pool, jobID); job.ClaimAttempts != 0 {
		t.Fatal("preflight/listen failure claimed Job")
	}
	if err := os.Remove(cookieFile); err != nil {
		t.Fatal(err)
	}
	execute(productserver.CloudPreflightAction, false)
	if err := os.WriteFile(cookieFile, cookie, 0600); err != nil {
		t.Fatal(err)
	}
	cfg.CloudDownload.BootstrapFile += "-missing"
	save()
	execute(productserver.CloudAction, false)
	cfg.CloudDownload.BootstrapFile = bootstrapFile
	save()
	// Readonly mode rejects acquisition configuration even when worker flag off.
	disabled := append(append([]string{}, env...), "PANTA_ACQUISITION_WORKER_ENABLED=false")
	cmd := exec.Command(binary, productserver.ServeAction)
	cmd.Env = disabled
	if _, err := cmd.CombinedOutput(); err == nil {
		t.Fatal("readonly silently enabled cloud config")
	}
	if job := readJob(t, f.ctx, f.pool, jobID); job.ClaimAttempts != 0 {
		t.Fatal("startup failures claimed Job")
	}
	// Start only on an EMPTY disposable queue. The real native adapter can parse
	// synthetic credentials but is never allowed to execute provider requests.
	if _, err := f.pool.Exec(f.ctx, `DELETE FROM acquisition_manifests WHERE manifest_id=$1`, string(manifestID)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(f.ctx, `DELETE FROM jobs WHERE job_id=$1`, string(jobID)); err != nil {
		t.Fatal(err)
	}
	reserved.Listener.Close()
	ctx, cancel := context.WithTimeout(f.ctx, 10*time.Second)
	defer cancel()
	cmd = exec.CommandContext(ctx, binary, productserver.CloudAction)
	cmd.Env = env
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	joined := false
	t.Cleanup(func() {
		if !joined {
			_ = cmd.Process.Signal(syscall.SIGTERM)
			_ = cmd.Wait()
		}
	})
	ready := false
	client := &http.Client{Timeout: time.Second}
	for deadline := time.Now().Add(4 * time.Second); time.Now().Before(deadline); {
		response, err := client.Get(origin + "/downloads/")
		if err == nil {
			response.Body.Close()
			if response.StatusCode == 200 {
				ready = true
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !ready {
		t.Fatal("cloud listener unavailable")
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	err := cmd.Wait()
	joined = true
	if err != nil || strings.Contains(output.String(), runtimeHintToken) {
		t.Fatal("unsafe cloud shutdown", err)
	}
	var tasks int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM acquisition_provider_tasks`).Scan(&tasks); err != nil || tasks != 0 {
		t.Fatal("idle runtime touched provider")
	}
	// Startup must not upgrade/repair a partial migration history.
	if _, err := f.pool.Exec(f.ctx, `DELETE FROM schema_migrations WHERE version=15`); err != nil {
		t.Fatal(err)
	}
	execute(productserver.CloudPreflightAction, false)
}
