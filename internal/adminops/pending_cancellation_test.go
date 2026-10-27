package adminops

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

type pendingStore struct {
	opStore
	applied  int
	bindings []string
}

func (s *pendingStore) ApplyPendingCancellation(_ context.Context, _ Actor, _ Command, b []string) (Result, error) {
	s.applied++
	s.bindings = append([]string(nil), b...)
	b[0] = "mutated"
	return Result{Status: "SUCCEEDED"}, s.err
}
func pendingCommand() Command {
	return Command{Action: CancelPending, RequestID: aid, TargetID: aid, Pending: &PendingPrecondition{OwnerUserID: aid, BindingID: aid, UpdatedAt: time.Date(2026, 10, 26, 1, 0, 0, 123000, time.UTC)}}
}
func TestPendingCancellationCommandValidationDigestAndOptionalPort(t *testing.T) {
	c := pendingCommand()
	if !c.Valid() {
		t.Fatal("valid command refused")
	}
	// Historical request digest bytes stay unchanged for every older action.
	old := Command{RequestID: aid, Action: RevokeShare, TargetID: aid}
	legacy := `{"request_id":"","action":"REVOKE_SHARE","target_id":"` + aid + `","login":"","enabled":false,"expected_version":0}`
	if old.Digest() != sha256.Sum256([]byte(legacy)) {
		t.Fatal("legacy digest changed")
	}
	d := c.Digest()
	c.Pending.UpdatedAt = c.Pending.UpdatedAt.In(time.FixedZone("offset", 3600))
	if c.Digest() != d {
		t.Fatal("equivalent database time changed intent")
	}
	c.Pending.OwnerUserID = "99500000-0000-4000-8000-000000000002"
	if c.Digest() == d {
		t.Fatal("owner not bound")
	}
	for _, mutate := range []func(*Command){func(c *Command) { c.Pending = nil }, func(c *Command) { c.Pending.UpdatedAt = time.Time{} }, func(c *Command) { c.Pending.UpdatedAt = c.Pending.UpdatedAt.Add(time.Nanosecond) }, func(c *Command) { c.Pending.BindingID = "bad" }, func(c *Command) { c.Action = RevokeShare }, func(c *Command) { c.ExpectedVersion = 1 }} {
		c = pendingCommand()
		mutate(&c)
		if c.Valid() {
			t.Fatal("invalid tuple accepted", c)
		}
	}
	missing := &opStore{fresh: true}
	s, _ := NewService(missing, nil, []string{aid})
	if _, e := s.Execute(context.Background(), admin(), pendingCommand()); !errors.Is(e, ErrUnavailable) || missing.calls != 0 {
		t.Fatal("missing port reserved/applied", e)
	}
	st := &pendingStore{opStore: opStore{fresh: true}}
	s, _ = NewService(st, nil, []string{aid})
	if _, e := s.Execute(context.Background(), admin(), pendingCommand()); e != nil || st.applied != 1 || st.calls != 1 || st.bindings[0] != aid || s.bindings[0] != aid {
		t.Fatal("wrong port or scope", e)
	}
	st.fresh = false
	if r, e := s.Execute(context.Background(), admin(), pendingCommand()); e != nil || r.Status != "UNKNOWN" || st.applied != 1 {
		t.Fatal("UNKNOWN replayed", r, e)
	}
	raw, _ := json.Marshal(Result{Manifest: &ManifestOutcome{State: "CANCELED"}})
	if string(raw) == "" {
		t.Fatal("empty receipt")
	}
}
