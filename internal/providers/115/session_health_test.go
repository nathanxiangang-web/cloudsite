package p115

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/SheltonZhu/115driver/pkg/driver"
	"github.com/nathanxiangang-web/panta/internal/sessionhealth"
)

type healthTransport func(*http.Request) (*http.Response, error)

func (f healthTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSessionHealthSameRuntimeJarReadOnlyAndStrictResponse(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		code       int
		want       sessionhealth.Status
	}{
		{"valid", `{"state":true,"data":{"user_id":123}}`, 200, sessionhealth.SessionAuthenticated},
		{"string-id", `{"state":true,"data":{"user_id":"123"}}`, 200, sessionhealth.SessionAuthenticated},
		{"other-account", `{"state":true,"data":{"user_id":456}}`, 200, sessionhealth.SessionUnknown},
		{"expired-sdk-decode-shape", `{"state":false,"data":[]}`, 200, sessionhealth.SessionLoginRequired},
		{"errno", `{"errno":990001}`, 200, sessionhealth.SessionLoginRequired},
		{"http401", `PRIVATE`, 401, sessionhealth.SessionLoginRequired},
		{"upstream500", `{"state":false}`, 500, sessionhealth.SessionUnknown},
		{"limited429", `{"state":false}`, 429, sessionhealth.SessionUnknown},
		{"unrecognized-error", `{"state":false,"code":12345}`, 200, sessionhealth.SessionUnknown},
		{"redirect", ``, 302, sessionhealth.SessionUnknown},
		{"missing-state", `{"data":{"user_id":123}}`, 200, sessionhealth.SessionUnknown},
		{"html", `<html>PRIVATE</html>`, 200, sessionhealth.SessionUnknown},
		{"oversize", strings.Repeat("x", 65537), 200, sessionhealth.SessionUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			transport := healthTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Method != "GET" || r.URL.String() != driver.ApiUserInfo || r.Header.Get("User-Agent") != driver.UA115Browser {
					t.Fatal("unexpected endpoint")
				}
				c, e := r.Cookie("UID")
				if e != nil || c.Value != "123_A1_1" {
					t.Fatal("runtime cookies not reused")
				}
				return &http.Response{StatusCode: tc.code, Body: io.NopCloser(strings.NewReader(tc.body)), Header: http.Header{"Location": []string{"https://example.invalid/never-follow"}}, Request: r}, nil
			})
			adapter, err := newAdapterFromCookie([]byte("UID=123_A1_1; CID=synthetic; SEID=synthetic; KID=synthetic"), Options{}, transport)
			if err != nil {
				t.Fatal(err)
			}
			if calls != 0 {
				t.Fatal("constructor probed")
			}
			if status := adapter.CheckSession(context.Background()); status != tc.want {
				t.Fatal(status, tc.want)
			}
			if calls != 1 {
				t.Fatal("retried", calls)
			}
		})
	}
}
func TestSessionHealthTransportAndCanceledAreUnknown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	client := &http.Client{Transport: healthTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Context().Err() != nil {
			return nil, r.Context().Err()
		}
		return nil, errors.New("PRIVATE")
	})}
	if checkSession(ctx, client, "123") != sessionhealth.SessionUnknown {
		t.Fatal("cancel")
	}
	if checkSession(context.Background(), client, "123") != sessionhealth.SessionUnknown {
		t.Fatal("transport")
	}
}
