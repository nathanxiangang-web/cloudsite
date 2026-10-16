package productserver

import (
	"context"
	"errors"
	"github.com/nathanxiangang-web/panta/internal/producthttp"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

type workerFake struct {
	calls            atomic.Int32
	started, stopped chan struct{}
	finish           <-chan struct{}
	failure          error
}

func (w *workerFake) Run(ctx context.Context) error {
	w.calls.Add(1)
	close(w.started)
	defer close(w.stopped)
	select {
	case <-ctx.Done():
		return nil
	case <-w.finish:
		return w.failure
	}
}
func TestCloudLifecycleJoinsOnCancellationAndWorkerFailure(t *testing.T) {
	for _, fatal := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "worker-exit"}[fatal], func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			finish := make(chan struct{})
			worker := &workerFake{started: make(chan struct{}), stopped: make(chan struct{}), finish: finish}
			done := make(chan error, 1)
			go func() {
				done <- serveCloud(ctx, listener, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }), worker)
			}()
			<-worker.started
			client := &http.Client{Timeout: time.Second}
			response, err := client.Get("http://" + listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if fatal {
				close(finish)
			} else {
				cancel()
			}
			select {
			case err := <-done:
				if fatal && !errors.Is(err, ErrStartup) || !fatal && err != nil {
					t.Fatal("wrong shutdown", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("lifecycle did not join")
			}
			select {
			case <-worker.stopped:
			default:
				t.Fatal("Worker still running")
			}
			if worker.calls.Load() != 1 {
				t.Fatal("multiple worker loops")
			}
			if connection, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second); err == nil {
				connection.Close()
				t.Fatal("listener survived Worker stop")
			}
		})
	}
}
func TestCloudLifecycleStopsWorkerAfterListenerFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	worker := &workerFake{started: make(chan struct{}), stopped: make(chan struct{}), finish: make(chan struct{})}
	done := make(chan error, 1)
	go func() { done <- serveCloud(ctx, listener, http.NotFoundHandler(), worker) }()
	<-worker.started
	listener.Close()
	select {
	case err := <-done:
		if !errors.Is(err, ErrStartup) {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("HTTP failure did not cancel Worker")
	}
	select {
	case <-worker.stopped:
	default:
		t.Fatal("Worker orphaned")
	}
}
func TestCloudConfigIsOptInAndFixedTarget(t *testing.T) {
	cfg := validConfig()
	cfg.CloudDownload = &CloudDownloadConfig{Target: producthttp.CloudDownloadTarget{BindingID: cfg.Bindings[0].ID, TargetPath: "/downloads", Label: "Downloads"}, BootstrapFile: "/protected/bootstrap.json"}
	if cfg.Validate() != nil {
		t.Fatal("valid cloud config")
	}
	for _, change := range []func(*CloudDownloadConfig){func(c *CloudDownloadConfig) { c.Target.TargetPath = "/a//b" }, func(c *CloudDownloadConfig) { c.Target.Label = "" }, func(c *CloudDownloadConfig) { c.BootstrapFile = "" }, func(c *CloudDownloadConfig) { c.Target.BindingID = "97000000-0000-4000-8000-000000000099" }} {
		copy := cfg
		setting := *cfg.CloudDownload
		copy.CloudDownload = &setting
		change(&setting)
		if copy.Validate() == nil {
			t.Fatal("unsafe cloud config")
		}
	}
	if err := Run(context.Background(), CloudAction, "/does/not/exist", ""); err == nil {
		t.Fatal("unconfigured cloud action started")
	}
	if err := Run(context.Background(), CloudPreflightAction, "/does/not/exist", ""); err == nil {
		t.Fatal("unconfigured cloud preflight started")
	}
}
