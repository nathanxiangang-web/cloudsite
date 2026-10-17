package postgres

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nathanxiangang-web/panta/internal/acquisition"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/producthttp"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

func TestPostgresAuthSessionExpiryRevocationIdentityAndHTTP(t *testing.T) {
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
	u, err := service.Provision(ctx, "nathan", "test-only-password-123", auth.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Provision(ctx, "NATHAN", "new-test-password-123", auth.RoleAdmin); !errors.Is(err, auth.ErrConflict) {
		t.Fatal("duplicate reset permitted")
	}
	if _, err = service.Login(ctx, "nathan", "new-test-password-123"); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatal("duplicate changed password")
	}
	result, err := service.Login(ctx, "nathan", "test-only-password-123")
	if err != nil {
		t.Fatal(err)
	}
	p, err := service.Authenticate(ctx, result.Token)
	if err != nil || p != u {
		t.Fatal("user/session identity mismatch")
	}
	digest := auth.TokenDigest(result.Token)
	var storedLen int
	var match bool
	if err = pool.QueryRow(ctx, `SELECT octet_length(token_hash),token_hash=$1 FROM product_sessions WHERE user_id=$2`, digest[:], string(u.UserID)).Scan(&storedLen, &match); err != nil || storedLen != 32 || !match {
		t.Fatal("not a hashed session")
	}
	var duration float64
	var expires time.Time
	if err = pool.QueryRow(ctx, `SELECT extract(epoch FROM expires_at-created_at),expires_at FROM product_sessions WHERE token_hash=$1`, digest[:]).Scan(&duration, &expires); err != nil || duration != 2592000 || !expires.Equal(result.ExpiresAt) {
		t.Fatal("DB-time session expiry is not30 days")
	}
	if _, err = pool.Exec(ctx, `UPDATE product_users SET role='ADMIN' WHERE user_id=$1`, string(u.UserID)); err != nil {
		t.Fatal(err)
	}
	p, err = service.Authenticate(ctx, result.Token)
	if err != nil || p.Role != auth.RoleAdmin {
		t.Fatal("stale role claims used")
	}
	if _, err = pool.Exec(ctx, `UPDATE product_users SET enabled=false WHERE user_id=$1`, string(u.UserID)); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Authenticate(ctx, result.Token); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatal("disabled account admitted")
	}
	if _, err = pool.Exec(ctx, `UPDATE product_users SET enabled=true WHERE user_id=$1`, string(u.UserID)); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE product_sessions SET created_at=clock_timestamp()-interval '2 hours',expires_at=clock_timestamp()-interval '1 minute' WHERE token_hash=$1`, digest[:]); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Authenticate(ctx, result.Token); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatal("DB expired session admitted")
	}
	if _, _, err = repo.CreateSession(ctx, u.UserID, digest[:], []byte("stale hash"), auth.SessionTTL); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatal("stale password authorization admitted")
	}
	handler, _ := producthttp.NewAuthHandler(service, "http://127.0.0.1:8180")
	r := httptest.NewRequest("POST", "http://127.0.0.1:8180/api/v1/auth/login", strings.NewReader(`{"login":"nathan","password":"test-only-password-123"}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "http://127.0.0.1:8180")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 200 || len(w.Result().Cookies()) != 1 {
		t.Fatal("real HTTP login failed")
	}
	cookie := w.Result().Cookies()[0]
	if cookie.MaxAge != 2592000 || !cookie.HttpOnly {
		t.Fatal("HTTP cookie does not preserve30-day contract")
	}
	r = httptest.NewRequest("POST", "http://127.0.0.1:8180/api/v1/auth/logout", nil)
	r.Header.Set("Origin", "http://127.0.0.1:8180")
	r.AddCookie(cookie)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 204 {
		t.Fatal("HTTP logout failed")
	}
	for attempt := 0; attempt < 2; attempt++ {
		if err = service.Logout(ctx, cookie.Value); err != nil {
			t.Fatal("revoke not idempotent")
		}
	}
	r = httptest.NewRequest("GET", "http://127.0.0.1:8180/api/v1/auth/me", nil)
	r.AddCookie(cookie)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("revoked HTTP session admitted")
	}
	if _, err = pool.Exec(ctx, `UPDATE product_users SET role='PREMIUM' WHERE user_id=$1`, string(u.UserID)); err == nil {
		t.Fatal("unsupported role accepted")
	}
}
func TestPostgresAuthFixedLifetimePreservesLegacyAndDoesNotSlide(t *testing.T) {
	ctx := context.Background()
	pool := integrationPool(t, ctx)
	resetTestSchema(t, ctx, pool)
	m, _ := NewMigrator(pool)
	if _, err := m.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	repo, _ := NewAuthRepository(pool)
	s, _ := auth.NewService(repo)
	if _, err := s.Provision(ctx, "session.user", "test-only-password-123", auth.RoleUser); err != nil {
		t.Fatal(err)
	}
	legacy, err := s.Login(ctx, "session.user", "test-only-password-123")
	if err != nil {
		t.Fatal(err)
	}
	digest := auth.TokenDigest(legacy.Token)
	// Simulate a previously issued1-hour session only in this owned test DB.
	var original time.Time
	if err = pool.QueryRow(ctx, `UPDATE product_sessions SET expires_at=created_at+interval '1 hour' WHERE token_hash=$1 RETURNING expires_at`, digest[:]).Scan(&original); err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 2; n++ {
		if _, err = s.Authenticate(ctx, legacy.Token); err != nil {
			t.Fatal(err)
		}
	}
	var after time.Time
	if err = pool.QueryRow(ctx, `SELECT expires_at FROM product_sessions WHERE token_hash=$1`, digest[:]).Scan(&after); err != nil || !after.Equal(original) {
		t.Fatal("legacy session silently extended")
	}
	fresh, err := s.Login(ctx, "session.user", "test-only-password-123")
	if err != nil {
		t.Fatal(err)
	}
	freshDigest := auth.TokenDigest(fresh.Token)
	if _, err = s.Authenticate(ctx, fresh.Token); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT expires_at FROM product_sessions WHERE token_hash=$1`, freshDigest[:]).Scan(&after); err != nil || !after.Equal(fresh.ExpiresAt) {
		t.Fatal("new session slides on read")
	}
	if _, err = pool.Exec(ctx, `UPDATE product_sessions SET created_at=clock_timestamp()-interval '31 days',expires_at=clock_timestamp()-interval '1 second' WHERE token_hash=$1`, freshDigest[:]); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Authenticate(ctx, fresh.Token); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatal("expired30-day session accepted")
	}
}
func TestPostgresAuthConcurrentTokensAndCancellation(t *testing.T) {
	ctx := context.Background()
	pool := integrationPool(t, ctx)
	resetTestSchema(t, ctx, pool)
	m, _ := NewMigrator(pool)
	if _, err := m.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	repo, _ := NewAuthRepository(pool)
	s, _ := auth.NewService(repo)
	u, err := s.Provision(ctx, "operator", "test-only-password-123", auth.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	const count = 8
	tokens := make(chan string, count)
	errs := make(chan error, count)
	var wg sync.WaitGroup
	for n := 0; n < count; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := s.Login(ctx, "operator", "test-only-password-123")
			if err != nil {
				errs <- err
				return
			}
			tokens <- result.Token
		}()
	}
	wg.Wait()
	close(tokens)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for token := range tokens {
		if seen[token] {
			t.Fatal("nonce reused")
		}
		seen[token] = true
		p, err := s.Authenticate(ctx, token)
		if err != nil || p.UserID != u.UserID {
			t.Fatal("wrong owner")
		}
	}
	if len(seen) != count {
		t.Fatal("session loss")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = s.Login(cancelled, "operator", "test-only-password-123"); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled login proceeded")
	}
	var stored int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM product_sessions").Scan(&stored); err != nil || stored != count {
		t.Fatal("cancelled login created a session")
	}
}
func TestPostgresAuthAppendUpgradePreservesLegacyAttribution(t *testing.T) {
	ctx := context.Background()
	pool := integrationPool(t, ctx)
	resetTestSchema(t, ctx, pool)
	m, _ := NewMigrator(pool)
	all := m.migrations
	m.migrations = all[:12]
	if _, err := m.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	var tables bool
	if err := pool.QueryRow(ctx, "SELECT to_regclass('public.product_users') IS NULL").Scan(&tables); err != nil || !tables {
		t.Fatal("auth before append")
	}
	var history int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&history); err != nil || history != 12 {
		t.Fatal("legacy history changed")
	}
	bindingID := storage.BindingID("94000000-0000-4000-8000-000000000001")
	seedStorageBinding(t, ctx, pool, bindingID, "legacy-auth-upgrade")
	manifests, err := NewAcquisitionManifestRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	legacyID := acquisition.UserID("94000000-0000-4000-8000-000000000099")
	now := time.Now().UTC().Truncate(time.Microsecond)
	before := []acquisition.Manifest{
		{ID: "94000000-0000-4000-8000-000000000010", SourceType: "http", SourceRef: "https://example.invalid/legacy", TargetStorageBindingID: bindingID, TargetPath: "/", State: acquisition.StatePending, CreatedAt: now, UpdatedAt: now},
		{ID: "94000000-0000-4000-8000-000000000011", UserID: &legacyID, SourceType: "http", SourceRef: "https://example.invalid/legacy-owner", TargetStorageBindingID: bindingID, TargetPath: "/", State: acquisition.StatePending, CreatedAt: now, UpdatedAt: now},
	}
	for _, manifest := range before {
		if err := manifests.CreateManifest(ctx, manifest); err != nil {
			t.Fatal(err)
		}
	}
	m.migrations = all
	status, err := m.Apply(ctx)
	if err != nil || status.CurrentVersion != CurrentSchemaVersion {
		t.Fatal("upgrade failed")
	}
	for _, manifest := range before {
		got, err := manifests.GetManifest(ctx, manifest.ID)
		if err != nil {
			t.Fatal(err)
		}
		assertManifestRoundTrip(t, got, manifest)
	}
	var defaults bool
	if err = pool.QueryRow(ctx, `SELECT column_default IS NULL FROM information_schema.columns WHERE table_name='acquisition_manifests' AND column_name='user_id'`).Scan(&defaults); err != nil || !defaults {
		t.Fatal("legacy attribution default added")
	}
	var foreign int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM pg_constraint WHERE conrelid='acquisition_manifests'::regclass AND contype='f' AND confrelid='product_users'::regclass`).Scan(&foreign); err != nil || foreign != 0 {
		t.Fatal("legacy ownership backfilled/constrained")
	}
}
