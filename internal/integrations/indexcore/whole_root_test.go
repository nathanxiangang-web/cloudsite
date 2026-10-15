package indexcore

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWholeRootBrowseUsesExistingQ6AndRejectsMixedHierarchy(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/v1/roots/root-one/active" || r.URL.Query().Get("parent_id") != "" || r.URL.Query().Get("limit") != "50" {
			t.Error("wrong Q6 coordinates", r.URL.Path, r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items":[],"next_cursor":""}`))
	}))
	defer s.Close()
	client, _ := NewClient(s.URL)
	if _, err := client.Browse(context.Background(), BrowseRequest{RootID: "root-one", WholeRoot: true, Limit: 50}); err != nil {
		t.Fatal(err)
	}
	parent := "parent"
	if _, err := client.Browse(context.Background(), BrowseRequest{RootID: "root-one", WholeRoot: true, ParentResourceID: &parent}); err == nil || calls != 1 {
		t.Fatal("mixed Q4/Q6 accepted")
	}
	if _, err := client.Browse(context.Background(), BrowseRequest{RootID: "root-one", WholeRoot: true, ReadVisibility: ReadVisibility{IncludeRemoved: true}}); err == nil || calls != 1 {
		t.Fatal("active visibility override")
	}
}
