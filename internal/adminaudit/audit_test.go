package adminaudit

import (
	"context"
	"errors"
	"github.com/nathanxiangang-web/panta/internal/adminops"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"strings"
	"testing"
	"time"
)

const actorID = "99500000-0000-4000-8000-000000000001"
const bindingID = "99500000-0000-4000-8000-000000000002"
const requestID = "99500000-0000-4000-8000-000000000003"

type readerFake struct {
	snapshot Snapshot
	calls    int
	err      error
	mutate   bool
}

func (r *readerFake) ReadAudit(ctx context.Context, a adminops.Actor, b []string, f Filter, p Position, id string) (Snapshot, error) {
	r.calls++
	if r.mutate {
		b[0] = "mutated"
	}
	return r.snapshot, r.err
}
func actor() adminops.Actor {
	return adminops.Actor{Principal: auth.Principal{UserID: auth.UserID(actorID), Login: "admin", Role: auth.RoleAdmin}, SessionDigest: [32]byte{1}}
}
func record() Record {
	return Record{Item: Item{ActorID: actorID, RequestID: requestID, Action: adminops.CreateUser, Status: "UNKNOWN", Code: "RESERVED_NO_CONFIRMED_RESULT", CreatedAt: time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC), TargetStatus: "NOT_RECORDED"}, Valid: true}
}
func TestAuditScopeFiltersProjectionAndCursor(t *testing.T) {
	a := actor()
	r := &readerFake{snapshot: Snapshot{Items: []Record{record()}, ReadAt: time.Now().UTC().Truncate(time.Microsecond)}, mutate: true}
	s, e := New(r, []string{bindingID})
	if e != nil {
		t.Fatal(e)
	}
	page, e := s.Search(context.Background(), a, Filter{Limit: 1})
	if e != nil || len(page.Items) != 1 || s.bindings[0] != bindingID {
		t.Fatal(page, e)
	}
	for _, f := range []Filter{{Limit: 0}, {Limit: 51}, {Limit: 1, Action: "RAW_SQL"}, {Limit: 1, Status: "RECOVERY_REQUIRED"}, {Limit: 1, Cursor: "bad"}, {Limit: 1, From: record().CreatedAt, Before: record().CreatedAt}} {
		before := r.calls
		if _, e = s.Search(context.Background(), a, f); !errors.Is(e, ErrInvalid) || before != r.calls {
			t.Fatal("invalid filter queried", f, e)
		}
	}
	before := r.calls
	if _, e = s.Search(context.Background(), a, Filter{Limit: 1, ActorID: bindingID}); !errors.Is(e, ErrForbidden) || before != r.calls {
		t.Fatal("foreign actor queried", e)
	}
	if _, e = s.Detail(context.Background(), a, bindingID, requestID); !errors.Is(e, ErrNotFound) || before != r.calls {
		t.Fatal(e)
	}
	for _, change := range []func(*Record){func(v *Record) { v.Valid = false }, func(v *Record) { v.ActorID = bindingID }, func(v *Record) { v.Status = "SUCCEEDED" }, func(v *Record) { v.Code = "Cookie=SECRET" }, func(v *Record) { v.ManifestState = "ACTIVE" }, func(v *Record) { v.TargetStatus = "SECRET" }} {
		v := record()
		change(&v)
		r.snapshot.Items = []Record{v}
		if _, e = s.Search(context.Background(), a, Filter{Limit: 1}); !errors.Is(e, ErrUnavailable) {
			t.Fatal("unsafe projection accepted", v, e)
		}
	}
	r.snapshot.Items = []Record{record()}
	r.err = errors.New("SECRET SQL password")
	if _, e = s.Search(context.Background(), a, Filter{Limit: 1}); !errors.Is(e, ErrUnavailable) || strings.Contains(e.Error(), "SECRET") {
		t.Fatal(e)
	}
}
func TestAuditAuthorizationOptionalPortAndCanceledContext(t *testing.T) {
	if _, e := New((*readerFake)(nil), []string{bindingID}); !errors.Is(e, ErrUnavailable) {
		t.Fatal(e)
	}
	r := &readerFake{}
	s, _ := New(r, []string{bindingID})
	for _, a := range []adminops.Actor{{}, {Principal: auth.Principal{UserID: actorID, Login: "user", Role: auth.RoleUser}, SessionDigest: [32]byte{1}}} {
		if _, e := s.Search(context.Background(), a, Filter{Limit: 1}); e == nil || r.calls != 0 {
			t.Fatal("unauthorized read", e)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := s.Search(ctx, actor(), Filter{Limit: 1}); !errors.Is(e, context.Canceled) || r.calls != 0 {
		t.Fatal("canceled query ran", e)
	}
}
