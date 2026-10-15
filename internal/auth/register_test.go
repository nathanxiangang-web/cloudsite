package auth

import (
	"context"
	"errors"
	"testing"
)

func TestRegisterAlwaysOrdinaryAndPreservesConflict(t *testing.T) {
	store := &memoryStore{}
	s, err := NewService(store)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.Register(context.Background(), "New.User", "test-only-password-123")
	if err != nil || p.Role != RoleUser || store.user.Role != RoleUser || p.Login != "new.user" || !store.user.Enabled || !store.user.Valid() || store.creates != 0 {
		t.Fatal("invalid self registration")
	}
	store.err = ErrConflict
	if _, err = s.Register(context.Background(), "new.user", "test-only-password-123"); !errors.Is(err, ErrConflict) {
		t.Fatal("conflict changed")
	}
	if _, err = s.Register(context.Background(), "new.user", "short"); !errors.Is(err, ErrInvalid) {
		t.Fatal("invalid password admitted")
	}
}
