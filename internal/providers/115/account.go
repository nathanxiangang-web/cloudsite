package p115

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/SheltonZhu/115driver/pkg/driver"
	"github.com/nathanxiangang-web/panta/internal/adminops"
)

// GetAccountInfo is separate from offline task clients. One SDK GetInfo call,
// no LoginCheck/GetUser, refresh, directory access, task polling or downloader.
func GetAccountInfo(ctx context.Context, cookie []byte) adminops.Capacity {
	return getAccountInfo(ctx, cookie, http.DefaultTransport)
}
func getAccountInfo(ctx context.Context, cookie []byte, next http.RoundTripper) adminops.Capacity {
	result := adminops.Capacity{Status: "UNKNOWN", Source: "115_GET_INFO"}
	credential, e := parseCredential(cookie)
	if e != nil {
		result.Status = "NOT_CONNECTED"
		result.ProbedAt = time.Now().UTC()
		return result
	}
	transport := &accountTransport{ctx: ctx, next: next}
	client := newDriverClient(&http.Client{Timeout: 8 * time.Second, Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).ImportCredential(credential)
	forgetCredential(credential)
	client.Client.SetRetryCount(0)
	info, e := client.GetInfo()
	result.ProbedAt = time.Now().UTC()
	if transport.notAuthenticated || errors.Is(e, driver.ErrNotLogin) {
		result.Status = "NOT_AUTHENTICATED"
		return result
	}
	if e != nil || ctx.Err() != nil || !transport.shapeOK || transport.calls != 1 {
		return result
	}
	t, u, r := info.SpaceInfo.AllTotal.Size, info.SpaceInfo.AllUse.Size, info.SpaceInfo.AllRemain.Size
	if t <= 0 || u < 0 || r < 0 || u > t || r != t-u {
		return result
	}
	result.Status = "AUTHENTICATED"
	result.TotalBytes = strconv.FormatInt(t, 10)
	result.UsedBytes = strconv.FormatInt(u, 10)
	result.RemainingBytes = strconv.FormatInt(r, 10)
	return result
}

var errAccountResponse = errors.New("invalid account response")

type accountTransport struct {
	ctx                       context.Context
	next                      http.RoundTripper
	calls                     int
	shapeOK, notAuthenticated bool
}

func (t *accountTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.calls++
	if t.calls != 1 || req.Method != "GET" || req.URL.String() != driver.ApiFileIndexInfo {
		return nil, errAccountResponse
	}
	resp, e := t.next.RoundTrip(req.Clone(t.ctx))
	if e != nil {
		return nil, errAccountResponse
	}
	if resp == nil || resp.Body == nil {
		return nil, errAccountResponse
	}
	defer resp.Body.Close()
	raw, e := io.ReadAll(io.LimitReader(resp.Body, (128<<10)+1))
	if e != nil || len(raw) > 128<<10 {
		return nil, errAccountResponse
	}
	if resp.StatusCode == 401 {
		t.notAuthenticated = true
	}
	if resp.StatusCode != 200 {
		return nil, errAccountResponse
	}
	var body struct {
		State bool `json:"state"`
		Code  int  `json:"code"`
		Errno int  `json:"errno"`
		Data  struct {
			Space map[string]json.RawMessage `json:"space_info"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &body) != nil {
		return nil, errAccountResponse
	}
	if body.Code == 990001 || body.Code == 99 || body.Errno == 990001 || body.Errno == 99 {
		t.notAuthenticated = true
	}
	// Pinned SDK would default omitted data to zero. Require explicit fields so
	// malformed success cannot be displayed as a healthy empty account.
	if body.State {
		t.shapeOK = true
		for _, key := range []string{"all_total", "all_use", "all_remain"} {
			var field struct {
				Size json.RawMessage `json:"size"`
			}
			v := body.Data.Space[key]
			if len(v) == 0 || json.Unmarshal(v, &field) != nil || len(field.Size) == 0 || bytes.Equal(bytes.TrimSpace(field.Size), []byte("null")) {
				t.shapeOK = false
				continue
			}
			text := string(field.Size)
			if field.Size[0] == '"' {
				if json.Unmarshal(field.Size, &text) != nil {
					t.shapeOK = false
					continue
				}
			}
			if text == "" {
				t.shapeOK = false
			}
			for _, ch := range text {
				if ch < '0' || ch > '9' {
					t.shapeOK = false
				}
			}
			if _, e := strconv.ParseInt(text, 10, 64); e != nil {
				t.shapeOK = false
			}
		}
	}
	resp.Body = io.NopCloser(bytes.NewReader(raw))
	return resp, nil
}
