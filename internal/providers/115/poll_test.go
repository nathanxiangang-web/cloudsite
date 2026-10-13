package p115

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/nathanxiangang-web/panta/internal/providers/contracts"
)

func TestPinnedOfflinePollingUsesGETWithoutRetryOrDisclosure(t *testing.T) {
	for _, test := range []struct {
		name, body string
		status     int
		category   contracts.PollCategory
		retry      bool
	}{
		{"bad JSON", "PRIVATE-BODY", 200, contracts.PollDecode, false},
		{"HTTP temporary", "PRIVATE-BODY", 503, contracts.PollHTTP, true},
		{"HTTP rate limit", "PRIVATE-BODY", 429, contracts.PollHTTP, true},
		{"HTTP unauthorized", "PRIVATE-BODY", 401, contracts.PollHTTP, false},
		{"provider rejects", `{"state":false,"errno":990001}`, 200, contracts.PollUnknown, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			client := newDriverClient(&http.Client{Timeout: time.Second, Transport: startRoundTripper(func(request *http.Request) (*http.Response, error) {
				calls++
				if request.Method != http.MethodGet {
					t.Fatal("poll made a write-method request")
				}
				return &http.Response{StatusCode: test.status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(test.body)), Request: request}, nil
			})})
			backend, _ := NewCookiedBackend(readOnlyOfflineClient{client})
			adapter, _ := New(Options{Backend: backend})
			_, err := adapter.DownloadStatus(context.Background(), contracts.TaskReference{Value: "PRIVATE-REFERENCE"})
			var diagnostic *contracts.PollError
			if !errors.As(err, &diagnostic) || diagnostic.Category != test.category || diagnostic.Retryable != test.retry || calls != 1 {
				t.Fatalf("category=%v calls=%d", err, calls)
			}
			if strings.Contains(err.Error(), "PRIVATE") {
				t.Fatal("diagnostic disclosure")
			}
		})
	}
}

func TestPollDiagnosticPreservesTypedCause(t *testing.T) {
	cause := &net.DNSError{Err: "PRIVATE", Name: "PRIVATE", IsTimeout: true}
	diagnostic := pollDiagnostic(cause, time.Second)
	var network net.Error
	if diagnostic.Category != contracts.PollTimeout || !diagnostic.Retryable || !errors.As(diagnostic, &network) || !errors.Is(diagnostic, cause) || strings.Contains(diagnostic.Error(), "PRIVATE") {
		t.Fatal("lost typed cause or redaction")
	}
	if pollDiagnostic(context.Canceled, 0).Retryable {
		t.Fatal("cancellation is not retryable")
	}
}
