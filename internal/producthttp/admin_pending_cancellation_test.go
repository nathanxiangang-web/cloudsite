package producthttp

import (
	"context"
	"errors"
	"github.com/nathanxiangang-web/panta/internal/adminops"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"net/http/httptest"
	"strings"
	"testing"
)

type pendingHTTPFake struct {
	operationsFake
	result adminops.Result
}

func (f *pendingHTTPFake) Execute(ctx context.Context, a adminops.Actor, c adminops.Command) (adminops.Result, error) {
	if !c.Valid() {
		return adminops.Result{}, adminops.ErrInvalid
	}
	_, e := f.operationsFake.Execute(ctx, a, c)
	r := f.result
	r.Action = c.Action
	r.RequestID = c.RequestID
	r.TargetID = c.TargetID
	return r, e
}
func TestAdminPendingCancellationHTTPBoundary(t *testing.T) {
	const id = "99500000-0000-4000-8000-000000000001"
	const path = "/api/v1/admin/downloads/" + id + "/cancel-pending"
	const body = `{"request_id":"` + id + `","expected_owner_user_id":"` + id + `","expected_binding_id":"` + id + `","expected_updated_at":"2026-10-26T00:00:00.123456Z"}`
	for _, tc := range []struct {
		name, method, body, origin, token, accept, status, code string
		role                                                    auth.Role
		port                                                    bool
		err                                                     error
		want                                                    int
	}{
		{name: "success", want: 200}, {name: "unknown", status: "UNKNOWN", want: 202}, {name: "changed", status: "REJECTED", code: "PENDING_CHANGED", want: 409}, {name: "hidden", status: "REJECTED", code: "MANIFEST_NOT_FOUND", want: 404},
		{name: "unauthorized", token: "none", want: 401}, {name: "forbidden", role: auth.RoleUser, want: 403}, {name: "get", method: "GET", want: 405}, {name: "head", method: "HEAD", want: 405},
		{name: "origin", origin: "https://evil.invalid", want: 403}, {name: "no-origin", origin: "none", want: 403}, {name: "accept", accept: "text/html", want: 406},
		{name: "missing-port", port: true, want: 503}, {name: "store-fault", err: errors.New("SECRET Cookie token"), want: 503}, {name: "key-conflict", err: adminops.ErrConflict, want: 409},
		{name: "bad-request", body: strings.Replace(body, id, "bad", 1), want: 400},
		{name: "bad-owner", body: strings.Replace(body, `"expected_owner_user_id":"`+id+`"`, `"expected_owner_user_id":"bad"`, 1), want: 400},
		{name: "bad-binding", body: strings.Replace(body, `"expected_binding_id":"`+id+`"`, `"expected_binding_id":"bad"`, 1), want: 400},
		{name: "extra-field", body: strings.TrimSuffix(body, "}") + `,"expected_version":0}`, want: 400},
		{name: "duplicate", body: strings.TrimSuffix(body, "}") + `,"request_id":"` + id + `"}`, want: 400},
		{name: "null", body: strings.Replace(body, `"2026-10-26T00:00:00.123456Z"`, `null`, 1), want: 400},
		{name: "time", body: strings.Replace(body, "2026-10-26T00:00:00.123456Z", "never", 1), want: 400},
		{name: "sub-microsecond", body: strings.Replace(body, "123456Z", "123456789Z", 1), want: 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			role := tc.role
			if role == "" {
				role = auth.RoleAdmin
			}
			a := &fakeAccess{p: auth.Principal{UserID: id, Login: "admin", Role: role}}
			f := &pendingHTTPFake{result: adminops.Result{Status: "SUCCEEDED"}}
			f.err = tc.err
			if tc.status != "" {
				f.result.Status = tc.status
			}
			f.result.Code = tc.code
			var port adminops.Operations = f
			if tc.port {
				port = nil
			}
			h, e := NewWorkspaceHandler(a, "http://127.0.0.1:8180", &readAccessFake{}, []DisplayBinding{{ID: id, Label: "Test"}}, 16, WithAdminOperations(port))
			if e != nil {
				t.Fatal(e)
			}
			method := tc.method
			if method == "" {
				method = "POST"
			}
			payload := tc.body
			if payload == "" {
				payload = body
			}
			req := httptest.NewRequest(method, "http://127.0.0.1:8180"+path, strings.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			origin := tc.origin
			if origin == "" {
				origin = "http://127.0.0.1:8180"
			}
			if origin != "none" {
				req.Header.Set("Origin", origin)
			}
			if tc.token != "none" {
				req.Header.Set("Cookie", SessionCookieName+"="+testToken)
			}
			if tc.accept != "" {
				req.Header.Set("Accept", tc.accept)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			if w.Code != tc.want || strings.Contains(w.Body.String(), "SECRET") {
				t.Fatal(w.Code, w.Body.String())
			}
			if method == "POST" && w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("cacheable")
			}
			if w.Code == 200 && (f.command.Action != adminops.CancelPending || f.command.TargetID != id || f.command.Pending == nil) {
				t.Fatal(f.command)
			}
		})
	}
}
