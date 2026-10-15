package content

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/nathanxiangang-web/panta/internal/fileaccess"
)

const contentUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36"

type ContentClient struct {
	base        *url.URL
	token       func(context.Context) (string, error)
	api, stream *http.Client
}

func NewContentClient(base, token string) (*ContentClient, error) {
	return NewContentClientWithTokenSource(base, func(context.Context) (string, error) { return token, nil })
}

// NewContentClientWithTokenSource supports protected application-token rotation
// without retaining a stale token or constructing a provider/login session.
func NewContentClientWithTokenSource(base string, token func(context.Context) (string, error)) (*ContentClient, error) {
	if token == nil {
		return nil, fileaccess.ErrUnavailable
	}
	initial, secretErr := token(context.Background())
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") || secretErr != nil || !validToken(initial) {
		return nil, fileaccess.ErrUnavailable
	}
	c := &ContentClient{base: u, token: token}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.ResponseHeaderTimeout = 15 * time.Second
	transport.DisableCompression = true
	transport.DialContext = c.dial
	c.api = &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	c.stream = &http.Client{Transport: transport, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) > 4 || !c.safeTarget(r.URL) {
			return fileaccess.ErrUnavailable
		}
		return nil
	}}
	return c, nil
}
func validToken(token string) bool {
	return strings.TrimSpace(token) != "" && len(token) <= 8192 && !strings.ContainsAny(token, "\r\n")
}
func publicIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, prefix := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32", "2001::/32", "2002::/16"} {
		if netip.MustParsePrefix(prefix).Contains(ip) {
			return false
		}
	}
	return true
}
func (c *ContentClient) safeTarget(u *url.URL) bool {
	if u == nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Fragment != "" || u.Opaque != "" {
		return false
	}
	if u.Scheme == c.base.Scheme && u.Host == c.base.Host {
		return strings.HasPrefix(u.Path, "/p/") || strings.HasPrefix(u.Path, "/d/")
	}
	if ip, e := netip.ParseAddr(u.Hostname()); e == nil {
		return publicIP(ip)
	}
	return true // hostnames are validated AGAIN at the actual socket dial.
}
func (c *ContentClient) dial(ctx context.Context, network, address string) (net.Conn, error) {
	if address == c.base.Host || address == net.JoinHostPort(c.base.Hostname(), defaultPort(c.base)) {
		return (&net.Dialer{Timeout: 8 * time.Second}).DialContext(ctx, network, address)
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fileaccess.ErrUnavailable
	}
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil || len(ips) == 0 {
		return nil, fileaccess.ErrUnavailable
	}
	for _, ip := range ips {
		if !publicIP(ip) {
			return nil, fileaccess.ErrUnavailable
		}
	}
	for _, ip := range ips {
		conn, e := (&net.Dialer{Timeout: 8 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if e == nil {
			return conn, nil
		}
	}
	return nil, fileaccess.ErrUnavailable
}
func defaultPort(u *url.URL) string {
	if u.Port() != "" {
		return u.Port()
	}
	if u.Scheme == "https" {
		return "443"
	}
	return "80"
}
func (c *ContentClient) resolveURL(ctx context.Context, r fileaccess.SourceRequest, userAgent string) (*url.URL, error) {
	token, err := c.token(ctx)
	if err != nil || !validToken(token) {
		return nil, fileaccess.ErrUnavailable
	}
	raw, _ := json.Marshal(map[string]string{"path": r.Path, "password": ""})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base.String()+"/api/fs/get", bytes.NewReader(raw))
	if err != nil {
		return nil, fileaccess.ErrUnavailable
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", token)
	request.Header.Set("User-Agent", userAgent)
	response, err := c.api.Do(request)
	if err != nil {
		return nil, fileaccess.ErrUnavailable
	}
	payload, readErr := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	response.Body.Close()
	var result struct {
		Code int `json:"code"`
		Data *struct {
			URL       string `json:"raw_url"`
			Size      *int64 `json:"size"`
			Directory *bool  `json:"is_dir"`
		} `json:"data"`
	}
	if readErr != nil || response.StatusCode != 200 || len(payload) > 1<<20 || json.Unmarshal(payload, &result) != nil || result.Code != 200 || result.Data == nil || result.Data.Directory == nil || *result.Data.Directory || result.Data.Size == nil || *result.Data.Size != r.Size {
		return nil, fileaccess.ErrUnavailable
	}
	target, err := url.Parse(result.Data.URL)
	if err != nil {
		return nil, fileaccess.ErrUnavailable
	}
	target = c.base.ResolveReference(target)
	if !c.safeTarget(target) {
		return nil, fileaccess.ErrUnavailable
	}
	return target, nil
}

// ResolveLink only reads OpenList metadata; it never GETs the target file.
func (c *ContentClient) ResolveLink(ctx context.Context, r fileaccess.SourceRequest) (string, error) {
	ua := r.UserAgent
	if ua == "" {
		ua = contentUA
	}
	if len(ua) > 2048 || strings.ContainsAny(ua, "\x00\r\n") {
		return "", fileaccess.ErrUnavailable
	}
	target, err := c.resolveURL(ctx, r, ua)
	if err != nil {
		return "", fileaccess.ErrUnavailable
	}
	return target.String(), nil
}

func (c *ContentClient) Open(ctx context.Context, r fileaccess.SourceRequest) (fileaccess.Content, error) {
	target, err := c.resolveURL(ctx, r, contentUA)
	if err != nil {
		return fileaccess.Content{}, err
	}
	method := http.MethodGet
	if r.Head {
		method = http.MethodHead
	}
	request, err := http.NewRequestWithContext(ctx, method, target.String(), nil)
	if err != nil {
		return fileaccess.Content{}, fileaccess.ErrUnavailable
	}
	request.Header.Set("User-Agent", contentUA)
	request.Header.Set("Accept-Encoding", "identity")
	if r.Range != "" {
		request.Header.Set("Range", r.Range)
	}
	response, err := c.stream.Do(request)
	if err != nil {
		return fileaccess.Content{}, fileaccess.ErrUnavailable
	}
	reject := func() (fileaccess.Content, error) {
		response.Body.Close()
		return fileaccess.Content{}, fileaccess.ErrUnavailable
	}
	start, end, partial, err := fileaccess.ByteRange(r.Range, r.Size)
	if err != nil {
		response.Body.Close()
		return fileaccess.Content{}, err
	}
	status := http.StatusOK
	length := r.Size
	expectedRange := ""
	if partial {
		status = http.StatusPartialContent
		length = end - start + 1
		expectedRange = fmt.Sprintf("bytes %d-%d/%d", start, end, r.Size)
	}
	if response.StatusCode != status || response.ContentLength != length || response.Header.Get("Content-Range") != expectedRange || response.Header.Get("Content-Encoding") != "" && response.Header.Get("Content-Encoding") != "identity" {
		return reject()
	}
	if r.Head {
		response.Body.Close()
		response.Body = io.NopCloser(strings.NewReader(""))
	}
	return fileaccess.Content{Body: response.Body, Status: status, Length: length, ContentRange: expectedRange}, nil
}

var _ fileaccess.Source = (*ContentClient)(nil)
