package indexcore

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestExactResourceReadEscapingVisibilityAndIdentity(t *testing.T) {
	var mismatch atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.EscapedPath() != "/prefix/v1/resources/id%2Fone" || r.URL.Query().Get("include_removed") != "true" {
			t.Error("request changed exact identity/visibility", r.URL)
		}
		id := "id/one"
		if mismatch.Load() {
			id = "different"
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"resource_id":%q,"root_id":"root","resource_presence":"REMOVED","is_dir":false,"introduced_at_generation":1,"last_confirmed_generation":2}`, id)
	}))
	defer server.Close()
	client, _ := NewClient(server.URL + "/prefix")
	r, err := client.Resource(context.Background(), "id/one", true)
	if err != nil || r.ResourceID != "id/one" || r.Presence != ResourceRemoved {
		t.Fatal(r, err)
	}
	mismatch.Store(true)
	if _, err = client.Resource(context.Background(), "id/one", true); !errors.Is(err, ErrMalformedResponse) {
		t.Fatal("foreign identity accepted", err)
	}
}
