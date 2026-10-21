package adminobserve

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nathanxiangang-web/panta/internal/auth"
)

const binding = "99000000-0000-4000-8000-000000000001"
const connection = "99000000-0000-4000-8000-000000000002"

func principal() auth.Principal {
	return auth.Principal{UserID: connection, Login: "fixture-admin", Role: auth.RoleAdmin}
}
func mapping() Mapping {
	now := time.Now().UTC()
	return Mapping{Storage: Storage{BindingID: binding, ConnectionID: connection, ProviderType: "115", BindingStatus: "ACTIVE", ConnectionStatus: "DISABLED", BindingUpdatedAt: now, ConnectionUpdatedAt: now, RootConfigured: true}, RootID: "private-root"}
}

type mappings struct {
	m     Mapping
	rows  []Storage
	err   error
	calls atomic.Int64
	onGet func(int64) Mapping
}

func (f *mappings) List(ctx context.Context, ids []string, n int, c string) ([]Storage, error) {
	f.calls.Add(1)
	return f.rows, f.err
}
func (f *mappings) Get(ctx context.Context, ids []string, id string) (Mapping, error) {
	n := f.calls.Add(1)
	if f.onGet != nil {
		return f.onGet(n), f.err
	}
	return f.m, f.err
}

type roots struct {
	calls atomic.Int64
	read  func(context.Context, string) (RootState, error)
}

func (f *roots) ReadRoot(ctx context.Context, id string) (RootState, error) {
	f.calls.Add(1)
	return f.read(ctx, id)
}
func setup() (*Service, *mappings, *roots) {
	m := mapping()
	db := &mappings{m: m, rows: []Storage{m.Storage}}
	r := &roots{read: func(ctx context.Context, id string) (RootState, error) {
		return RootState{RootID: id, Lifecycle: "ACTIVE", Generation: 4}, nil
	}}
	s, _ := NewService(db, r, []Display{{binding, "Public label"}})
	return s, db, r
}
func TestObserveDTOAndConfigurationNotHealth(t *testing.T) {
	s, db, r := setup()
	db.rows[0].DisplayLabel = "SECRET"
	page, err := s.Storage(context.Background(), principal(), 1, "")
	if err != nil || len(page.Items) != 1 || page.Items[0].DisplayLabel != "Public label" {
		t.Fatal(page, err)
	}
	raw, _ := json.Marshal(page.Items[0])
	var fields map[string]any
	_ = json.Unmarshal(raw, &fields)
	if len(fields) != 9 || strings.Contains(string(raw), "SECRET") || r.calls.Load() != 0 {
		t.Fatal(string(raw))
	}
	for _, state := range []string{"NEW", "ACTIVE", "DEPRECATED", "DELETED"} {
		r.read = func(ctx context.Context, id string) (RootState, error) {
			return RootState{RootID: id, Lifecycle: state, Generation: 0}, nil
		}
		before := time.Now()
		result, err := s.Root(context.Background(), principal(), binding)
		if err != nil || result.Lifecycle != state || result.LastAdmission != nil || result.ObservedAt.Before(before) || result.EvidenceSource != EvidenceSource {
			t.Fatal(result, err)
		}
		raw, _ = json.Marshal(result)
		_ = json.Unmarshal(raw, &fields)
		if strings.Contains(string(raw), "private-root") {
			t.Fatal("root leaked")
		}
	}
}
func TestObserveAuthorizationScopeMalformedAndPagination(t *testing.T) {
	s, db, r := setup()
	user := principal()
	user.Role = auth.RoleUser
	for _, p := range []auth.Principal{{}, user} {
		if _, e := s.Storage(context.Background(), p, 20, ""); e == nil {
			t.Fatal("auth")
		}
		if _, e := s.Root(context.Background(), p, binding); e == nil {
			t.Fatal("auth")
		}
	}
	for _, id := range []string{"bad", connection} {
		if _, e := s.Root(context.Background(), principal(), id); e == nil {
			t.Fatal("scope")
		}
	}
	for _, n := range []int{0, 51} {
		if _, e := s.Storage(context.Background(), principal(), n, ""); !errors.Is(e, ErrInvalid) {
			t.Fatal(e)
		}
	}
	if db.calls.Load() != 0 || r.calls.Load() != 0 {
		t.Fatal("invalid call reached ports")
	}
	row := db.rows[0]
	row.BindingID = connection
	s, _ = NewService(db, r, []Display{{binding, "a"}, {connection, "b"}})
	db.rows = append(db.rows, row)
	page, e := s.Storage(context.Background(), principal(), 1, "")
	if e != nil || page.NextCursor != binding || len(page.Items) != 1 {
		t.Fatal(page, e)
	}
	db.rows = []Storage{row}
	page, e = s.Storage(context.Background(), principal(), 1, binding)
	if e != nil || page.NextCursor != "" {
		t.Fatal(page, e)
	}
	db.rows = []Storage{row, row}
	if _, e = s.Storage(context.Background(), principal(), 20, ""); !errors.Is(e, ErrUnavailable) {
		t.Fatal("duplicate", e)
	}
	for _, mutate := range []func(*Storage){func(v *Storage) { v.ConnectionID = "" }, func(v *Storage) { v.ProviderType = "https://SECRET" }, func(v *Storage) { v.BindingStatus = "ONLINE" }, func(v *Storage) { v.BindingUpdatedAt = time.Time{} }, func(v *Storage) { v.BindingID = "invalid" }} {
		v := mapping().Storage
		mutate(&v)
		db.rows = []Storage{v}
		if _, e = s.Storage(context.Background(), principal(), 20, ""); !errors.Is(e, ErrUnavailable) {
			t.Fatal(v, e)
		}
	}
	var absent *mappings
	if _, e = NewService(absent, r, []Display{{binding, "a"}}); e == nil {
		t.Fatal("nil")
	}
	for _, scope := range [][]Display{nil, {{binding, ""}}, {{binding, "a"}, {binding, "b"}}, {{"invalid", "a"}}} {
		if _, e = NewService(db, r, scope); e == nil {
			t.Fatal("scope accepted")
		}
	}
}
func TestObserveRejectsMalformedRemappingAndUnknownErrors(t *testing.T) {
	for _, bad := range []RootState{{RootID: "other", Lifecycle: "ACTIVE"}, {RootID: "private-root", Lifecycle: "INVALID"}, {RootID: "private-root", Lifecycle: "ACTIVE", Generation: -1}, {RootID: "private-root", Lifecycle: "ACTIVE", LastAdmission: new(int64(-1))}} {
		s, _, r := setup()
		r.read = func(context.Context, string) (RootState, error) { return bad, nil }
		if _, e := s.Root(context.Background(), principal(), binding); !errors.Is(e, ErrUnavailable) {
			t.Fatal(bad, e)
		}
	}
	for _, err := range []error{ErrRootNotFound, ErrNotReady, context.Canceled, context.DeadlineExceeded, errors.New("Cookie=SECRET https://private.invalid")} {
		s, _, r := setup()
		r.read = func(context.Context, string) (RootState, error) { return RootState{}, err }
		_, e := s.Root(context.Background(), principal(), binding)
		if err == ErrRootNotFound || err == ErrNotReady || err == context.Canceled || err == context.DeadlineExceeded {
			if !errors.Is(e, err) {
				t.Fatal(e)
			}
		} else if e != ErrUnavailable {
			t.Fatal(e)
		}
	}
	s, db, r := setup()
	m := db.m
	db.onGet = func(n int64) Mapping {
		if n == 2 {
			m.RootID = "new-root"
		}
		return m
	}
	if _, e := s.Root(context.Background(), principal(), binding); !errors.Is(e, ErrUnavailable) || r.calls.Load() != 1 {
		t.Fatal("remap accepted", e)
	}
	s, db, r = setup()
	db.m.RootID = ""
	db.m.RootConfigured = false
	if _, e := s.Root(context.Background(), principal(), binding); !errors.Is(e, ErrNotConnected) || r.calls.Load() != 0 {
		t.Fatal(e)
	}
	s, _, _ = setup()
	s.roots = nil
	if _, e := s.Root(context.Background(), principal(), binding); !errors.Is(e, ErrNotConnected) {
		t.Fatal(e)
	}
}
func TestObserveProbeConcurrencyRateAndCancellation(t *testing.T) {
	s, _, r := setup()
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	r.read = func(ctx context.Context, id string) (RootState, error) {
		entered <- struct{}{}
		select {
		case <-release:
			return RootState{RootID: id, Lifecycle: "ACTIVE"}, nil
		case <-ctx.Done():
			return RootState{}, ctx.Err()
		}
	}
	done := make(chan error, 2)
	for range 2 {
		go func() { _, e := s.Root(context.Background(), principal(), binding); done <- e }()
	}
	for range 2 {
		<-entered
	}
	if _, e := s.Root(context.Background(), principal(), binding); !errors.Is(e, ErrBusy) {
		t.Fatal(e)
	}
	close(release)
	for range 2 {
		if e := <-done; e != nil {
			t.Fatal(e)
		}
	}
	if r.calls.Load() != 2 {
		t.Fatal("fanout")
	}
	s, _, r = setup()
	for range 8 {
		if _, e := s.Root(context.Background(), principal(), binding); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := s.Root(context.Background(), principal(), binding); !errors.Is(e, ErrBusy) || r.calls.Load() != 8 {
		t.Fatal(e)
	}
	s.window = time.Now().Add(-time.Minute)
	if _, e := s.Root(context.Background(), principal(), binding); e != nil {
		t.Fatal(e)
	}
	s, db, r := setup()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := s.Root(ctx, principal(), binding); !errors.Is(e, context.Canceled) || db.calls.Load() != 0 {
		t.Fatal(e)
	}
	r.read = func(ctx context.Context, id string) (RootState, error) {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 5*time.Second {
			t.Error("no bounded deadline")
		}
		<-ctx.Done()
		return RootState{}, ctx.Err()
	}
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, e := s.Root(ctx, principal(), binding); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal(e)
	}
}
