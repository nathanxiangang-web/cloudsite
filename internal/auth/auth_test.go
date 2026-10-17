package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type memoryStore struct {
	user    User
	digest  string
	active  bool
	err     error
	creates int
	ttl     time.Duration
}

func (m *memoryStore) CreateUser(_ context.Context, u User) error { m.user = u; return m.err }
func (m *memoryStore) UserByLogin(_ context.Context, l string) (User, error) {
	if m.err != nil {
		return User{}, m.err
	}
	if l != m.user.Login {
		return User{}, ErrNotFound
	}
	return m.user, nil
}
func (m *memoryStore) CreateSession(_ context.Context, id UserID, digest, hash []byte, ttl time.Duration) (Principal, time.Time, error) {
	if m.err != nil {
		return Principal{}, time.Time{}, m.err
	}
	m.creates++
	m.ttl = ttl
	m.digest = string(digest)
	m.active = true
	return Principal{m.user.ID, m.user.Login, m.user.Role}, time.Now().Add(ttl), nil
}
func (m *memoryStore) SessionPrincipal(_ context.Context, digest []byte) (Principal, error) {
	if m.err != nil {
		return Principal{}, m.err
	}
	if !m.active || string(digest) != m.digest {
		return Principal{}, ErrUnauthorized
	}
	return Principal{m.user.ID, m.user.Login, m.user.Role}, nil
}
func (m *memoryStore) RevokeSession(_ context.Context, digest []byte) error {
	if m.err != nil {
		return m.err
	}
	if string(digest) == m.digest {
		m.active = false
	}
	return nil
}
func TestProductAuthRealHashLoginSessionRevocation(t *testing.T) {
	ctx := context.Background()
	store := &memoryStore{}
	s, err := NewService(store)
	if err != nil {
		t.Fatal(err)
	}
	user, err := s.Provision(ctx, "Nathan", "test-only-password-123", RoleUser)
	if err != nil || !user.Valid() {
		t.Fatalf("provision failed: %v", err)
	}
	if string(store.user.PasswordHash) == "test-only-password-123" || bcrypt.CompareHashAndPassword(store.user.PasswordHash, []byte("test-only-password-123")) != nil {
		t.Fatal("not a real password hash")
	}
	for _, login := range []string{"nobody", "nathan"} {
		if _, err = s.Login(ctx, login, "wrong-password-123"); !errors.Is(err, ErrUnauthorized) {
			t.Fatal("credentials admitted")
		}
	}
	if store.creates != 0 {
		t.Fatal("denied login created a session")
	}
	result, err := s.Login(ctx, "NATHAN", "test-only-password-123")
	if err != nil {
		t.Fatal(err)
	}
	if !ValidToken(result.Token) || store.digest == result.Token || len(store.digest) != 32 {
		t.Fatal("token was not randomly generated and hashed")
	}
	if store.ttl != 30*24*time.Hour || SessionTTL != 30*24*time.Hour {
		t.Fatal("product session is not fixed30 days")
	}
	p, err := s.Authenticate(ctx, result.Token)
	if err != nil || p != user {
		t.Fatal("principal mismatch")
	}
	if err = s.Logout(ctx, result.Token); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Authenticate(ctx, result.Token); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("revoked session admitted")
	}
	if err = s.Logout(ctx, result.Token); err != nil {
		t.Fatal("logout not idempotent")
	}
}
func TestAuthBoundsUnwiredAndFailure(t *testing.T) {
	for _, v := range []string{"abcdef", "123456", "中文密码六字", strings.Repeat("x", 72)} {
		if !ValidPassword(v) {
			t.Fatal("valid six-character policy input rejected")
		}
	}
	if _, err := NewService(nil); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	for _, v := range []string{"", strings.Repeat("x", 5), strings.Repeat("x", 73), "valid-password\x00", "中文密码短", strings.Repeat("中", 25), string([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff})} {
		if ValidPassword(v) {
			t.Fatal("invalid password accepted")
		}
	}
	for _, v := range []string{"", " abcd", "a", "a@b", "中文账号"} {
		if _, err := NormalizeLogin(v); err == nil {
			t.Fatal("invalid login accepted")
		}
	}
	if ValidToken(strings.Repeat("a", 42)) || ValidToken(strings.Repeat("!", 43)) {
		t.Fatal("invalid token")
	}
	m := &memoryStore{err: errors.New("Cookie=SECRET database source text")}
	s, err := NewService(m)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Login(context.Background(), "nathan", "valid-password-123"); err != ErrUnavailable {
		t.Fatal("raw persistence cause escaped")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = s.Provision(ctx, "nathan", "valid-password-123", RoleAdmin); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled creation continued")
	}
	if _, err = s.Authenticate(ctx, strings.Repeat("A", 43)); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestPrincipalCannotBeProviderOrClientRole(t *testing.T) {
	id, err := NewUserID()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []Principal{{"provider-account", "nathan", RoleUser}, {id, "nathan", "premium"}, {id, "invalid/login", RoleAdmin}} {
		if p.Valid() {
			t.Fatal("untrusted identity accepted")
		}
	}
}
