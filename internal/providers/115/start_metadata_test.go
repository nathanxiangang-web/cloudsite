package p115

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/nathanxiangang-web/panta/internal/providers/contracts"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// All SDK traffic is redirected to a loopback fake; there is no live fallback.
func TestOfflineResponseMetadata(t *testing.T) {
	const planted = "FAKE-COOKIE-URL-HEADER-TOKEN"
	for _, test := range []struct {
		name, media, body, family, bucket, valid, shape, stage string
		status                                                 int
		incomplete                                             bool
	}{
		{"text", "text/plain; charset=utf-8", planted, "TEXT", "1-127", "FALSE", "TEXT", "OFFLINE_POST_OUTER_JSON", 200, false},
		{"html", "text/html", "<html>" + planted + "</html>", "HTML", "1-127", "FALSE", "HTML", "OFFLINE_POST_OUTER_JSON", 200, false},
		{"misleading JSON header", "application/json", planted, "JSON", "1-127", "FALSE", "TEXT", "OFFLINE_POST_OUTER_JSON", 200, false},
		{"valid JSON wrong schema", "application/json", `{"state":"` + planted + `"}`, "JSON", "1-127", "TRUE", "OBJECT", "OFFLINE_POST_OUTER_JSON", 200, false},
		{"malformed object", "application/json", `{"state":`, "JSON", "1-127", "FALSE", "OBJECT", "OFFLINE_POST_OUTER_JSON", 200, false},
		{"unauthorized text", "text/plain", planted, "TEXT", "1-127", "FALSE", "TEXT", "UNKNOWN", 401, false},
		{"forbidden text", "text/plain", planted, "TEXT", "1-127", "FALSE", "TEXT", "UNKNOWN", 403, false},
		{"incomplete", "text/plain", planted, "TEXT", "INCOMPLETE", "UNKNOWN", "UNKNOWN", "OFFLINE_POST_TRANSPORT", 200, true},
		{"oversize", "text/plain", strings.Repeat("x", startResponseLimit+1), "TEXT", "OVERSIZE", "UNKNOWN", "UNKNOWN", "UNKNOWN", 200, false},
		{"missing media", "", planted, "MISSING", "1-127", "FALSE", "TEXT", "OFFLINE_POST_OUTER_JSON", 200, false},
		{"arbitrary media", "application/" + planted, planted, "OTHER", "1-127", "FALSE", "TEXT", "OFFLINE_POST_OUTER_JSON", 200, false},
		{"128 bytes", "text/plain", strings.Repeat("x", 128), "TEXT", "128-1023", "FALSE", "TEXT", "OFFLINE_POST_OUTER_JSON", 200, false},
		{"1024 bytes", "text/plain", strings.Repeat("x", 1024), "TEXT", "1024-4095", "FALSE", "TEXT", "OFFLINE_POST_OUTER_JSON", 200, false},
		{"4096 bytes", "text/plain", strings.Repeat("x", 4096), "TEXT", "4K-64K", "FALSE", "TEXT", "OFFLINE_POST_OUTER_JSON", 200, false},
		{"cap bytes", "text/plain", strings.Repeat("x", startResponseLimit), "TEXT", "4K-64K", "FALSE", "TEXT", "OFFLINE_POST_OUTER_JSON", 200, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != http.MethodPost || r.URL.Path != "/lixianssp/" || r.URL.Query().Get("ac") != "add_task_urls" {
					t.Error("SDK request contract changed")
				}
				w.Header()["Content-Type"] = nil // prevent automatic sniffing
				if test.media != "" {
					w.Header().Set("Content-Type", test.media)
				}
				w.Header().Set("X-Private-Test", planted)
				w.Header().Set("Set-Cookie", "private="+planted)
				if test.incomplete {
					w.Header().Set("Content-Length", fmt.Sprint(len(test.body)+20))
				}
				w.WriteHeader(test.status)
				_, _ = io.WriteString(w, test.body)
			}))
			defer server.Close()
			fakeURL, _ := url.Parse(server.URL)
			trace := &startHTTPTrace{}
			transport := startRoundTripper(func(r *http.Request) (*http.Response, error) {
				if r.URL.Scheme != "https" || r.URL.Host != "lixian.115.com" {
					return nil, errors.New("unexpected SDK request")
				}
				copy := r.Clone(r.Context())
				copy.URL = new(url.URL)
				*copy.URL = *r.URL
				copy.URL.Scheme, copy.URL.Host = fakeURL.Scheme, fakeURL.Host
				return server.Client().Transport.RoundTrip(copy)
			})
			client := newDriverClient(&http.Client{Timeout: time.Second, Transport: statusTransport{next: transport, trace: trace}})
			client.UserID = 1
			backend, _ := NewCookiedBackend(client)
			backend.trace = trace
			adapter, _ := New(Options{Backend: backend})
			result, err := adapter.StartDownload(context.Background(), validRequest())
			var d *StartDiagnostic
			if !errors.As(err, &d) || !errors.Is(err, ErrBackendStart) || calls.Load() != 1 {
				t.Fatalf("typed error=%v calls=%d", err, calls.Load())
			}
			if result != (contracts.TaskReference{}) {
				t.Fatalf("false task reference: %#v", result)
			}
			if d.DiagnosticResponseMediaFamily() != test.family || d.DiagnosticResponseSizeBucket() != test.bucket || d.DiagnosticOuterJSONValid() != test.valid || d.DiagnosticResponseShape() != test.shape || d.DiagnosticStage() != test.stage {
				t.Fatalf("metadata: family=%s size=%s valid=%s shape=%s stage=%s", d.DiagnosticResponseMediaFamily(), d.DiagnosticResponseSizeBucket(), d.DiagnosticOuterJSONValid(), d.DiagnosticResponseShape(), d.DiagnosticStage())
			}
			output := fmt.Sprintf("%v %s %s %s %s %s", err, d.DiagnosticResponseMediaFamily(), d.DiagnosticResponseSizeBucket(), d.DiagnosticOuterJSONValid(), d.DiagnosticResponseShape(), d.DiagnosticStage())
			var captured bytes.Buffer
			log.New(&captured, "", 0).Print(output)
			if strings.Contains(captured.String(), planted) {
				t.Fatal("private response content escaped diagnostic log")
			}
		})
	}
}

func TestResponseMetadataClosedBoundaries(t *testing.T) {
	for size, want := range map[int]string{0: "0", 1: "1-127", 127: "1-127", 128: "128-1023", 1023: "128-1023", 1024: "1024-4095", 4095: "1024-4095", 4096: "4K-64K", 65536: "4K-64K"} {
		if responseSizeBucket(size) != want {
			t.Fatal("overlapping size buckets")
		}
	}
	for _, operation := range []string{"USER_INFO_GET", "UNKNOWN"} {
		trace := &startHTTPTrace{}
		trace.reset()
		trace.operation = operation
		body := &tracedBody{ReadCloser: io.NopCloser(strings.NewReader(`{}`)), trace: trace, mediaFamily: "JSON"}
		_, _ = io.ReadAll(body)
		_ = body.Close()
		if trace.mediaFamily != "UNKNOWN" || trace.sizeBucket != "UNKNOWN" || trace.jsonValid != "UNKNOWN" || body.buffer != nil {
			t.Fatal("non-offline metadata or raw buffer retained")
		}
	}
}
