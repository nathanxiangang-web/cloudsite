package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nathanxiangang-web/panta/internal/acquisition"
	"github.com/nathanxiangang-web/panta/internal/adminread"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/jobs"
	"github.com/nathanxiangang-web/panta/internal/product"
	"github.com/nathanxiangang-web/panta/internal/producthttp"
	"github.com/nathanxiangang-web/panta/internal/resourceview"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

const adminBinding = "98100000-0000-4000-8000-000000000001"
const adminOutside = "98100000-0000-4000-8000-000000000002"

func adminUUID(n int) string { return fmt.Sprintf("98200000-0000-4000-8000-%012d", n) }

type adminTestDB struct {
	pool    *pgxpool.Pool
	queries int
	tx      pgx.Tx
	options pgx.TxOptions
}
type adminTestTx struct {
	pgx.Tx
	db *adminTestDB
}

func (d *adminTestDB) BeginTx(ctx context.Context, options pgx.TxOptions) (pgx.Tx, error) {
	tx, err := d.pool.BeginTx(ctx, options)
	if err != nil {
		return nil, err
	}
	d.options = options
	d.tx = tx
	return &adminTestTx{Tx: tx, db: d}, nil
}
func (t *adminTestTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	t.db.queries++
	return t.Tx.Query(ctx, sql, args...)
}
func adminFixture(t *testing.T) (context.Context, *pgxpool.Pool, *adminTestDB, *AdminReadRepository, *adminread.Service, auth.Principal) {
	t.Helper()
	ctx := context.Background()
	pool := integrationPool(t, ctx)
	resetTestSchema(t, ctx, pool)
	m, _ := NewMigrator(pool)
	if _, err := m.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	seedStorageBinding(t, ctx, pool, storage.BindingID(adminBinding), "admin-root")
	seedStorageBinding(t, ctx, pool, storage.BindingID(adminOutside), "outside-root")
	db := &adminTestDB{pool: pool}
	repo, _ := NewAdminReadRepository(db)
	service, err := adminread.NewService(repo, []string{adminBinding})
	if err != nil {
		t.Fatal(err)
	}
	return ctx, pool, db, repo, service, auth.Principal{UserID: auth.UserID(adminUUID(1)), Login: "fixture-admin", Role: auth.RoleAdmin}
}
func TestPostgresAdminThousandsKeysetProjectionAndScope(t *testing.T) {
	ctx, pool, db, repo, s, p := adminFixture(t)
	const total = 2307
	// Fixture-only bulk writes; the application adapter has no write methods.
	_, err := pool.Exec(ctx, `INSERT INTO product_users(user_id,login,password_hash,role,enabled,created_at,updated_at)
 SELECT ('98200000-0000-4000-8000-'||lpad(n::text,12,'0'))::uuid,'fixture-'||n,decode(repeat('41',60),'hex'),CASE WHEN n%3=0 THEN 'ADMIN' ELSE 'USER' END,n%2=0,now(),now() FROM generate_series(1,$1::int) n`, total)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO acquisition_manifests(manifest_id,user_id,target_storage_binding_id,target_path,source_type,source_ref,expected_name,state,created_at,updated_at)
 SELECT ('98300000-0000-4000-8000-'||lpad(n::text,12,'0'))::uuid,('98200000-0000-4000-8000-'||lpad((n%2+1)::text,12,'0'))::uuid,$2,'/','https','https://SECRET.invalid/download?token=SECRET','SECRET-NAME','PENDING',now(),now() FROM generate_series(1,$1::int) n`, total, adminBinding)
	if err != nil {
		t.Fatal(err)
	}
	manifests, _ := NewAcquisitionManifestRepository(pool)
	now := time.Now().UTC()
	owner := acquisition.UserID(p.UserID)
	for i, item := range []struct {
		owner   *acquisition.UserID
		binding string
	}{{nil, adminBinding}, {&owner, adminOutside}} {
		if err = manifests.CreateManifest(ctx, acquisition.Manifest{ID: acquisition.ManifestID(adminUUID(8000 + i)), UserID: item.owner, TargetStorageBindingID: storage.BindingID(item.binding), TargetPath: "/", SourceType: "https", SourceRef: "https://SECRET.invalid", State: acquisition.StatePending, CreatedAt: now, UpdatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	for _, kind := range []string{"users", "downloads"} {
		cursor := ""
		seen := map[string]bool{}
		pages := 0
		disabled, admins := 0, 0
		for {
			before := db.queries
			var next string
			if kind == "users" {
				page, e := s.Users(ctx, p, 50, cursor)
				if e != nil {
					t.Fatal(e)
				}
				for _, row := range page.Items {
					if seen[row.UserID] {
						t.Fatal("duplicate user")
					}
					seen[row.UserID] = true
					if !row.Enabled {
						disabled++
					}
					if row.Role == auth.RoleAdmin {
						admins++
					}
				}
				next = page.NextCursor
			} else {
				page, e := s.Downloads(ctx, p, 50, cursor, "")
				if e != nil {
					t.Fatal(e)
				}
				for _, row := range page.Items {
					if seen[row.ManifestID] {
						t.Fatal("duplicate download")
					}
					seen[row.ManifestID] = true
					raw, _ := json.Marshal(row)
					if strings.Contains(string(raw), "SECRET") || strings.Contains(string(raw), "source_ref") {
						t.Fatal("secret projection")
					}
				}
				next = page.NextCursor
			}
			if db.queries-before != 1 || db.options.AccessMode != pgx.ReadOnly || db.options.IsoLevel != pgx.RepeatableRead {
				t.Fatal("unbounded/N+1/non-read-only query")
			}
			pages++
			if next == "" {
				break
			}
			if next <= cursor {
				t.Fatal("cursor did not advance")
			}
			cursor = next
		}
		if len(seen) != total || pages != 47 {
			t.Fatal(kind, len(seen), pages)
		}
		if kind == "users" && (disabled == 0 || admins == 0) {
			t.Fatal("mixed accounts missing")
		}
	}
	filtered, e := s.Downloads(ctx, p, 50, "", string(p.UserID))
	if e != nil || len(filtered.Items) != 50 {
		t.Fatal(e)
	}
	for _, row := range filtered.Items {
		if row.OwnerUserID != string(p.UserID) {
			t.Fatal("owner filter")
		}
	}
	empty, e := s.Downloads(ctx, p, 20, "", adminUUID(9999))
	if e != nil || len(empty.Items) != 0 || empty.NextCursor != "" {
		t.Fatal("empty filter", e)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = s.Users(canceled, p, 20, ""); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	repo.afterRead = func(ctx context.Context) error {
		_, err := db.tx.Exec(ctx, `UPDATE product_users SET enabled=false`)
		return err
	}
	if _, e = s.Users(ctx, p, 20, ""); !errors.Is(e, adminread.ErrUnavailable) {
		t.Fatal("read-only transaction allowed write", e)
	}
	repo.afterRead = nil
	var enabled int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM product_users WHERE enabled`).Scan(&enabled); err != nil || enabled != total/2 {
		t.Fatal("read mutated users", enabled, err)
	}
}
func TestPostgresAdminSnapshotCorruptLink(t *testing.T) {
	ctx, pool, db, repo, s, p := adminFixture(t)
	now := time.Now().UTC()
	owner := acquisition.UserID(p.UserID)
	id := acquisition.ManifestID(adminUUID(100))
	job := jobs.JobID(adminUUID(101))
	manifests, _ := NewAcquisitionManifestRepository(pool)
	if err := manifests.CreateManifest(ctx, acquisition.Manifest{ID: id, UserID: &owner, TargetStorageBindingID: storage.BindingID(adminBinding), TargetPath: "/", SourceType: "https", SourceRef: "https://SECRET.invalid", State: acquisition.StatePending, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	activationRepo, _ := NewAcquisitionActivationRepository(pool)
	activation, _ := acquisition.NewActivationService(activationRepo)
	if _, err := activation.Activate(ctx, acquisition.ActivateRequest{ManifestID: id, JobID: job, MaxAttempts: 3}); err != nil {
		t.Fatal(err)
	}
	// Concurrent paired writer commits after the single SELECT, before reader commit.
	repo.afterRead = func(ctx context.Context) error {
		tx, e := pool.Begin(ctx)
		if e != nil {
			return e
		}
		defer func() { _ = tx.Rollback(context.Background()) }()
		if _, e = tx.Exec(ctx, `UPDATE acquisition_manifests SET state='FAILED',updated_at=clock_timestamp() WHERE manifest_id=$1`, string(id)); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE jobs SET state='FAILED',claim_attempts=7,attempt_count=2,last_error='Cookie=SECRET',updated_at=clock_timestamp() WHERE job_id=$1`, string(job)); e != nil {
			return e
		}
		return tx.Commit(ctx)
	}
	before := db.queries
	page, err := s.Downloads(ctx, p, 20, "", "")
	if err != nil || len(page.Items) != 1 || page.Items[0].ManifestState != "ACTIVE" || *page.Items[0].JobState != "QUEUED" || db.queries-before != 1 {
		t.Fatal("mixed snapshot", page, err)
	}
	repo.afterRead = nil
	page, err = s.Downloads(ctx, p, 20, "", "")
	if err != nil || page.Items[0].ManifestState != "FAILED" || *page.Items[0].JobState != "FAILED" || page.Items[0].ClaimAttempts != 7 || page.Items[0].FailureCount != 2 {
		t.Fatal("new snapshot/counter semantics", page, err)
	}
	for _, sql := range []string{`UPDATE jobs SET payload='{}' WHERE job_id=$1`, `UPDATE jobs SET payload=jsonb_build_object('schema_version',1,'manifest_id','bad') WHERE job_id=$1`, `UPDATE jobs SET payload=payload||'{"extra":true}'::jsonb WHERE job_id=$1`, `UPDATE jobs SET payload=jsonb_set(payload,'{schema_version}','1.0'::jsonb) WHERE job_id=$1`, `UPDATE jobs SET job_type='OTHER' WHERE job_id=$1`, `UPDATE jobs SET idempotency_key='wrong' WHERE job_id=$1`, `UPDATE jobs SET attempt_count=4 WHERE job_id=$1`} {
		if _, err = pool.Exec(ctx, sql, string(job)); err != nil {
			t.Fatal(err)
		}
		if _, err = s.Downloads(ctx, p, 20, "", ""); !errors.Is(err, adminread.ErrUnavailable) {
			t.Fatal("broken link accepted", err)
		}
		if _, err = pool.Exec(ctx, `UPDATE jobs SET payload=jsonb_build_object('schema_version',1,'manifest_id',$2::text),job_type='ACQUISITION',idempotency_key='acquisition:'||$2::text,attempt_count=2 WHERE job_id=$1`, string(job), string(id)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = pool.Exec(ctx, `UPDATE jobs SET state='QUEUED' WHERE job_id=$1`, string(job)); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Downloads(ctx, p, 20, "", ""); !errors.Is(err, adminread.ErrUnavailable) {
		t.Fatal("mismatched states accepted", err)
	}
	// Deliberate corruption only in this isolated fixture: emulate a missing FK row.
	if _, err = pool.Exec(ctx, `ALTER TABLE acquisition_manifests DROP CONSTRAINT acquisition_manifests_job_id_fkey`); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE acquisition_manifests SET job_id=$2 WHERE manifest_id=$1`, string(id), adminUUID(9999)); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Downloads(ctx, p, 20, "", ""); !errors.Is(err, adminread.ErrUnavailable) {
		t.Fatal("missing job accepted", err)
	}
}

func TestPostgresAdminHTTPFreshIdentityAndOwnerBoundary(t *testing.T) {
	ctx, pool, _, _, service, _ := adminFixture(t)
	authRepo, _ := NewAuthRepository(pool)
	accounts, err := auth.NewService(authRepo)
	if err != nil {
		t.Fatal(err)
	}
	a, err := accounts.Provision(ctx, "fixture-admin", "test-only-password", auth.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	b, err := accounts.Provision(ctx, "fixture-user", "test-only-password", auth.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	al, err := accounts.Login(ctx, a.Login, "test-only-password")
	if err != nil {
		t.Fatal(err)
	}
	bl, err := accounts.Login(ctx, b.Login, "test-only-password")
	if err != nil {
		t.Fatal(err)
	}
	topology, _ := NewStorageRepository(pool)
	catalogRepo, _ := NewCatalogRepository(pool)
	ownerRepo, _ := NewProductReadRepository(pool)
	physical, _ := resourceview.NewPhysicalService(productIndexStub{}, catalogRepo)
	ownerReads, err := product.NewReadService(ownerRepo, topology, productIndexStub{}, physical, catalogRepo, []storage.BindingID{storage.BindingID(adminBinding)})
	if err != nil {
		t.Fatal(err)
	}
	handler, err := producthttp.NewWorkspaceHandler(accounts, "http://127.0.0.1:8180", ownerReads, []producthttp.DisplayBinding{{ID: storage.BindingID(adminBinding), Label: "Fixture"}}, 15, producthttp.WithAdminReads(service))
	if err != nil {
		t.Fatal(err)
	}
	manifests, _ := NewAcquisitionManifestRepository(pool)
	owner := acquisition.UserID(b.UserID)
	now := time.Now().UTC()
	if err = manifests.CreateManifest(ctx, acquisition.Manifest{ID: acquisition.ManifestID(adminUUID(100)), UserID: &owner, TargetStorageBindingID: storage.BindingID(adminBinding), TargetPath: "/", SourceType: "https", SourceRef: "https://SECRET.invalid/source", ExpectedName: new("SECRET-NAME"), State: acquisition.StatePending, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	get := func(path, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "http://127.0.0.1:8180"+path, nil)
		r.Header.Set("X-Role", "ADMIN")
		if token != "" {
			r.AddCookie(&http.Cookie{Name: producthttp.SessionCookieName, Value: token})
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	for _, route := range []string{"users", "downloads"} {
		for _, tc := range []struct {
			token string
			want  int
		}{{"", 401}, {bl.Token, 403}, {al.Token, 200}} {
			w := get("/api/v1/admin/"+route, tc.token)
			if w.Code != tc.want || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal(route, w.Code, w.Body.String())
			}
			for _, secret := range []string{"SECRET", "password_hash", "source_ref", "lease_owner", "provider_ref", "payload"} {
				if strings.Contains(w.Body.String(), secret) {
					t.Fatal("secret leaked", secret)
				}
			}
		}
	}
	w := get("/api/v1/admin/downloads?user_id="+string(b.UserID), al.Token)
	if w.Code != 200 || !strings.Contains(w.Body.String(), string(b.UserID)) {
		t.Fatal("admin cross-owner read missing", w.Body.String())
	}
	w = get("/api/v1/tasks", al.Token)
	if w.Code != 200 || strings.Contains(w.Body.String(), adminUUID(100)) {
		t.Fatal("owner endpoint changed", w.Body.String())
	}
	w = get("/api/v1/tasks/"+adminUUID(100), al.Token)
	if w.Code != 404 {
		t.Fatal("admin adopted owner", w.Code)
	}
	if _, err = pool.Exec(ctx, `UPDATE product_users SET role='USER' WHERE user_id=$1`, string(a.UserID)); err != nil {
		t.Fatal(err)
	}
	for _, route := range []string{"users", "downloads"} {
		if w = get("/api/v1/admin/"+route, al.Token); w.Code != 403 {
			t.Fatal("persisted role downgrade bypass", w.Code)
		}
	}
	if _, err = pool.Exec(ctx, `UPDATE product_users SET role='ADMIN',enabled=false WHERE user_id=$1`, string(a.UserID)); err != nil {
		t.Fatal(err)
	}
	for _, route := range []string{"users", "downloads"} {
		if w = get("/api/v1/admin/"+route, al.Token); w.Code != 401 {
			t.Fatal("disabled account bypass", w.Code)
		}
	}
}

func TestPostgresAdminQueryCancellationAndInvalidRepositoryCalls(t *testing.T) {
	ctx, pool, db, _, service, p := adminFixture(t)
	blocker, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = blocker.Rollback(context.Background()) }()
	if _, err = blocker.Exec(ctx, `LOCK TABLE product_users IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}
	deadline, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	if _, err = service.Users(deadline, p, 20, ""); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("blocked read ignored deadline", err)
	}
	if err = blocker.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if page, e := service.Users(ctx, p, 20, ""); e != nil || len(page.Items) != 0 {
		t.Fatal("reader did not recover after cancellation", page, e)
	}
	repo, _ := NewAdminReadRepository(db)
	before := db.queries
	for _, limit := range []int{0, 52} {
		if _, err = repo.ListUsers(ctx, limit, ""); !errors.Is(err, adminread.ErrInvalid) {
			t.Fatal("unbounded repository read", err)
		}
	}
	for _, q := range []struct {
		bindings      []string
		cursor, owner string
	}{{nil, "", ""}, {[]string{"invalid"}, "", ""}, {[]string{adminBinding}, "offset:20", ""}, {[]string{adminBinding}, "", "invalid"}} {
		if _, err = repo.ListDownloads(ctx, q.bindings, 21, q.cursor, q.owner); !errors.Is(err, adminread.ErrInvalid) {
			t.Fatal("invalid repository input", err)
		}
	}
	if before != db.queries {
		t.Fatal("invalid input queried database")
	}
	var absent *pgxpool.Pool
	if _, err = NewAdminReadRepository(absent); err == nil {
		t.Fatal("typed nil database accepted")
	}
}
