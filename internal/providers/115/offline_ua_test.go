package p115

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/SheltonZhu/115driver/pkg/driver"
)

// Upstream #56 documents a plaintext "decode fail!" rejection for the
// default UA and recommends UA115Browser. This fixture asserts our wire
// contract; it is not evidence that the live provider accepted a task.
func TestOfflineRequestUses115BrowserAndDoesNotRetryTextRejection(t *testing.T) {
	trace := &startHTTPTrace{}
	posts := 0
	transport := startRoundTripper(func(request *http.Request) (*http.Response, error) {
		posts++
		if request.Method != http.MethodPost || request.URL.Host != "lixian.115.com" || request.URL.Path != "/lixianssp/" || request.URL.Query().Get("ac") != "add_task_urls" {
			t.Fatal("unexpected SDK request contract")
		}
		if request.Header.Get("User-Agent") != driver.UA115Browser {
			t.Fatal("offline POST must use the pinned 115 Browser identity")
		}
		if err := request.ParseForm(); err != nil || request.Form.Get("data") == "" {
			t.Fatal("encrypted form payload missing")
		}
		return &http.Response{StatusCode: http.StatusOK,
			Header: http.Header{"Content-Type": []string{"text/plain"}},
			Body:   io.NopCloser(strings.NewReader("decode fail!")), Request: request}, nil
	})
	client := newDriverClient(&http.Client{Timeout: time.Second, Transport: statusTransport{next: transport, trace: trace}})
	client.UserID = 1
	backend, _ := NewCookiedBackend(client)
	backend.trace = trace
	adapter, _ := New(Options{Backend: backend})
	result, err := adapter.StartDownload(context.Background(), validRequest())
	var diagnostic *StartDiagnostic
	if !errors.As(err, &diagnostic) || diagnostic.DiagnosticStage() != "OFFLINE_POST_OUTER_JSON" ||
		diagnostic.DiagnosticResponseMediaFamily() != "TEXT" || diagnostic.DiagnosticResponseSizeBucket() != "1-127" || diagnostic.DiagnosticOuterJSONValid() != "FALSE" {
		t.Fatal("plaintext failure must retain the closed parser diagnostic")
	}
	if posts != 1 || result.Value != "" || strings.Contains(err.Error(), "decode fail!") {
		t.Fatal("retry, invented reference, or raw response disclosure")
	}
}
