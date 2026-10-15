package postgres

import (
	"context"
	"errors"
	"fmt"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/fileaccess"
	"github.com/nathanxiangang-web/panta/internal/fileshare"
	"github.com/nathanxiangang-web/panta/internal/storage"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPostgresProductSharesExpiryRevokeHashAndConcurrentCaps(t *testing.T) {
	ctx := context.Background()
	pool := integrationPool(t, ctx)
	resetTestSchema(t, ctx, pool)
	m, _ := NewMigrator(pool)
	if _, err := m.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	users, _ := NewAuthRepository(pool)
	accounts, _ := auth.NewService(users)
	owner, err := accounts.Provision(ctx, "share-owner", "share-test-password-123", auth.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	other, err := accounts.Provision(ctx, "share-other", "share-test-password-456", auth.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	binding := storage.BindingID("95000000-0000-4000-8000-000000000001")
	seedStorageBinding(t, ctx, pool, binding, "share-root")
	repo, _ := NewShareRepository(pool)
	makeRecord := func(n int) fileshare.Record {
		return fileshare.Record{ID: fmt.Sprintf("95000000-0000-4000-8000-%012d", n), Owner: owner.UserID, Target: fileaccess.Request{BindingID: binding, ResourceID: "file", Path: "/file.pdf"}, Name: "file.pdf", Size: 4, TokenHash: fileshare.SecretHash(fmt.Sprint(n))}
	}
	first, err := repo.Create(ctx, makeRecord(1), time.Hour)
	if err != nil || !first.Active {
		t.Fatal(err)
	}
	var tokenLen int
	if err = pool.QueryRow(ctx, `SELECT octet_length(token_hash) FROM product_file_shares WHERE share_id=$1`, first.ID).Scan(&tokenLen); err != nil || tokenLen != 32 {
		t.Fatal("raw secret persisted", err)
	}
	grant := fileshare.SecretHash("grant-one")
	if err = repo.IssueGrant(ctx, first.ID, grant); err != nil {
		t.Fatal(err)
	}
	_, p, err := repo.Granted(ctx, first.ID, grant)
	if err != nil || p.UserID != owner.UserID {
		t.Fatal("grant owner", err)
	}
	if err = repo.Revoke(ctx, other.UserID, first.ID); !errors.Is(err, fileshare.ErrDenied) {
		t.Fatal("admin bypassed owner")
	}
	pool.Exec(ctx, `UPDATE product_users SET enabled=false WHERE user_id=$1`, string(owner.UserID))
	if _, _, err = repo.Granted(ctx, first.ID, grant); !errors.Is(err, fileshare.ErrDenied) {
		t.Fatal("disabled owner grant")
	}
	pool.Exec(ctx, `UPDATE product_users SET enabled=true WHERE user_id=$1`, string(owner.UserID))
	if err = repo.Revoke(ctx, owner.UserID, first.ID); err != nil {
		t.Fatal(err)
	}
	if err = repo.Revoke(ctx, owner.UserID, first.ID); err != nil {
		t.Fatal("idempotent revoke", err)
	}
	if _, _, err = repo.Granted(ctx, first.ID, grant); !errors.Is(err, fileshare.ErrDenied) {
		t.Fatal("revoked grant")
	}
	if err = repo.IssueGrant(ctx, first.ID, fileshare.SecretHash("another")); !errors.Is(err, fileshare.ErrDenied) {
		t.Fatal("revoked redemption")
	}
	expired, err := repo.Create(ctx, makeRecord(2), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	expiredGrant := fileshare.SecretHash("expired-grant")
	if err = repo.IssueGrant(ctx, expired.ID, expiredGrant); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE product_file_shares SET created_at=clock_timestamp()-interval '2 hours',expires_at=clock_timestamp()-interval '1 minute' WHERE share_id=$1`, expired.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.Secret(ctx, expired.TokenHash); !errors.Is(err, fileshare.ErrDenied) {
		t.Fatal("DB expiry not enforced")
	}
	if _, _, err = repo.Granted(ctx, expired.ID, expiredGrant); !errors.Is(err, fileshare.ErrDenied) {
		t.Fatal("DB expiry grant bypass")
	}
	for n := 3; n < 102; n++ {
		if _, err = repo.Create(ctx, makeRecord(n), time.Hour); err != nil {
			t.Fatal(n, err)
		}
	}
	var group sync.WaitGroup
	var success atomic.Int32
	for n := 102; n < 110; n++ {
		group.Add(1)
		go func(n int) {
			defer group.Done()
			_, err := repo.Create(ctx, makeRecord(n), time.Hour)
			if err == nil {
				success.Add(1)
			} else if !errors.Is(err, fileshare.ErrLimit) {
				t.Error(err)
			}
		}(n)
	}
	group.Wait()
	if success.Load() != 1 {
		t.Fatal("concurrent share cap", success.Load())
	}
	active, err := repo.Secret(ctx, makeRecord(3).TokenHash)
	if err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 99; n++ {
		if err = repo.IssueGrant(ctx, active.ID, fileshare.SecretHash(fmt.Sprint("grant-", n))); err != nil {
			t.Fatal(err)
		}
	}
	success.Store(0)
	for n := 99; n < 107; n++ {
		group.Add(1)
		go func(n int) {
			defer group.Done()
			err := repo.IssueGrant(ctx, active.ID, fileshare.SecretHash(fmt.Sprint("grant-", n)))
			if err == nil {
				success.Add(1)
			} else if !errors.Is(err, fileshare.ErrLimit) {
				t.Error(err)
			}
		}(n)
	}
	group.Wait()
	if success.Load() != 1 {
		t.Fatal("concurrent grant cap", success.Load())
	}
	list, err := repo.List(ctx, other.UserID)
	if err != nil || len(list) != 0 {
		t.Fatal("owner isolation")
	}
}
