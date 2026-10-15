package postgres

import (
	"context"
	"errors"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"sync"
	"testing"
)

func TestPostgresRegistrationConcurrentUniqueUserAndExistingPassword(t *testing.T) {
	ctx := context.Background()
	pool := integrationPool(t, ctx)
	resetTestSchema(t, ctx, pool)
	m, _ := NewMigrator(pool)
	if _, err := m.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	repo, _ := NewAuthRepository(pool)
	service, err := auth.NewService(repo)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := service.Register(ctx, "New.User", "abcdef")
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	created, conflict := 0, 0
	for err := range results {
		if err == nil {
			created++
		} else if errors.Is(err, auth.ErrConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if created != 1 || conflict != 1 {
		t.Fatal("unique registration race")
	}
	if _, err = service.Register(ctx, "new.user", "another-password-123"); !errors.Is(err, auth.ErrConflict) {
		t.Fatal("duplicate reset")
	}
	result, err := service.Login(ctx, "new.user", "abcdef")
	if err != nil || result.Principal.Role != auth.RoleUser {
		t.Fatal("registered ordinary user login failed")
	}
	if _, err = service.Login(ctx, "new.user", "another-password-123"); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatal("duplicate changed original password")
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM product_users WHERE login='new.user' AND role='USER' AND enabled`).Scan(&count); err != nil || count != 1 {
		t.Fatal("invalid persisted registration")
	}
}
