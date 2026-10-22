package adminindex_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nathanxiangang-web/panta/internal/adminobserve"
	"github.com/nathanxiangang-web/panta/internal/integrations/indexcore"
	"github.com/nathanxiangang-web/panta/internal/platform/adminindex"
)

func TestAdminQ9ExistingClientContractAndRedaction(t *testing.T) {
	for _, tc := range []struct {
		body   string
		status int
		want   error
	}{
		{`{"root_id":"private-root","lifecycle_state":"NEW","current_generation":0,"last_applied_admission_seq":null}`, 200, nil},
		{`{"root_id":"private-root","lifecycle_state":"ACTIVE","current_generation":8,"last_applied_admission_seq":0}`, 200, nil},
		{`{"root_id":"private-root","lifecycle_state":"DEPRECATED","current_generation":3,"last_applied_admission_seq":7}`, 200, nil},
		{`{"root_id":"private-root","lifecycle_state":"DELETED","current_generation":3,"last_applied_admission_seq":7}`, 200, nil},
		{`{"error":"not_found","message":"SECRET private-url"}`, 404, adminobserve.ErrRootNotFound},
		{`{"error":"not_ready","message":"SECRET"}`, 503, adminobserve.ErrNotReady},
		{`{"root_id":"private-root","lifecycle_state":"BOGUS","current_generation":0}`, 200, adminobserve.ErrUnavailable},
		{`{"root_id":"private-root"}`, 200, adminobserve.ErrUnavailable}, {`SECRET`, 502, adminobserve.ErrUnavailable},
	} {
		t.Run(fmt.Sprint(tc.status, tc.body), func(t *testing.T) {
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != "GET" || r.URL.Path != "/v1/roots/private-root/status" || r.URL.Query().Get("include_deleted_root") != "true" || r.URL.Query().Get("include_deprecated_root") != "true" {
					t.Error("not single Q9 GET", r.Method, r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			client, _ := indexcore.NewClient(server.URL)
			reader, _ := adminindex.New(client)
			s, e := reader.ReadRoot(context.Background(), "private-root")
			if !errors.Is(e, tc.want) || calls.Load() != 1 {
				t.Fatal(s, e, calls.Load())
			}
			if e != nil && strings.Contains(e.Error(), "SECRET") {
				t.Fatal("raw error")
			}
		})
	}
}
func TestAdminQ9CancellationAndNoAutomaticRetries(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); <-r.Context().Done() }))
	defer server.Close()
	client, _ := indexcore.NewClient(server.URL, indexcore.WithTimeout(time.Second))
	reader, _ := adminindex.New(client)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, e := reader.ReadRoot(ctx, "root")
	if !errors.Is(e, context.DeadlineExceeded) || calls.Load() != 1 {
		t.Fatal(e, calls.Load())
	}
	if _, e = adminindex.New(nil); e == nil {
		t.Fatal("nil port")
	}
}
