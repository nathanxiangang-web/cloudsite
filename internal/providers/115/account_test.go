package p115

import (
	"context"
	"encoding/json"
	"github.com/SheltonZhu/115driver/pkg/driver"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type accountRoundTrip func(*http.Request) (*http.Response, error)

func (f accountRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

const accountCookie = "UID=123_A1_123; CID=synthetic-cid; SEID=synthetic-seid"
const validAccountBody = `{"state":true,"data":{"space_info":{"all_total":{"size":"9007199254740993"},"all_use":{"size":1},"all_remain":{"size":"9007199254740992"}},"login_devices_info":{"last":{"ip":"SECRET-IP"}},"imei_info":"SECRET-DEVICE"}}`

func TestAccountGetInfoPinnedSDKProtocolAndRedaction(t *testing.T) {
	cases := []struct {
		name, body, status string
		http               int
	}{
		{"valid", validAccountBody, "AUTHENTICATED", 200},
		{"expired", `{"state":false,"code":990001,"error":"SECRET"}`, "NOT_AUTHENTICATED", 200},
		{"denied", `{}`, "NOT_AUTHENTICATED", 401},
		{"forbidden-not-login-proof", `{}`, "UNKNOWN", 403},
		{"upstream-throttled", `{}`, "UNKNOWN", 429},
		{"server", `SECRET`, "UNKNOWN", 500},
		{"redirect", ``, "UNKNOWN", 302},
		{"html", `<SECRET>`, "UNKNOWN", 200},
		{"missing", `{"state":true,"data":{}}`, "UNKNOWN", 200},
		{"null", `{"state":true,"data":{"space_info":null}}`, "UNKNOWN", 200},
		{"missing-size", `{"state":true,"data":{"space_info":{"all_total":{"size":10},"all_use":{},"all_remain":{"size":10}}}}`, "UNKNOWN", 200},
		{"fraction", `{"state":true,"data":{"space_info":{"all_total":{"size":"10.5"},"all_use":{"size":0},"all_remain":{"size":10}}}}`, "UNKNOWN", 200},
		{"inconsistent", `{"state":true,"data":{"space_info":{"all_total":10,"all_use":2,"all_remain":3}}}`, "UNKNOWN", 200},
		{"oversize", strings.Repeat("x", 129<<10), "UNKNOWN", 200},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			rt := accountRoundTrip(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Method != "GET" || r.URL.String() != driver.ApiFileIndexInfo || r.Header.Get("User-Agent") != driver.UA115Browser {
					t.Fatal("wrong endpoint/method/UA")
				}
				return &http.Response{StatusCode: tc.http, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})
			result := getAccountInfo(context.Background(), []byte(accountCookie), rt)
			if result.Status != tc.status || calls != 1 || result.ProbedAt.IsZero() {
				t.Fatal(result, calls)
			}
			raw, _ := json.Marshal(result)
			for _, bad := range []string{"SECRET", accountCookie, "webapi.115.com", "synthetic"} {
				if strings.Contains(string(raw), bad) {
					t.Fatal("secret leaked")
				}
			}
			if tc.status == "AUTHENTICATED" && result.TotalBytes != "9007199254740993" {
				t.Fatal("rounded bytes")
			}
			if tc.status != "AUTHENTICATED" && result.TotalBytes != "" {
				t.Fatal("fake capacity")
			}
		})
	}
}
func TestAccountGetInfoCancellationAndBadCredential(t *testing.T) {
	ctx, stop := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer stop()
	calls := 0
	result := getAccountInfo(ctx, []byte(accountCookie), accountRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		<-r.Context().Done()
		return nil, r.Context().Err()
	}))
	if result.Status != "UNKNOWN" || calls != 1 {
		t.Fatal("cancel not propagated")
	}
	result = getAccountInfo(context.Background(), []byte("bad-secret"), accountRoundTrip(func(*http.Request) (*http.Response, error) { t.Fatal("unexpected network"); return nil, nil }))
	if result.Status != "NOT_CONNECTED" {
		t.Fatal(result)
	}
}
