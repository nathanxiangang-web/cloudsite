package p115

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/SheltonZhu/115driver/pkg/driver"
	"github.com/nathanxiangang-web/panta/internal/sessionhealth"
)

// CheckSession reads fresh user information with the exact cookies used by
// this runtime's downloader. It does not use SSO LoginCheck, refresh credentials,
// mutate the SDK's cached user, or touch offline tasks. A separate HTTP client
// avoids serializing this GET behind a multi-page offline poll or its trace.
func (a *Adapter) CheckSession(ctx context.Context) sessionhealth.Status {
	backend, ok := a.backend.(*CookiedBackend)
	if !ok || backend.healthHTTP == nil {
		return sessionhealth.SessionUnknown
	}
	return checkSession(ctx, backend.healthHTTP, backend.accountID)
}

// 115driver imports credentials into resty's immutable client Cookies slice,
// not the net/http jar. Reuse those objects, without parsing a second secret or
// touching the SDK's mutable Request pointer. Never send them to another URL.
type healthCookieTransport struct {
	next    http.RoundTripper
	cookies []*http.Cookie
}

func (t healthCookieTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method != http.MethodGet || req.URL.String() != driver.ApiUserInfo {
		return nil, errors.New("unexpected session probe")
	}
	for _, cookie := range t.cookies {
		req.AddCookie(cookie)
	}
	return t.next.RoundTrip(req)
}

func checkSession(ctx context.Context, client *http.Client, accountID string) sessionhealth.Status {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, driver.ApiUserInfo, nil)
	if err != nil {
		return sessionhealth.SessionUnknown
	}
	req.Header.Set("User-Agent", driver.UA115Browser)
	req.Header.Set("Cache-Control", "no-cache")
	resp, err := client.Do(req)
	if err != nil || resp == nil {
		return sessionhealth.SessionUnknown
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return sessionhealth.SessionLoginRequired
	}
	if resp.StatusCode != http.StatusOK {
		return sessionhealth.SessionUnknown
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, (64<<10)+1))
	defer clear(raw)
	if err != nil || len(raw) > 64<<10 || ctx.Err() != nil {
		return sessionhealth.SessionUnknown
	}
	// Unmarshal the envelope independently: expired sessions can return data as
	// a different type, which the SDK otherwise reports as a JSON decode error.
	var envelope struct {
		State *bool           `json:"state"`
		Code  int             `json:"code"`
		Errno int             `json:"errno"`
		Data  json.RawMessage `json:"data"`
	}
	if json.Unmarshal(raw, &envelope) != nil {
		return sessionhealth.SessionUnknown
	}
	defer clear(envelope.Data)
	if envelope.Code == 99 || envelope.Code == 990001 || envelope.Errno == 99 || envelope.Errno == 990001 {
		return sessionhealth.SessionLoginRequired
	}
	if envelope.State == nil {
		return sessionhealth.SessionUnknown
	}
	if envelope.Code != 0 || envelope.Errno != 0 {
		return sessionhealth.SessionUnknown
	}
	if !*envelope.State {
		return sessionhealth.SessionLoginRequired
	}
	var data struct {
		UserID json.Number `json:"user_id"`
	}
	if json.Unmarshal(envelope.Data, &data) != nil {
		return sessionhealth.SessionUnknown
	}
	uid, err := strconv.ParseInt(data.UserID.String(), 10, 64)
	if err != nil || uid <= 0 || data.UserID.String() != accountID {
		return sessionhealth.SessionUnknown
	}
	return sessionhealth.SessionAuthenticated
}
