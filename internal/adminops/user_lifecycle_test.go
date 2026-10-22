package adminops

import (
	"context"
	"encoding/json"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"strings"
	"testing"
)

func TestUserLifecycleTypedCommands(t *testing.T) {
	for _, action := range []string{RenameUser, ResetPassword, DeleteUser, RestoreUser} {
		t.Run(action, func(t *testing.T) {
			c := Command{RequestID: aid, TargetID: aid, Action: action, ExpectedVersion: 2}
			if action == RenameUser {
				c.Login = "changed-user"
			}
			if action == ResetPassword || action == RestoreUser {
				c.Password = "new-secret-password"
			}
			if !c.Valid() {
				t.Fatal("valid command rejected")
			}
			bad := c
			bad.ExpectedVersion = -1
			if bad.Valid() {
				t.Fatal("negative version")
			}
			bad = c
			bad.TargetID = "bad"
			if bad.Valid() {
				t.Fatal("invalid target")
			}
			bad = c
			bad.Enabled = true
			if bad.Valid() {
				t.Fatal("cross-action enabled accepted")
			}
			bad = c
			bad.RequestID = ""
			if bad.Valid() {
				t.Fatal("missing idempotency key")
			}
			b, _ := json.Marshal(c)
			if strings.Contains(string(b), "new-secret-password") {
				t.Fatal("password serialized")
			}
			old := c.Digest()
			c.Password = "different-secret"
			if c.Digest() != old {
				t.Fatal("password-derived ledger digest")
			}
			store := &opStore{fresh: true}
			s, _ := NewService(store, nil, []string{aid})
			if _, e := s.Execute(context.Background(), admin(), bad); e != ErrInvalid || store.calls != 0 {
				t.Fatal("invalid command reached port")
			}
			actor := admin()
			actor.Principal.Role = auth.RoleUser
			if _, e := s.Execute(context.Background(), actor, c); e != ErrForbidden || store.calls != 0 {
				t.Fatal("USER command reached port")
			}
		})
	}
}
