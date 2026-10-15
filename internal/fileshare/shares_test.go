package fileshare

import (
	"context"
	"errors"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/fileaccess"
	"io"
	"strings"
	"testing"
	"time"
)

type fakeStore struct {
	record  Record
	grant   []byte
	revoked bool
	issued  int
}

func (f *fakeStore) Create(_ context.Context, r Record, _ time.Duration) (Record, error) {
	r.CreatedAt = time.Now()
	r.ExpiresAt = r.CreatedAt.Add(time.Hour)
	f.record = r
	return r, nil
}
func (f *fakeStore) List(context.Context, auth.UserID) ([]Record, error) {
	return []Record{f.record}, nil
}
func (f *fakeStore) Revoke(context.Context, auth.UserID, string) error { f.revoked = true; return nil }
func (f *fakeStore) Secret(_ context.Context, h []byte) (Record, error) {
	if f.revoked || string(h) != string(f.record.TokenHash) {
		return Record{}, ErrDenied
	}
	return f.record, nil
}
func (f *fakeStore) IssueGrant(_ context.Context, id string, hash []byte) error {
	if f.revoked || id != f.record.ID {
		return ErrDenied
	}
	f.grant = hash
	f.issued++
	return nil
}
func (f *fakeStore) Granted(_ context.Context, id string, hash []byte) (Record, auth.Principal, error) {
	if f.revoked || id != f.record.ID || string(hash) != string(f.grant) {
		return Record{}, auth.Principal{}, ErrDenied
	}
	return f.record, owner(), nil
}

type fakeFiles struct {
	err    error
	opened int
}

func (f *fakeFiles) Describe(context.Context, auth.Principal, fileaccess.Request) (fileaccess.File, error) {
	return fileaccess.File{Name: "file.pdf", Size: 4}, f.err
}
func (f *fakeFiles) Open(context.Context, auth.Principal, fileaccess.Request) (fileaccess.File, error) {
	f.opened++
	return fileaccess.File{Name: "file.pdf", Size: 4, Content: fileaccess.Content{Body: io.NopCloser(strings.NewReader("test"))}}, f.err
}
func owner() auth.Principal {
	return auth.Principal{UserID: "95000000-0000-4000-8000-000000000001", Login: "test-owner", Role: auth.RoleUser}
}
func TestShareCodeGrantIsolationAndCanonicalRevalidation(t *testing.T) {
	ctx := context.Background()
	store := &fakeStore{}
	files := &fakeFiles{}
	s, _ := New(store, files)
	for _, input := range []struct{ ttl, code string }{{"permanent", ""}, {"1h", "123"}, {"1h", "123 456"}} {
		if _, _, err := s.Create(ctx, owner(), fileaccess.Request{}, input.ttl, input.code); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid share")
		}
	}
	files.err = ErrDenied
	if _, _, err := s.Create(ctx, owner(), fileaccess.Request{}, "1h", ""); err == nil {
		t.Fatal("unavailable canonical shared")
	}
	files.err = nil
	record, token, err := s.Create(ctx, owner(), fileaccess.Request{}, "1h", "code123")
	if err != nil || !record.Protected || !validSecret(token) || string(record.TokenHash) == token || len(record.CodeHash) != 60 {
		t.Fatal("share secret persistence", err)
	}
	if _, err = s.Redeem(ctx, token, "wrong-code"); !errors.Is(err, ErrDenied) || store.issued != 0 {
		t.Fatal("code bypass")
	}
	if _, err = s.Open(ctx, record.ID, token, "", false); !errors.Is(err, ErrDenied) || files.opened != 0 {
		t.Fatal("link token forged grant")
	}
	grant, err := s.Redeem(ctx, token, "code123")
	if err != nil || grant.Token == token {
		t.Fatal("grant issuance", err)
	}
	file, err := s.Open(ctx, record.ID, grant.Token, "", false)
	if err != nil || files.opened != 1 {
		t.Fatal("canonical byte authorization", err)
	}
	file.Content.Body.Close()
	files.err = fileaccess.ErrUnavailable
	if _, err = s.Open(ctx, record.ID, grant.Token, "", false); err == nil {
		t.Fatal("stale canonical bypass")
	}
	files.err = nil
	s.Revoke(ctx, owner(), record.ID)
	if _, err = s.Open(ctx, record.ID, grant.Token, "", false); !errors.Is(err, ErrDenied) {
		t.Fatal("revoke bypass")
	}
}
