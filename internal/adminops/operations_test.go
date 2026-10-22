package adminops

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"strings"
	"testing"
	"time"
)

const aid = "99500000-0000-4000-8000-000000000001"

func admin() Actor {
	return Actor{auth.Principal{UserID: aid, Login: "admin", Role: auth.RoleAdmin}, [32]byte{1}}
}

type opStore struct {
	fresh    bool
	calls    int
	err      error
	capacity Capacity
}

func (s *opStore) Reserve(ctx context.Context, a Actor, c Command, b []string) (Result, bool, error) {
	s.calls++
	if _, ok := ctx.Deadline(); !ok {
		panic("unbounded")
	}
	return Result{Status: "UNKNOWN", ProbeTarget: &ProbeTarget{aid, aid, "secret-ref"}}, s.fresh, s.err
}
func (s *opStore) ApplyUser(context.Context, Actor, Command) (Result, error) {
	s.calls++
	return Result{Status: "SUCCEEDED"}, nil
}
func (s *opStore) ProbeTarget(context.Context, Actor, string) (ProbeTarget, error) {
	return ProbeTarget{aid, aid, "secret-ref"}, nil
}
func (s *opStore) CompleteProbe(_ context.Context, _ Actor, _ Command, _ ProbeTarget, c Capacity) (Result, error) {
	s.capacity = c
	return Result{Capacity: &c}, nil
}
func (s *opStore) Status(context.Context, Actor, string) (Result, error) { return Result{}, s.err }

type probeFunc func(context.Context, ProbeTarget) Capacity

func (f probeFunc) Probe(ctx context.Context, t ProbeTarget) Capacity { return f(ctx, t) }
func TestAdminOperationsAuthorizationIdempotencyAndSafeErrors(t *testing.T) {
	st := &opStore{fresh: true}
	svc, _ := NewService(st, nil, []string{aid})
	c := Command{RequestID: aid, Action: CreateUser, Login: "test-user", Password: "test-password"}
	for _, a := range []Actor{{}, {Principal: auth.Principal{UserID: aid, Login: "test", Role: auth.RoleUser}}, {Principal: admin().Principal}} {
		if _, e := svc.Execute(context.Background(), a, c); e == nil {
			t.Fatal("actor bypass")
		}
	}
	if st.calls != 0 {
		t.Fatal("unauthorized store call")
	}
	if _, e := svc.Execute(context.Background(), admin(), c); e != nil || st.calls != 2 {
		t.Fatal(e, st.calls)
	}
	st.fresh = false
	st.calls = 0
	if _, e := svc.Execute(context.Background(), admin(), c); e != nil || st.calls != 1 {
		t.Fatal("replayed command executed")
	}
	st.err = errors.New("SECRET-cookie-upstream-url")
	if _, e := svc.Execute(context.Background(), admin(), c); e != ErrUnavailable {
		t.Fatal("raw error", e)
	}
	digest := c.Digest()
	c.Password = "different-password"
	if digest != c.Digest() {
		t.Fatal("password-derived digest")
	}
	raw, _ := json.Marshal(c)
	if strings.Contains(string(raw), "password") {
		t.Fatal("password exposed")
	}
}
func TestAccountCapacityBoundaryAndBoundedProbe(t *testing.T) {
	for _, status := range []string{"AUTHENTICATED", "NOT_AUTHENTICATED", "UNKNOWN", "NOT_CONNECTED", "invented"} {
		st := &opStore{fresh: true}
		calls := 0
		p := probeFunc(func(ctx context.Context, tgt ProbeTarget) Capacity {
			calls++
			if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > 8*time.Second {
				t.Fatal("unbounded")
			}
			return Capacity{Status: status, Source: "115_GET_INFO", ProbedAt: time.Now(), TotalBytes: "9007199254740993", UsedBytes: "1", RemainingBytes: "9007199254740992"}
		})
		svc, _ := NewService(st, p, []string{aid})
		_, e := svc.Execute(context.Background(), admin(), Command{Action: ProbeAccount, RequestID: aid, TargetID: aid})
		if e != nil || calls != 1 {
			t.Fatal(e, calls)
		}
		if status == "AUTHENTICATED" {
			if st.capacity.TotalBytes != "9007199254740993" {
				t.Fatal("lossy bytes")
			}
		} else if st.capacity.Status != "UNKNOWN" || st.capacity.TotalBytes != "" {
			t.Fatal("invalid fields accepted")
		}
	}
	for _, c := range []Capacity{{Status: "AUTHENTICATED", TotalBytes: "10", UsedBytes: "11", RemainingBytes: "-1"}, {Status: "AUTHENTICATED", TotalBytes: "10", UsedBytes: "0", RemainingBytes: "0"}, {Status: "UNKNOWN", TotalBytes: "0"}} {
		if validCapacity(c) {
			t.Fatal("bad capacity accepted")
		}
	}
}
