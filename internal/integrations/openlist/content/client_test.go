package content

import (
	"context"
	"encoding/json"
	"github.com/nathanxiangang-web/panta/internal/fileaccess"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func TestContentProtocolHeadersRangesAndRedaction(t *testing.T) {
	var base, target string
	data := "0123456789"
	badRange := false
	contentCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/fs/get" {
			if r.Header.Get("Authorization") != "test-only-SECRET" || r.Header.Get("User-Agent") != contentUA {
				t.Error("protected API auth missing")
			}
			json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": map[string]any{"raw_url": target, "is_dir": false, "size": len(data)}})
			return
		}
		contentCalls++
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" || r.Header.Get("User-Agent") != contentUA {
			t.Error("content credential leak")
		}
		start, end, partial, _ := fileaccess.ByteRange(r.Header.Get("Range"), int64(len(data)))
		body := data
		if partial {
			body = data[start : end+1]
			if !badRange {
				w.Header().Set("Content-Range", "bytes "+strconv.FormatInt(start, 10)+"-"+strconv.FormatInt(end, 10)+"/10")
			}
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		if partial {
			w.WriteHeader(206)
		}
		if r.Method != "HEAD" {
			io.WriteString(w, body)
		}
	}))
	defer server.Close()
	base = server.URL
	target = base + "/p/file?sign=test-only-value"
	client, _ := NewContentClient(base, "test-only-SECRET")
	for _, r := range []fileaccess.SourceRequest{{Path: "/file", Size: 10}, {Path: "/file", Size: 10, Range: "bytes=2-4"}, {Path: "/file", Size: 10, Head: true}} {
		content, err := client.Open(context.Background(), r)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(content.Body)
		content.Body.Close()
		if r.Head && len(raw) != 0 || r.Range != "" && string(raw) != "234" {
			t.Fatal(string(raw))
		}
	}
	before := contentCalls
	for _, bad := range []string{"http://127.0.0.1:1/p/x", "http://169.254.169.254/latest/meta-data", "file:///secret", "http://user:pass@public.example/x", base + "/api/admin/secret"} {
		target = bad
		if _, err := client.Open(context.Background(), fileaccess.SourceRequest{Path: "/file", Size: 10}); err == nil || strings.Contains(err.Error(), "SECRET") {
			t.Fatal("unsafe source", bad, err)
		}
	}
	if contentCalls != before {
		t.Fatal("SSRF made content call")
	}
	target = base + "/p/file"
	badRange = true
	if _, err := client.Open(context.Background(), fileaccess.SourceRequest{Path: "/file", Size: 10, Range: "bytes=2-4"}); err == nil {
		t.Fatal("bad upstream partial body")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.Open(ctx, fileaccess.SourceRequest{Path: "/file", Size: 10}); err == nil {
		t.Fatal("cancelled access")
	}
}
func TestPublicNetworkAndRedirectTargetPolicy(t *testing.T) {
	for _, s := range []string{"127.0.0.1", "10.0.0.1", "192.168.178.50", "169.254.169.254", "100.64.0.1", "0.0.0.0", "198.18.0.1", "192.0.2.1", "240.0.0.1", "::1", "fc00::1", "::ffff:127.0.0.1", "2001:db8::1", "2002:c0a8:101::1"} {
		if publicIP(netip.MustParseAddr(s)) {
			t.Fatal("private/reserved IP accepted", s)
		}
	}
	if !publicIP(netip.MustParseAddr("8.8.8.8")) {
		t.Fatal("public IPv4 refused")
	}
	c, _ := NewContentClient("http://127.0.0.1:5244", "token")
	for _, s := range []string{"http://127.0.0.1:5244/api/fs/get", "http://192.168.178.50/p/file", "http://user:pass@cdn.example/file", "https://cdn.example/file#fragment"} {
		u, _ := url.Parse(s)
		if c.safeTarget(u) {
			t.Fatal(s)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if conn, err := c.dial(ctx, "tcp", "127.0.0.1:1"); err == nil {
		conn.Close()
		t.Fatal("dial private")
	}
}
