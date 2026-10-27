package adminops

import (
	"context"
	"crypto/sha256"
	"errors"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/copypresentation"
	"testing"
)

type copyPortStore struct {
	Store
	fresh             bool
	reserves, applies int
	mutate            func()
}

func (s *copyPortStore) Reserve(_ context.Context, _ Actor, c Command, _ []string) (Result, bool, error) {
	s.reserves++
	if s.mutate != nil {
		s.mutate()
	}
	return Result{Status: "UNKNOWN"}, s.fresh, nil
}
func (s *copyPortStore) ApplyCopyPresentation(_ context.Context, _ Actor, c Command, _ []string) (Result, error) {
	s.applies++
	if c.Presentation.DisplayName != "Original" {
		panic("intent changed after validation")
	}
	return Result{Status: "SUCCEEDED"}, nil
}
func TestCopyCommandFencesPortIntentAndUnknown(t *testing.T) {
	id := "99500000-0000-4000-8000-000000000001"
	a := Actor{Principal: auth.Principal{UserID: auth.UserID(id), Login: "admin", Role: auth.RoleAdmin}, SessionDigest: [32]byte{1}}
	c := Command{Action: SetCopyPresentation, RequestID: id, TargetID: id, Presentation: &copypresentation.Change{BindingID: id, DisplayName: "Original", Category: "AUTO"}}
	if !c.Valid() {
		t.Fatal("valid command rejected")
	}
	digest := c.Digest()
	c.ExpectedVersion = 1
	if digest == c.Digest() {
		t.Fatal("version not frozen in digest")
	}
	c.ExpectedVersion = 0
	old := Command{Action: SetEnabled, RequestID: id, TargetID: id, ExpectedVersion: 2}
	raw := `{"request_id":"","action":"SET_USER_ENABLED","target_id":"` + id + `","login":"","enabled":false,"expected_version":2}`
	if old.Digest() != sha256.Sum256([]byte(raw)) {
		t.Fatal("existing command digest compatibility changed")
	}
	for _, fresh := range []bool{false, true} {
		p := &copyPortStore{fresh: fresh}
		svc, _ := NewService(p, nil, []string{id})
		p.mutate = func() { c.Presentation.DisplayName = "mutated" }
		r, e := svc.Execute(context.Background(), a, c)
		if e != nil || p.reserves != 1 || p.applies != map[bool]int{false: 0, true: 1}[fresh] {
			t.Fatal(r, e)
		}
		c.Presentation.DisplayName = "Original"
	}
	missing, _ := NewService(&struct{ Store }{}, nil, []string{id})
	if _, e := missing.Execute(context.Background(), a, c); !errors.Is(e, ErrUnavailable) {
		t.Fatal(e)
	}
	for _, change := range []func(*Command){func(c *Command) { c.ExpectedVersion = -1 }, func(c *Command) { c.ExpectedVersion = copypresentation.MaxVersion }, func(c *Command) { c.Login = "other" }, func(c *Command) { c.Pending = &PendingPrecondition{} }, func(c *Command) { c.Presentation = nil }} {
		v := c
		change(&v)
		if v.Valid() {
			t.Fatal(v)
		}
	}
	old.Presentation = c.Presentation
	if old.Valid() {
		t.Fatal("resource intent accepted on user action")
	}
}
