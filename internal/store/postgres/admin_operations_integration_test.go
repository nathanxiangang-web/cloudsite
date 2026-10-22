package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nathanxiangang-web/panta/internal/adminops"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/product"
	"github.com/nathanxiangang-web/panta/internal/producthttp"
	"github.com/nathanxiangang-web/panta/internal/resourceview"
	"github.com/nathanxiangang-web/panta/internal/storage"
	"github.com/nathanxiangang-web/panta/migrations"
)

func opID(n int) string { return fmt.Sprintf("99500000-0000-4000-8000-%012d", n) }
func opFixture(t *testing.T) (context.Context, *pgxpool.Pool, *auth.Service, adminops.Actor, string, *AdminOperationsRepository) {
	t.Helper()
	ctx, pool, _, _, _, _ := adminFixture(t)
	repo, _ := NewAuthRepository(pool)
	accounts, _ := auth.NewService(repo)
	_, e := accounts.Provision(ctx, "ops-admin", "isolated-password", auth.RoleAdmin)
	if e != nil {
		t.Fatal(e)
	}
	login, e := accounts.Login(ctx, "ops-admin", "isolated-password")
	if e != nil {
		t.Fatal(e)
	}
	ops, _ := NewAdminOperationsRepository(pool)
	return ctx, pool, accounts, adminops.Actor{Principal: login.Principal, SessionDigest: auth.TokenDigest(login.Token)}, login.Token, ops
}
func opRun(t *testing.T, s *adminops.Service, a adminops.Actor, c adminops.Command) adminops.Result {
	t.Helper()
	r, e := s.Execute(context.Background(), a, c)
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func TestPostgresAdminOperationsUserAuditCASAndRevocation(t *testing.T) {
	ctx, pool, accounts, a, _, repo := opFixture(t)
	svc, _ := adminops.NewService(repo, nil, []string{adminBinding})
	create := adminops.Command{RequestID: opID(1), Action: adminops.CreateUser, Login: "created-user", Password: "private-test-password"}
	first := opRun(t, svc, a, create)
	if first.Status != "SUCCEEDED" || first.User == nil || !first.User.Enabled || first.User.Version != 0 {
		t.Fatal(first)
	}
	replay := opRun(t, svc, a, create)
	if replay.TargetID != first.TargetID || !replay.RecordedAt.Equal(first.RecordedAt) {
		t.Fatal("non-idempotent creation")
	}
	create.RequestID = opID(2)
	if r := opRun(t, svc, a, create); r.Status != "REJECTED" || r.Code != "CONFLICT" {
		t.Fatal(r)
	}
	login, e := accounts.Login(ctx, create.Login, create.Password)
	if e != nil {
		t.Fatal(e)
	}
	disabled := opRun(t, svc, a, adminops.Command{RequestID: opID(3), Action: adminops.SetEnabled, TargetID: first.TargetID, ExpectedVersion: 0, Enabled: false})
	if disabled.User == nil || disabled.User.Enabled || disabled.User.Version != 1 {
		t.Fatal(disabled)
	}
	if _, e = accounts.Authenticate(ctx, login.Token); !errors.Is(e, auth.ErrUnauthorized) {
		t.Fatal("old session survived disable", e)
	}
	if _, e = accounts.Login(ctx, create.Login, create.Password); !errors.Is(e, auth.ErrUnauthorized) {
		t.Fatal("disabled login")
	}
	enabled := opRun(t, svc, a, adminops.Command{RequestID: opID(4), Action: adminops.SetEnabled, TargetID: first.TargetID, ExpectedVersion: 1, Enabled: true})
	if enabled.User == nil || !enabled.User.Enabled {
		t.Fatal(enabled)
	}
	if _, e = accounts.Authenticate(ctx, login.Token); !errors.Is(e, auth.ErrUnauthorized) {
		t.Fatal("enable resurrected revoked session")
	}
	for i, target := range []string{string(a.Principal.UserID), opID(999)} {
		r := opRun(t, svc, a, adminops.Command{RequestID: opID(10 + i), Action: adminops.SetEnabled, TargetID: target, Enabled: false})
		if r.Status != "REJECTED" {
			t.Fatal("admin/self bypass")
		}
	}
	// Two distinct administrators can race the same USER, but only one version wins.
	_, e = accounts.Provision(ctx, "other-admin", "isolated-password", auth.RoleAdmin)
	if e != nil {
		t.Fatal(e)
	}
	secondLogin, _ := accounts.Login(ctx, "other-admin", "isolated-password")
	b := adminops.Actor{Principal: secondLogin.Principal, SessionDigest: auth.TokenDigest(secondLogin.Token)}
	results := make(chan adminops.Result, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i, actor := range []adminops.Actor{a, b} {
		wg.Add(1)
		go func(i int, actor adminops.Actor) {
			defer wg.Done()
			r, e := svc.Execute(ctx, actor, adminops.Command{RequestID: opID(100 + i), Action: adminops.SetEnabled, TargetID: first.TargetID, ExpectedVersion: 2, Enabled: false})
			results <- r
			errs <- e
		}(i, actor)
	}
	wg.Wait()
	close(results)
	wins, conflicts := 0, 0
	for r := range results {
		if r.Status == "SUCCEEDED" {
			wins++
		}
		if r.Code == "VERSION_CONFLICT" {
			conflicts++
		}
	}
	for i := 0; i < 2; i++ {
		if e := <-errs; e != nil {
			t.Fatal(e)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatal(wins, conflicts)
	}
	var leaked bool
	e = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM product_admin_action_results WHERE outcome::text LIKE '%private-test-password%' OR outcome::text LIKE '%password_hash%' OR outcome::text LIKE '%token_hash%')`).Scan(&leaked)
	if e != nil || leaked {
		t.Fatal("audit leaked", e)
	}
	// Actor role/session changes are checked again inside the write transaction.
	_, e = pool.Exec(ctx, `UPDATE product_users SET role='USER' WHERE user_id=$1`, string(a.Principal.UserID))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = svc.Execute(ctx, a, create); !errors.Is(e, adminops.ErrForbidden) {
		t.Fatal("stale role accepted", e)
	}
}

type opProbe func(context.Context, adminops.ProbeTarget) adminops.Capacity

func (f opProbe) Probe(ctx context.Context, t adminops.ProbeTarget) adminops.Capacity {
	return f(ctx, t)
}
func TestPostgresAdminOperationsProbeCooldownMappingAndUnknown(t *testing.T) {
	ctx, pool, _, a, _, repo := opFixture(t)
	_, e := pool.Exec(ctx, `UPDATE storage_connections SET provider_type='115',credential_ref='SECRET-REF',status='ACTIVE' WHERE storage_connection_id=(SELECT storage_connection_id FROM storage_bindings WHERE storage_binding_id=$1)`, adminBinding)
	if e != nil {
		t.Fatal(e)
	}
	calls := atomic.Int32{}
	entered, release := make(chan struct{}), make(chan struct{})
	p := opProbe(func(ctx context.Context, target adminops.ProbeTarget) adminops.Capacity {
		calls.Add(1)
		close(entered)
		<-release
		return adminops.Capacity{Status: "AUTHENTICATED", Source: "115_GET_INFO", ProbedAt: time.Now().UTC(), TotalBytes: "9007199254740993", UsedBytes: "1", RemainingBytes: "9007199254740992"}
	})
	svc, _ := adminops.NewService(repo, p, []string{adminBinding})
	c := adminops.Command{RequestID: opID(1), Action: adminops.ProbeAccount, TargetID: adminBinding}
	done := make(chan error, 1)
	go func() { _, e := svc.Execute(ctx, a, c); done <- e }()
	<-entered
	r := opRun(t, svc, a, c)
	if r.Status != "UNKNOWN" {
		t.Fatal("pending replay")
	}
	other := c
	other.RequestID = opID(2)
	r = opRun(t, svc, a, other)
	if r.Code != "RATE_LIMITED" || calls.Load() != 1 {
		t.Fatal("parallel probe escaped", r)
	}
	// Change only this synthetic binding after the remote read. No stale success.
	_, e = pool.Exec(ctx, `UPDATE storage_bindings SET status='DISABLED' WHERE storage_binding_id=$1`, adminBinding)
	if e != nil {
		t.Fatal(e)
	}
	close(release)
	if e = <-done; e != nil {
		t.Fatal(e)
	}
	r, e = svc.Status(ctx, a, c.RequestID)
	if e != nil || r.Capacity == nil || r.Capacity.Status != "UNKNOWN" || r.Capacity.TotalBytes != "" {
		t.Fatal("stale mapping accepted", r, e)
	}
	opRun(t, svc, a, c)
	if calls.Load() != 1 {
		t.Fatal("replayed remote call")
	}
	outside := c
	outside.RequestID = opID(3)
	outside.TargetID = adminOutside
	r = opRun(t, svc, a, outside)
	if r.Status != "REJECTED" || calls.Load() != 1 {
		t.Fatal("scope bypass")
	}
	raw, _ := json.Marshal(r)
	if strings.Contains(string(raw), "SECRET") {
		t.Fatal("credential leaked")
	}
}
func TestPostgresAdminOperationsAtomicFailureAndCrashReservations(t *testing.T) {
	ctx, pool, accounts, a, _, repo := opFixture(t)
	svc, _ := adminops.NewService(repo, nil, []string{adminBinding})
	created := opRun(t, svc, a, adminops.Command{RequestID: opID(1), Action: adminops.CreateUser, Login: "rollback-user", Password: "rollback-password"})
	login, _ := accounts.Login(ctx, "rollback-user", "rollback-password")
	c := adminops.Command{RequestID: opID(2), Action: adminops.SetEnabled, TargetID: created.TargetID, ExpectedVersion: 0, Enabled: false}
	// Crash after durable reservation: replay never executes it later.
	if _, fresh, e := repo.Reserve(ctx, a, c, []string{adminBinding}); e != nil || !fresh {
		t.Fatal(e)
	}
	if r := opRun(t, svc, a, c); r.Status != "UNKNOWN" {
		t.Fatal("abandoned intent replayed")
	}
	// Failure to append result rolls back USER state and session revocation.
	c.RequestID = opID(3)
	if _, fresh, e := repo.Reserve(ctx, a, c, []string{adminBinding}); e != nil || !fresh {
		t.Fatal(e)
	}
	if _, e := pool.Exec(ctx, `ALTER TABLE product_admin_action_results ADD CONSTRAINT fixture_fail CHECK(false) NOT VALID`); e != nil {
		t.Fatal(e)
	}
	if _, e := repo.ApplyUser(ctx, a, c); e == nil {
		t.Fatal("audit failure accepted")
	}
	if _, e := accounts.Authenticate(ctx, login.Token); e != nil {
		t.Fatal("partial session revocation", e)
	}
	var enabled bool
	var version int64
	if e := pool.QueryRow(ctx, `SELECT enabled,admin_version FROM product_users WHERE user_id=$1`, created.TargetID).Scan(&enabled, &version); e != nil || !enabled || version != 0 {
		t.Fatal("partial state", e)
	}
	if _, e := pool.Exec(ctx, `ALTER TABLE product_admin_action_results DROP CONSTRAINT fixture_fail`); e != nil {
		t.Fatal(e)
	}
	r, e := svc.Status(ctx, a, c.RequestID)
	if e != nil || r.Status != "UNKNOWN" {
		t.Fatal("uncommitted success", r, e)
	}
	// Real backend disconnect during the transaction also rolls back all writes.
	c.RequestID = opID(4)
	repo.beforeUserCommit = func(ctx context.Context, tx pgx.Tx) error {
		var pid int
		if e := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); e != nil {
			return e
		}
		_, e := pool.Exec(ctx, `SELECT pg_terminate_backend($1)`, pid)
		return e
	}
	if _, e = svc.Execute(ctx, a, c); e == nil {
		t.Fatal("terminated transaction succeeded")
	}
	repo.beforeUserCommit = nil
	if _, e = accounts.Authenticate(ctx, login.Token); e != nil {
		t.Fatal("crash left partial revoke", e)
	}
}
func TestPostgresAdminOperationsMigration15To16AndRollback(t *testing.T) {
	ctx := context.Background()
	pool := integrationPool(t, ctx)
	resetTestSchema(t, ctx, pool)
	all, e := migrations.All()
	if e != nil {
		t.Fatal(e)
	}
	// Only isolated transactional DDL; no down-migration is shipped or run live.
	for _, m := range all[:15] {
		if _, e = pool.Exec(ctx, m.SQL); e != nil {
			t.Fatal(m.Version, e)
		}
	}
	if _, e = pool.Exec(ctx, `INSERT INTO product_users(user_id,login,password_hash,role,enabled,created_at,updated_at) VALUES($1,'migrated',decode(repeat('41',60),'hex'),'USER',true,now(),now())`, opID(1)); e != nil {
		t.Fatal(e)
	}
	tx, e := pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, all[15].SQL); e != nil {
		t.Fatal(e)
	}
	var v int64
	if e = tx.QueryRow(ctx, `SELECT admin_version FROM product_users`).Scan(&v); e != nil || v != 0 {
		t.Fatal(e)
	}
	if e = tx.Rollback(ctx); e != nil {
		t.Fatal(e)
	}
	var name *string
	if e = pool.QueryRow(ctx, `SELECT to_regclass('public.product_admin_actions')::text`).Scan(&name); e != nil || name != nil {
		t.Fatal("migration rollback", name, e)
	}
	var count int
	if e = pool.QueryRow(ctx, `SELECT count(*) FROM product_users WHERE login='migrated' AND enabled AND password_hash=decode(repeat('41',60),'hex')`).Scan(&count); e != nil || count != 1 {
		t.Fatal("legacy data changed", e)
	}
}
func TestPostgresAdminOperationsHTTPActualSession(t *testing.T) {
	ctx, pool, accounts, a, token, repo := opFixture(t)
	svc, _ := adminops.NewService(repo, nil, []string{adminBinding})
	_, _, _, _, reads, _ := adminFixtureNoReset(t, ctx, pool)
	h, e := producthttp.NewWorkspaceHandler(accounts, "http://127.0.0.1:8180", reads, []producthttp.DisplayBinding{{ID: storage.BindingID(adminBinding), Label: "Synthetic"}}, 16, producthttp.WithAdminOperations(svc))
	if e != nil {
		t.Fatal(e)
	}
	call := func(token, origin, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "http://127.0.0.1:8180/api/v1/admin/users", strings.NewReader(body))
		req.Host = "127.0.0.1:8180"
		req.Header.Set("Origin", origin)
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: producthttp.SessionCookieName, Value: token})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}
	body := `{"request_id":"` + opID(1) + `","login":"http-user","password":"test-password"}`
	if w := call(token, "https://evil.invalid", body); w.Code != 403 {
		t.Fatal(w.Code)
	}
	if w := call(token, "http://127.0.0.1:8180", body); w.Code != 200 || !strings.Contains(w.Body.String(), "USER_CREATED") {
		t.Fatal(w.Code, w.Body.String())
	}
	if _, e = pool.Exec(ctx, `UPDATE product_users SET enabled=false WHERE user_id=$1`, string(a.Principal.UserID)); e != nil {
		t.Fatal(e)
	}
	if w := call(token, "http://127.0.0.1:8180", body); w.Code != 401 {
		t.Fatal("disabled actor", w.Code)
	}
}

func TestPostgresAdminOperationsConcurrentLoginCannotEscapeDisable(t *testing.T) {
	ctx, pool, accounts, a, _, repo := opFixture(t)
	svc, _ := adminops.NewService(repo, nil, []string{adminBinding})
	u := opRun(t, svc, a, adminops.Command{RequestID: opID(1), Action: adminops.CreateUser, Login: "concurrent-user", Password: "isolated-password"})
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	repo.beforeUserCommit = func(context.Context, pgx.Tx) error { close(entered); <-release; return nil }
	disabled := make(chan error, 1)
	go func() {
		_, e := svc.Execute(ctx, a, adminops.Command{RequestID: opID(2), Action: adminops.SetEnabled, TargetID: u.TargetID, ExpectedVersion: 0, Enabled: false})
		disabled <- e
	}()
	<-entered
	loginResult := make(chan error, 1)
	go func() { _, e := accounts.Login(ctx, "concurrent-user", "isolated-password"); loginResult <- e }()
	// Observe the actual session-creation SELECT blocked on the USER row lock.
	locked := false
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if e := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE wait_event_type='Lock' AND query LIKE 'SELECT user_id::text,login,role FROM product_users%')`).Scan(&locked); e != nil {
			t.Fatal(e)
		}
		if locked {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	releaseOnce.Do(func() { close(release) })
	if e := <-disabled; e != nil {
		t.Fatal(e)
	}
	if e := <-loginResult; !errors.Is(e, auth.ErrUnauthorized) {
		t.Fatal("concurrent session escaped disable", e)
	}
	if !locked {
		t.Fatal("did not observe real lock coordination")
	}
}

// Reuse accepted product reads without resetting the fixture used by auth.
func adminFixtureNoReset(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (context.Context, *pgxpool.Pool, any, any, *product.ReadService, any) {
	tasks, _ := NewProductReadRepository(pool)
	topology, _ := NewStorageRepository(pool)
	catalog, _ := NewCatalogRepository(pool)
	physical, _ := resourceview.NewPhysicalService(productIndexStub{}, catalog)
	reads, e := product.NewReadService(tasks, topology, productIndexStub{}, physical, catalog, []storage.BindingID{storage.BindingID(adminBinding)})
	if e != nil {
		t.Fatal(e)
	}
	return ctx, pool, nil, nil, reads, nil
}
