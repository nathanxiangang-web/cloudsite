package content

import (
	"context"
	"encoding/json"
	"github.com/nathanxiangang-web/panta/internal/fileaccess"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDirectLinkResolutionIsMetadataOnlyWithCallerUAAndRotatingToken(t *testing.T) {
	token := "first-test-token"
	calls := 0
	valid := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/api/fs/get" || r.Method != "POST" || r.Header.Get("Authorization") != token || r.Header.Get("User-Agent") != "Browser/2" || r.Header.Get("Cookie") != "" || r.Header.Get("Range") != "" {
			t.Error("unexpected metadata request or credential leak")
		}
		json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": map[string]any{"raw_url": "https://cdn.example.test/file?temporary=opaque", "size": 4, "is_dir": !valid}})
	}))
	defer server.Close()
	client, err := NewContentClientWithTokenSource(server.URL, func(context.Context) (string, error) { return token, nil })
	if err != nil {
		t.Fatal(err)
	}
	// Any accidental byte GET would use this transport and fail the test.
	client.stream = &http.Client{Transport: rejectStream{t}}
	for _, value := range []string{"first-test-token", "second-test-token"} {
		token = value
		target, err := client.ResolveLink(context.Background(), fileaccess.SourceRequest{Path: "/known/file", Size: 4, UserAgent: "Browser/2"})
		if err != nil || target != "https://cdn.example.test/file?temporary=opaque" {
			t.Fatal("link resolution", err)
		}
	}
	valid = false
	if _, err := client.ResolveLink(context.Background(), fileaccess.SourceRequest{Path: "/known/file", Size: 4, UserAgent: "Browser/2"}); err == nil {
		t.Fatal("directory accepted")
	}
	if calls != 3 {
		t.Fatal("unexpected byte request")
	}
	if _, err := client.ResolveLink(context.Background(), fileaccess.SourceRequest{UserAgent: "bad\r\nua"}); err == nil || calls != 3 {
		t.Fatal("invalid UA sent upstream")
	}
}

type rejectStream struct{ t *testing.T }

func (r rejectStream) RoundTrip(*http.Request) (*http.Response, error) {
	r.t.Fatal("direct resolver opened source byte stream")
	return nil, fileaccess.ErrUnavailable
}
