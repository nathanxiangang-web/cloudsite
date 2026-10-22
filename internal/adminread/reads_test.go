package adminread

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nathanxiangang-web/panta/internal/auth"
)

const aid = "98000000-0000-4000-8000-000000000001"
const bid = "98000000-0000-4000-8000-000000000002"
const mid = "98000000-0000-4000-8000-000000000003"
const jid = "98000000-0000-4000-8000-000000000004"

type readerFake struct {
	users         []User
	downloads     []DownloadRecord
	err           error
	calls, limit  int
	cursor, owner string
	deadline      bool
}

func (f *readerFake) ListUsers(ctx context.Context, n int, c string) ([]User, error) {
	f.calls++
	f.limit = n
	f.cursor = c
	_, f.deadline = ctx.Deadline()
	return f.users, f.err
}
func (f *readerFake) ListDownloads(ctx context.Context, b []string, n int, c, u string) ([]DownloadRecord, error) {
	f.calls++
	f.limit = n
	f.cursor = c
	f.owner = u
	_, f.deadline = ctx.Deadline()
	b[0] = "corrupt-copy"
	return f.downloads, f.err
}
func admin() auth.Principal {
	return auth.Principal{UserID: aid, Login: "test-admin", Role: auth.RoleAdmin}
}
func user(id string) User {
	now := time.Now()
	return User{UserID: id, Login: "test-user", Role: auth.RoleUser, Enabled: true, CreatedAt: now, UpdatedAt: now}
}
func download() DownloadRecord {
	now := time.Now()
	return DownloadRecord{Download: Download{ManifestID: mid, OwnerUserID: aid, BindingID: bid, ManifestState: "PENDING", CreatedAt: now, UpdatedAt: now}, LinkValid: true}
}
func TestAdminReadAuthorizationPaginationAndIsolation(t *testing.T) {
	f := &readerFake{users: []User{user(aid), user(bid)}, downloads: []DownloadRecord{download()}}
	bindings := []string{bid}
	s, err := NewService(f, bindings)
	if err != nil {
		t.Fatal(err)
	}
	bindings[0] = aid
	page, err := s.Users(context.Background(), admin(), 1, "")
	if err != nil || len(page.Items) != 1 || page.NextCursor != aid || f.limit != 2 || !f.deadline {
		t.Fatal(page, err)
	}
	f.users = []User{user(bid)}
	page, err = s.Users(context.Background(), admin(), 1, aid)
	if err != nil || page.NextCursor != "" || f.cursor != aid {
		t.Fatal(page, err)
	}
	d, err := s.Downloads(context.Background(), admin(), 20, "", aid)
	if err != nil || len(d.Items) != 1 || f.owner != aid || s.bindings[0] != bid {
		t.Fatal(d, err)
	}
	before := f.calls
	for _, p := range []auth.Principal{{}, {UserID: aid, Login: "test-user", Role: auth.RoleUser}} {
		if _, err = s.Users(context.Background(), p, 20, ""); err == nil {
			t.Fatal("unauthorized")
		}
		if _, err = s.Downloads(context.Background(), p, 20, "", ""); err == nil {
			t.Fatal("unauthorized")
		}
	}
	for _, q := range []struct {
		n    int
		c, u string
	}{{0, "", ""}, {51, "", ""}, {20, "offset:20", ""}, {20, "", "not-a-uuid"}} {
		if _, err = s.Downloads(context.Background(), admin(), q.n, q.c, q.u); !errors.Is(err, ErrInvalid) {
			t.Fatal(q, err)
		}
	}
	if before != f.calls {
		t.Fatal("invalid call reached port")
	}
}
func TestAdminReadFailsClosedOnCorruptProjection(t *testing.T) {
	for _, mutate := range []func(*DownloadRecord){
		func(r *DownloadRecord) { r.BindingID = jid }, func(r *DownloadRecord) { r.OwnerUserID = "" }, func(r *DownloadRecord) { r.LinkValid = false }, func(r *DownloadRecord) { r.ManifestState = "READY" }, func(r *DownloadRecord) { r.JobID = new(jid) }, func(r *DownloadRecord) { r.FailureCount = 1 }, func(r *DownloadRecord) { r.CreatedAt = time.Time{} }, func(r *DownloadRecord) { r.ResultCopyID = new(jid) },
	} {
		r := download()
		mutate(&r)
		s, _ := NewService(&readerFake{downloads: []DownloadRecord{r}}, []string{bid})
		if _, err := s.Downloads(context.Background(), admin(), 20, "", ""); !errors.Is(err, ErrUnavailable) {
			t.Fatal(r, err)
		}
	}
	for _, rows := range [][]User{{user(aid), user(aid)}, {user(bid), user(aid)}, {user("invalid")}} {
		s, _ := NewService(&readerFake{users: rows}, []string{bid})
		if _, err := s.Users(context.Background(), admin(), 20, ""); !errors.Is(err, ErrUnavailable) {
			t.Fatal("bad user page", err)
		}
	}
	for _, bad := range []func(*User){func(r *User) { r.Login = "Cookie=SECRET" }, func(r *User) { r.Role = "ROOT" }, func(r *User) { r.CreatedAt = time.Time{} }, func(r *User) { r.UpdatedAt = r.CreatedAt.Add(-time.Second) }} {
		r := user(aid)
		bad(&r)
		s, _ := NewService(&readerFake{users: []User{r}}, []string{bid})
		if _, err := s.Users(context.Background(), admin(), 20, ""); !errors.Is(err, ErrUnavailable) {
			t.Fatal("bad user", err)
		}
	}
}
func TestAdminReadStateMatrixAndDTOAllowlist(t *testing.T) {
	for _, pair := range [][2]string{{"ACTIVE", "QUEUED"}, {"ACTIVE", "RUNNING"}, {"ACTIVE", "RETRY_WAIT"}, {"AWAITING_VISIBILITY", "RETRY_WAIT"}, {"AWAITING_CANONICAL", "RUNNING"}, {"RECOVERY_REQUIRED", "RECOVERY_REQUIRED"}, {"READY", "SUCCEEDED"}, {"FAILED", "FAILED"}, {"CANCELED", "CANCELED"}} {
		r := download()
		r.ManifestState = pair[0]
		r.JobID = new(jid)
		r.JobState = new(pair[1])
		r.ClaimAttempts = 57
		r.FailureCount = 2
		r.MaxAttempts = 3
		if pair[0] == "READY" {
			r.ResultCopyID = new(jid)
		}
		if !validDownload(r) {
			t.Fatal(pair)
		}
		s, _ := NewService(&readerFake{downloads: []DownloadRecord{r}}, []string{bid})
		page, err := s.Downloads(context.Background(), admin(), 20, "", "")
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(page.Items[0])
		var fields map[string]any
		_ = json.Unmarshal(raw, &fields)
		if len(fields) != 11 {
			t.Fatal(string(raw))
		}
		for _, name := range []string{"LinkValid", "MaxAttempts", "source_ref", "payload", "provider_ref", "operation_key", "last_error", "lease_owner"} {
			if strings.Contains(string(raw), name) {
				t.Fatal(name)
			}
		}
		r.JobState = new("SUCCEEDED")
		if pair[0] != "READY" && validDownload(r) {
			t.Fatal("mismatched state accepted", pair)
		}
	}
	raw, _ := json.Marshal(user(aid))
	var fields map[string]any
	_ = json.Unmarshal(raw, &fields)
	if len(fields) != 9 {
		t.Fatal(string(raw))
	}
	for _, name := range []string{"version", "user_id", "login", "role", "enabled", "created_at", "updated_at", "deleted_at", "deleted_by"} {
		if _, ok := fields[name]; !ok {
			t.Fatal("missing allowlisted field", name)
		}
	}
}
func TestAdminReadCancellationErrorsAndConstructor(t *testing.T) {
	for _, err := range []error{errors.New("Cookie=SECRET"), context.Canceled, context.DeadlineExceeded} {
		f := &readerFake{err: err}
		s, _ := NewService(f, []string{bid})
		_, got := s.Users(context.Background(), admin(), 20, "")
		if err == context.Canceled || err == context.DeadlineExceeded {
			if !errors.Is(got, err) {
				t.Fatal(got)
			}
		} else if got != ErrUnavailable {
			t.Fatal(got)
		}
	}
	f := &readerFake{}
	s, _ := NewService(f, []string{bid})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Downloads(ctx, admin(), 20, "", ""); !errors.Is(err, context.Canceled) || f.calls != 0 {
		t.Fatal(err)
	}
	var typedNil *readerFake
	for _, port := range []Reader{nil, typedNil} {
		if _, err := NewService(port, []string{bid}); err == nil {
			t.Fatal("nil port")
		}
	}
	for _, ids := range [][]string{nil, {"bad"}, {bid, bid}} {
		if _, err := NewService(f, ids); err == nil {
			t.Fatal("bad scope")
		}
	}
}
