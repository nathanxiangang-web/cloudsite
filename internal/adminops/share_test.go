package adminops

import (
	"context"
	"testing"

	"github.com/nathanxiangang-web/panta/internal/auth"
)

type shareStoreFake struct {
	opStore
	shareCalls int
	command    Command
}

func (s *shareStoreFake) ApplyShare(_ context.Context, _ Actor, c Command) (Result, error) {
	s.shareCalls++
	s.command = c
	return Result{RequestID: c.RequestID, Action: c.Action, TargetID: c.TargetID, Status: "SUCCEEDED", Code: "SHARE_REVOKED"}, nil
}

func TestShareRevokeTypedCommandAndDispatch(t *testing.T) {
	valid := Command{RequestID: aid, Action: RevokeShare, TargetID: aid}
	if !valid.Valid() {
		t.Fatal("valid revoke rejected")
	}
	for _, bad := range []Command{
		{RequestID: aid, Action: RevokeShare},
		{RequestID: aid, Action: RevokeShare, TargetID: "not-a-uuid"},
		{RequestID: aid, Action: RevokeShare, TargetID: aid, ExpectedVersion: 1},
		{RequestID: aid, Action: RevokeShare, TargetID: aid, Login: "user"},
		{RequestID: aid, Action: RevokeShare, TargetID: aid, Password: "secret-password"},
		{RequestID: aid, Action: RevokeShare, TargetID: aid, Enabled: true},
	} {
		if bad.Valid() {
			t.Fatal("invalid revoke accepted", bad)
		}
	}
	st := &shareStoreFake{opStore: opStore{fresh: true}}
	svc, _ := NewService(st, nil, []string{aid})
	r, e := svc.Execute(context.Background(), admin(), valid)
	if e != nil || st.shareCalls != 1 || r.Code != "SHARE_REVOKED" || r.Action != RevokeShare || r.TargetID != aid || st.command.Action != RevokeShare {
		t.Fatal(r, e, st.shareCalls, st.command)
	}
	// A store without the share port fails closed instead of silently succeeding.
	plain, _ := NewService(&opStore{fresh: true}, nil, []string{aid})
	if _, e = plain.Execute(context.Background(), admin(), valid); e != ErrUnavailable {
		t.Fatal("missing share port accepted", e)
	}
	p := admin()
	p.Principal.Role = auth.RoleUser
	if _, e = svc.Execute(context.Background(), p, valid); e == nil || st.shareCalls != 1 {
		t.Fatal("non-admin reached share port", e)
	}
}
