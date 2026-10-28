package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nathanxiangang-web/panta/internal/adminaudit"
	"github.com/nathanxiangang-web/panta/internal/adminops"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/catalog"
	"github.com/nathanxiangang-web/panta/internal/copypresentation"
	"github.com/nathanxiangang-web/panta/internal/fileaccess"
	"github.com/nathanxiangang-web/panta/internal/fileshare"
	"github.com/nathanxiangang-web/panta/internal/integrations/indexcore"
	"github.com/nathanxiangang-web/panta/internal/product"
	"github.com/nathanxiangang-web/panta/internal/producthttp"
	"github.com/nathanxiangang-web/panta/internal/projector"
	"github.com/nathanxiangang-web/panta/internal/resourceview"
	"github.com/nathanxiangang-web/panta/internal/storage"
	"github.com/nathanxiangang-web/panta/migrations"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func copyProject(t *testing.T, ctx context.Context, pool *pgxpool.Pool, seq int64, state string) {
	t.Helper()
	p, _ := NewProjectionStore(pool)
	if e := p.ApplyBatch(ctx, projector.Batch{StorageBindingID: storage.BindingID(adminBinding), ExpectedCursor: seq - 1, LastEventSeq: seq, Mutations: []projector.Mutation{{CopyID: catalog.CopyID(opID(500)), StorageBindingID: storage.BindingID(adminBinding), IndexCoreRootID: "admin-root", IndexCoreResourceID: "stable-canonical-id", Availability: projector.Availability(state)}}}); e != nil {
		t.Fatal(e)
	}
}
func copyCommand(n int, v int64) adminops.Command {
	return adminops.Command{Action: adminops.SetCopyPresentation, RequestID: opID(n), TargetID: opID(500), ExpectedVersion: v, Presentation: &copypresentation.Change{BindingID: adminBinding, DisplayName: "Private product alias", Category: "DOCUMENT", Hidden: true}}
}
func copyCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, want int) {
	t.Helper()
	var got int
	if e := pool.QueryRow(ctx, `SELECT count(*) FROM product_copy_presentation`).Scan(&got); e != nil || got != want {
		t.Fatal(got, e)
	}
}
func TestPostgresAdminCopyPresentationPersistClearReplayAndSafeAudit(t *testing.T) {
	ctx, pool, _, a, _, repo := opFixture(t)
	copyProject(t, ctx, pool, 1, "PRESENT")
	svc, _ := adminops.NewService(repo, nil, []string{adminBinding})
	before, e := svc.CopyPresentation(ctx, a, opID(500), adminBinding)
	if e != nil || before.Presentation.Version != 0 || before.Evidence.ProjectionCursor != 1 || !before.CanEdit {
		t.Fatal(before, e)
	}
	r := opRun(t, svc, a, copyCommand(1, 0))
	if r.Status != "SUCCEEDED" || r.PresentationVersion == nil || *r.PresentationVersion != 1 {
		t.Fatal(r)
	}
	again := opRun(t, svc, a, copyCommand(1, 0))
	if !again.RecordedAt.Equal(r.RecordedAt) || *again.PresentationVersion != 1 {
		t.Fatal("replay changed version")
	}
	changed := copyCommand(1, 0)
	changed.Presentation.DisplayName = "Other"
	if _, e := svc.Execute(ctx, a, changed); !errors.Is(e, adminops.ErrConflict) {
		t.Fatal(e)
	}
	if r := opRun(t, svc, a, copyCommand(2, 0)); r.Code != "VERSION_CONFLICT" {
		t.Fatal(r)
	}
	d, e := svc.CopyPresentation(ctx, a, opID(500), adminBinding)
	if e != nil || d.Presentation.DisplayName != "Private product alias" || d.Presentation.Version != 1 || !d.Presentation.Hidden || d.VisibilityEffect != "PRESENTATION_ONLY_NO_ACCESS_CONTROL" {
		t.Fatal(d, e)
	}
	copyProject(t, ctx, pool, 2, "REMOVED")
	d, e = svc.CopyPresentation(ctx, a, opID(500), adminBinding)
	if e != nil || d.CanEdit || d.Presentation.Version != 1 || d.Evidence.Availability != "REMOVED" {
		t.Fatal(d, e)
	}
	if r := opRun(t, svc, a, copyCommand(3, 1)); r.Code != "COPY_UNAVAILABLE" {
		t.Fatal(r)
	}
	copyProject(t, ctx, pool, 3, "PRESENT")
	clear := copyCommand(4, 1)
	clear.Presentation.DisplayName = ""
	clear.Presentation.Category = "AUTO"
	clear.Presentation.Hidden = false
	if r := opRun(t, svc, a, clear); r.Status != "SUCCEEDED" {
		t.Fatal(r)
	}
	d, e = svc.CopyPresentation(ctx, a, opID(500), adminBinding)
	if e != nil || d.Presentation.DisplayName != "" || d.Presentation.Version != 2 || d.EmptyNameFallback != "CURRENT_CANONICAL_DISPLAY" || d.Evidence.ResourceID != "stable-canonical-id" || d.Evidence.CopyID != opID(500) {
		t.Fatal(d, e)
	}
	var variant *string
	var assets, providers int
	if e = pool.QueryRow(ctx, `SELECT variant_id::text,(SELECT count(*) FROM assets),(SELECT count(*) FROM acquisition_provider_tasks) FROM copies WHERE copy_id=$1`, opID(500)).Scan(&variant, &assets, &providers); e != nil || variant != nil || assets != 0 || providers != 0 {
		t.Fatal("physical/resource identity changed", e)
	}
	auditRepo, _ := NewAdminAuditRepository(pool)
	audit, _ := adminaudit.New(auditRepo, []string{adminBinding})
	page, e := audit.Search(ctx, a, adminaudit.Filter{Limit: 20, Action: adminops.SetCopyPresentation})
	if e != nil || len(page.Items) != 4 {
		t.Fatal(page, e)
	}
	for _, item := range page.Items {
		b, _ := json.Marshal(item)
		if strings.Contains(string(b), "Private product alias") || strings.Contains(string(b), opID(500)) || item.TargetStatus != "REDACTED" {
			t.Fatal("unsafe audit", string(b))
		}
	}
	var leaked bool
	if e = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM product_admin_action_results WHERE outcome::text LIKE '%Private product alias%' OR outcome ? 'display_name' OR outcome ? 'hidden')`).Scan(&leaked); e != nil || leaked {
		t.Fatal("raw product metadata leaked into receipt", e)
	}
}

func TestPostgresAdminCopyPresentationScopeCorruptionAndAuthentication(t *testing.T) {
	for _, kind := range []string{"outside", "missing", "binding-mismatch", "root-mismatch", "no-projection", "bad-availability", "corrupt-name", "corrupt-binding", "revoked", "expired", "disabled", "demoted", "deleted", "invalid-token", "wrong-login"} {
		t.Run(kind, func(t *testing.T) {
			ctx, pool, _, a, _, repo := opFixture(t)
			copyProject(t, ctx, pool, 1, "PRESENT")
			svc, _ := adminops.NewService(repo, nil, []string{adminBinding})
			c := copyCommand(1, 0)
			expected := adminops.ErrNotFound
			exec := func(sql string, args ...any) {
				if _, e := pool.Exec(ctx, sql, args...); e != nil {
					t.Fatal(e)
				}
			}
			switch kind {
			case "outside":
				c.Presentation.BindingID = adminOutside
			case "missing":
				c.TargetID = opID(501)
			case "binding-mismatch":
				exec(`UPDATE copies SET storage_binding_id=$1 WHERE copy_id=$2`, adminOutside, c.TargetID)
			case "root-mismatch":
				exec(`UPDATE copies SET indexcore_root_id='other-root' WHERE copy_id=$1`, c.TargetID)
			case "no-projection":
				exec(`DELETE FROM indexcore_projection_cursors`)
			case "bad-availability":
				exec(`UPDATE copies SET availability='CORRUPT'`)
				expected = adminops.ErrUnavailable
			case "corrupt-name", "corrupt-binding":
				exec(`INSERT INTO product_copy_presentation VALUES($1,$2,$3,'AUTO',false,1,now(),$4)`, c.TargetID, map[bool]string{true: adminOutside, false: adminBinding}[kind == "corrupt-binding"], map[bool]string{true: "<script>", false: "name"}[kind == "corrupt-name"], string(a.Principal.UserID))
				expected = adminops.ErrUnavailable
			case "revoked":
				exec(`UPDATE product_sessions SET revoked_at=now()`)
				expected = auth.ErrUnauthorized
			case "expired":
				exec(`UPDATE product_sessions SET created_at=now()-interval '2 second',expires_at=now()-interval '1 second'`)
				expected = auth.ErrUnauthorized
			case "disabled":
				exec(`UPDATE product_users SET enabled=false WHERE user_id=$1`, string(a.Principal.UserID))
				expected = adminops.ErrForbidden
			case "demoted":
				exec(`UPDATE product_users SET role='USER' WHERE user_id=$1`, string(a.Principal.UserID))
				expected = adminops.ErrForbidden
			case "deleted":
				exec(`UPDATE product_users SET enabled=false,deleted_at=now(),deleted_by=user_id WHERE user_id=$1`, string(a.Principal.UserID))
				expected = auth.ErrUnauthorized
			case "invalid-token":
				a.SessionDigest = [32]byte{1}
				expected = auth.ErrUnauthorized
			case "wrong-login":
				a.Principal.Login = "wrong"
				expected = adminops.ErrForbidden
			}
			if _, e := svc.CopyPresentation(ctx, a, c.TargetID, c.Presentation.BindingID); !errors.Is(e, expected) {
				t.Fatal(kind, e, expected)
			}
			r, e := svc.Execute(ctx, a, c)
			if errors.Is(expected, adminops.ErrNotFound) {
				if e != nil || r.Code != "COPY_NOT_FOUND" {
					t.Fatal(r, e)
				}
			} else if !errors.Is(e, expected) {
				t.Fatal(r, e, expected)
			}
			if !strings.HasPrefix(kind, "corrupt-") {
				copyCount(t, ctx, pool, 0)
			}
		})
	}
}

func TestPostgresAdminCopyPresentationTwoAdminsAndDuplicateRequests(t *testing.T) {
	for _, same := range []bool{false, true} {
		t.Run(map[bool]string{true: "same-request", false: "two-admins"}[same], func(t *testing.T) {
			ctx, pool, accounts, a, _, repo := opFixture(t)
			copyProject(t, ctx, pool, 1, "PRESENT")
			svc, _ := adminops.NewService(repo, nil, []string{adminBinding})
			b := a
			if !same {
				if _, e := accounts.Provision(ctx, "copy-other-admin", "isolated-password", auth.RoleAdmin); e != nil {
					t.Fatal(e)
				}
				l, e := accounts.Login(ctx, "copy-other-admin", "isolated-password")
				if e != nil {
					t.Fatal(e)
				}
				b = adminops.Actor{Principal: l.Principal, SessionDigest: auth.TokenDigest(l.Token)}
			}
			var wg sync.WaitGroup
			results := make(chan adminops.Result, 2)
			errs := make(chan error, 2)
			start := make(chan struct{})
			for _, actor := range []adminops.Actor{a, b} {
				wg.Add(1)
				go func(a adminops.Actor) {
					defer wg.Done()
					<-start
					r, e := svc.Execute(ctx, a, copyCommand(1, 0))
					results <- r
					errs <- e
				}(actor)
			}
			close(start)
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
				if r.Status == "UNKNOWN" && !same {
					t.Fatal(r)
				}
			}
			for range 2 {
				if e := <-errs; e != nil {
					t.Fatal(e)
				}
			}
			if !same && (wins != 1 || conflicts != 1) {
				t.Fatal(wins, conflicts)
			}
			d, e := svc.CopyPresentation(ctx, a, opID(500), adminBinding)
			if e != nil || d.Presentation.Version != 1 {
				t.Fatal(d, e)
			}
			var n int
			if e = pool.QueryRow(ctx, `SELECT count(*) FROM product_admin_action_results`).Scan(&n); e != nil || n != map[bool]int{true: 1, false: 2}[same] {
				t.Fatal(n, e)
			}
		})
	}
}

func TestPostgresAdminCopyPresentationProjectorAndLifecycleLockOrder(t *testing.T) {
	for _, competitor := range []string{"projector-after-write", "write-after-projector", "revoke-after-write", "delete-author-after-write", "expiry-after-lock"} {
		t.Run(competitor, func(t *testing.T) {
			ctx, pool, _, a, _, repo := opFixture(t)
			copyProject(t, ctx, pool, 1, "PRESENT")
			svc, _ := adminops.NewService(repo, nil, []string{adminBinding})
			c := copyCommand(1, 0)
			if competitor == "write-after-projector" {
				tx, e := pool.Begin(ctx)
				if e != nil {
					t.Fatal(e)
				}
				defer tx.Rollback(ctx)
				if _, e = tx.Exec(ctx, `UPDATE indexcore_projection_cursors SET last_event_seq=2`); e != nil {
					t.Fatal(e)
				}
				if _, e = tx.Exec(ctx, `UPDATE copies SET availability='REMOVED'`); e != nil {
					t.Fatal(e)
				}
				done := make(chan adminops.Result, 1)
				errc := make(chan error, 1)
				go func() { r, e := svc.Execute(ctx, a, c); done <- r; errc <- e }()
				pendingBlocked(t, ctx, pool, tx.Conn().PgConn().PID())
				if e = tx.Commit(ctx); e != nil {
					t.Fatal(e)
				}
				if r := <-done; r.Code != "COPY_UNAVAILABLE" {
					t.Fatal(r)
				}
				if e = <-errc; e != nil {
					t.Fatal(e)
				}
				copyCount(t, ctx, pool, 0)
				return
			}
			entered, release := make(chan uint32, 1), make(chan struct{})
			repo.afterCopyPresentationLock = pendingBarrier(entered, release)
			if competitor == "expiry-after-lock" {
				repo.beforeUserCommit = func(ctx context.Context, tx pgx.Tx) error {
					_, e := tx.Exec(ctx, `UPDATE product_sessions SET created_at=now()-interval '2 second',expires_at=now()-interval '1 second' WHERE token_hash=$1`, a.SessionDigest[:])
					return e
				}
			}
			done := make(chan error, 1)
			go func() { _, e := svc.Execute(ctx, a, c); done <- e }()
			pid := <-entered
			if competitor == "expiry-after-lock" {
				close(release)
				if e := <-done; !errors.Is(e, auth.ErrUnauthorized) {
					t.Fatal(e)
				}
				copyCount(t, ctx, pool, 0)
				return
			}
			other := make(chan error, 1)
			go func() {
				var e error
				if competitor == "projector-after-write" {
					p, _ := NewProjectionStore(pool)
					e = p.ApplyBatch(ctx, projector.Batch{StorageBindingID: storage.BindingID(adminBinding), ExpectedCursor: 1, LastEventSeq: 2, Mutations: []projector.Mutation{{CopyID: catalog.CopyID(opID(501)), StorageBindingID: storage.BindingID(adminBinding), IndexCoreRootID: "admin-root", IndexCoreResourceID: "stable-canonical-id", Availability: projector.AvailabilityRemoved}}})
				} else if competitor == "revoke-after-write" {
					_, e = pool.Exec(ctx, `UPDATE product_sessions SET revoked_at=now() WHERE token_hash=$1`, a.SessionDigest[:])
				} else {
					_, e = pool.Exec(ctx, `UPDATE product_users SET enabled=false,deleted_at=now(),deleted_by=user_id WHERE user_id=$1`, string(a.Principal.UserID))
				}
				other <- e
			}()
			pendingBlocked(t, ctx, pool, pid)
			close(release)
			if e := <-done; e != nil {
				t.Fatal(e)
			}
			if e := <-other; e != nil {
				t.Fatal(e)
			}
			copyCount(t, ctx, pool, 1)
			var v int64
			var id, availability string
			if e := pool.QueryRow(ctx, `SELECT p.admin_version,c.copy_id::text,c.availability FROM copies c JOIN product_copy_presentation p USING(copy_id)`).Scan(&v, &id, &availability); e != nil || v != 1 || id != opID(500) {
				t.Fatal(v, id, e)
			}
			if competitor == "projector-after-write" && availability != "REMOVED" {
				t.Fatal(availability)
			}
		})
	}
}

func TestPostgresAdminCopyPresentationRollbackAndCommitUncertainty(t *testing.T) {
	for _, mode := range []string{"before-receipt", "receipt-insert", "commit-before", "commit-after"} {
		t.Run(mode, func(t *testing.T) {
			ctx, pool, _, a, _, repo := opFixture(t)
			copyProject(t, ctx, pool, 1, "PRESENT")
			c := copyCommand(1, 0)
			switch mode {
			case "before-receipt":
				repo.beforeUserCommit = func(context.Context, pgx.Tx) error { return errors.New("SECRET failure") }
			case "receipt-insert":
				if _, e := pool.Exec(ctx, `CREATE FUNCTION reject_copy_receipt() RETURNS trigger LANGUAGE plpgsql AS 'BEGIN RAISE EXCEPTION ''receipt failure''; END'; CREATE TRIGGER reject_copy_receipt BEFORE INSERT ON product_admin_action_results FOR EACH ROW EXECUTE FUNCTION reject_copy_receipt()`); e != nil {
					t.Fatal(e)
				}
			default:
				repo, _ = NewAdminOperationsRepository(pendingFailDB{pool, mode == "commit-after"})
			}
			real, _ := NewAdminOperationsRepository(pool)
			if _, fresh, e := real.Reserve(ctx, a, c, []string{adminBinding}); e != nil || !fresh {
				t.Fatal(e)
			}
			if _, e := repo.ApplyCopyPresentation(ctx, a, c, []string{adminBinding}); e == nil {
				t.Fatal("fault ignored")
			}
			want := 0
			status := "UNKNOWN"
			if mode == "commit-after" {
				want = 1
				status = "SUCCEEDED"
			}
			copyCount(t, ctx, pool, want)
			svc, _ := adminops.NewService(real, nil, []string{adminBinding})
			r, e := svc.Status(ctx, a, c.RequestID)
			if e != nil || r.Status != status {
				t.Fatal(r, e)
			}
			r = opRun(t, svc, a, c)
			if r.Status != status {
				t.Fatal("uncertain reservation replayed", r)
			}
			copyCount(t, ctx, pool, want)
		})
	}
}

func TestPostgresAdminCopyPresentationMigration19To20Atomic(t *testing.T) {
	ctx := context.Background()
	pool := integrationPool(t, ctx)
	resetTestSchema(t, ctx, pool)
	all, e := migrations.All()
	if e != nil {
		t.Fatal(e)
	}
	m, _ := NewMigrator(pool)
	m.migrations = all[:19]
	if _, e = m.Apply(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e = pool.Exec(ctx, `INSERT INTO product_users(user_id,login,password_hash,role,enabled,created_at,updated_at) VALUES($1,'legacy',decode(repeat('41',60),'hex'),'ADMIN',true,now(),now());`, opID(1)); e != nil {
		t.Fatal(e)
	}
	insert := `INSERT INTO product_admin_actions VALUES($1,$2,$3,$4,decode(repeat('42',32),'hex'),now())`
	if _, e = pool.Exec(ctx, insert, opID(1), opID(2), adminops.CancelPending, opID(500)); e != nil {
		t.Fatal(e)
	}
	var before, after string
	snapshot := `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(u)) FROM product_users u),(SELECT jsonb_agg(to_jsonb(a)) FROM product_admin_actions a),(SELECT jsonb_agg(to_jsonb(r)) FROM product_admin_action_results r))::text`
	if e = pool.QueryRow(ctx, snapshot).Scan(&before); e != nil {
		t.Fatal(e)
	}
	tx, e := pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, all[19].SQL); e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(ctx, insert, opID(1), opID(3), adminops.SetCopyPresentation, opID(500)); e != nil {
		t.Fatal(e)
	}
	if e = tx.Rollback(ctx); e != nil {
		t.Fatal(e)
	}
	var exists bool
	if e = pool.QueryRow(ctx, `SELECT to_regclass('product_copy_presentation') IS NOT NULL`).Scan(&exists); e != nil || exists {
		t.Fatal("DDL rollback incomplete", e)
	}
	if _, e = pool.Exec(ctx, insert, opID(1), opID(3), adminops.SetCopyPresentation, opID(500)); e == nil {
		t.Fatal("allowlist rollback incomplete")
	}
	m, _ = NewMigrator(pool)
	st, e := m.Apply(ctx)
	if e != nil || st.CurrentVersion != 20 || !st.Compatible {
		t.Fatal(st, e)
	}
	if e = pool.QueryRow(ctx, snapshot).Scan(&after); e != nil || before != after {
		t.Fatal("upgrade changed legacy state", e)
	}
	if _, e = pool.Exec(ctx, insert, opID(1), opID(3), adminops.SetCopyPresentation, opID(500)); e != nil {
		t.Fatal(e)
	}
	old, _ := NewMigrator(pool)
	old.migrations = all[:19]
	oldStatus, e := old.Status(ctx)
	if e != nil || oldStatus.Compatible {
		t.Fatal("old binary accepted schema20", e)
	}
}

func TestPostgresAdminCopyPresentationActualProtectedHTTP(t *testing.T) {
	ctx, pool, accounts, a, token, repo := opFixture(t)
	copyProject(t, ctx, pool, 1, "PRESENT")
	svc, _ := adminops.NewService(repo, nil, []string{adminBinding})
	_, _, _, _, reads, _ := adminFixtureNoReset(t, ctx, pool)
	h, e := producthttp.NewWorkspaceHandler(accounts, "http://127.0.0.1:8180", reads, []producthttp.DisplayBinding{{ID: storage.BindingID(adminBinding), Label: "Fixture"}}, 20, producthttp.WithAdminOperations(svc))
	if e != nil {
		t.Fatal(e)
	}
	c := copyCommand(1, 0)
	body := `{"request_id":"` + c.RequestID + `","expected_version":0,"binding_id":"` + adminBinding + `","display_name":"Private product alias","category":"DOCUMENT","hidden":true}`
	path := "/api/v1/admin/copies/" + c.TargetID + "/presentation"
	call := func(method, url, body, cookie string) int {
		req := httptest.NewRequest(method, "http://127.0.0.1:8180"+url, strings.NewReader(body))
		req.Header.Set("Origin", "http://127.0.0.1:8180")
		req.Header.Set("Content-Type", "application/json")
		if cookie != "" {
			req.AddCookie(&http.Cookie{Name: "panta_session", Value: cookie})
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w.Code
	}
	if got := call("GET", path+"?binding_id="+adminBinding, "", token); got != 200 {
		t.Fatal(got)
	}
	if got := call("PUT", path, body, token); got != 200 {
		t.Fatal(got)
	}
	if got := call("GET", "/api/v1/admin/actions/"+c.RequestID, "", token); got != 200 {
		t.Fatal(got)
	}
	if got := call("PUT", path, strings.Replace(body, c.RequestID, opID(2), 1), token); got != 409 {
		t.Fatal(got)
	}
	if got := call("GET", path+"?binding_id="+adminBinding, "", token); got != 200 {
		t.Fatal(got)
	}
	if got := call("GET", path+"?binding_id="+adminOutside, "", token); got != 404 {
		t.Fatal(got)
	}
	if got := call("DELETE", path, "", token); got != 405 {
		t.Fatal(got)
	}
	if got := call("PUT", path, body, ""); got != 401 {
		t.Fatal(got)
	}
	if _, e = pool.Exec(ctx, `UPDATE product_users SET role='USER' WHERE user_id=$1`, string(a.Principal.UserID)); e != nil {
		t.Fatal(e)
	}
	if got := call("PUT", path, body, token); got != 403 && got != 401 {
		t.Fatal(got)
	}
	copyCount(t, ctx, pool, 1)
}

func TestPostgresAdminCopyPresentationCorruptReceiptsFailClosed(t *testing.T) {
	for _, change := range []string{"missing-version", "bad-version", "unexpected-user", "wrong-target", "wrong-action", "terminal-unknown", "wrong-time"} {
		t.Run(change, func(t *testing.T) {
			ctx, pool, _, a, _, repo := opFixture(t)
			copyProject(t, ctx, pool, 1, "PRESENT")
			svc, _ := adminops.NewService(repo, nil, []string{adminBinding})
			c := copyCommand(1, 0)
			opRun(t, svc, a, c)
			sql := ""
			switch change {
			case "missing-version":
				sql = `outcome-'presentation_version'`
			case "bad-version":
				sql = `jsonb_set(outcome,'{presentation_version}','-1')`
			case "unexpected-user":
				sql = `outcome||'{"user":{"login":"SECRET"}}'::jsonb`
			case "wrong-target":
				sql = `jsonb_set(outcome,'{target_id}','"99500000-0000-4000-8000-000000000099"')`
			case "wrong-action":
				sql = `jsonb_set(outcome,'{action}','"CREATE_USER"')`
			case "terminal-unknown":
				sql = `jsonb_set(outcome,'{status}','"UNKNOWN"')`
			case "wrong-time":
				sql = `jsonb_set(outcome,'{recorded_at}','"2000-01-01T00:00:00Z"')`
			}
			if _, e := pool.Exec(ctx, `UPDATE product_admin_action_results SET outcome=`+sql); e != nil {
				t.Fatal(e)
			}
			if _, e := svc.Status(ctx, a, c.RequestID); !errors.Is(e, adminops.ErrUnavailable) {
				t.Fatal("unsafe specific status", e)
			}
			if _, e := svc.Execute(ctx, a, c); !errors.Is(e, adminops.ErrUnavailable) {
				t.Fatal("unsafe duplicate status", e)
			}
			reader, _ := NewAdminAuditRepository(pool)
			audit, _ := adminaudit.New(reader, []string{adminBinding})
			if _, e := audit.Detail(ctx, a, string(a.Principal.UserID), c.RequestID); !errors.Is(e, adminaudit.ErrUnavailable) {
				t.Fatal("unsafe audit read", e)
			}
			copyCount(t, ctx, pool, 1)
		})
	}
}

// Fixture-only Canonical path changes; this port has no mutation method.
type copyCanonicalFixture struct{ path, name string }

func (f *copyCanonicalFixture) item() indexcore.ResourceContext {
	directory := false
	size := int64(4)
	return indexcore.ResourceContext{RootID: "admin-root", ResourceID: "stable-canonical-id", CanonicalPath: &f.path, Name: &f.name, Directory: &directory, SizeBytes: &size, Presence: indexcore.ResourcePresent}
}
func (f *copyCanonicalFixture) Browse(context.Context, indexcore.BrowseRequest) (indexcore.ResourcePage, error) {
	return indexcore.ResourcePage{Items: []indexcore.ResourceContext{f.item()}}, nil
}
func (f *copyCanonicalFixture) Resolve(context.Context, indexcore.ResolveRequest) (indexcore.ResolveResult, error) {
	return indexcore.ResolveResult{Matches: []indexcore.ResourceContext{f.item()}}, nil
}
func TestPostgresAdminCopyPresentationDoesNotChangeConsumerCanonicalOrShareAccess(t *testing.T) {
	ctx, pool, _, a, _, repo := opFixture(t)
	copyProject(t, ctx, pool, 1, "PRESENT")
	fixture := &copyCanonicalFixture{path: "/original.pdf", name: "original.pdf"}
	catalogRepo, _ := NewCatalogRepository(pool)
	physical, _ := resourceview.NewPhysicalService(fixture, catalogRepo)
	tasks, _ := NewProductReadRepository(pool)
	topology, _ := NewStorageRepository(pool)
	reads, e := product.NewReadService(tasks, topology, fixture, physical, catalogRepo, []storage.BindingID{storage.BindingID(adminBinding)})
	if e != nil {
		t.Fatal(e)
	}
	shares, _ := NewShareRepository(pool)
	share, e := shares.Create(ctx, fileshare.Record{ID: opID(600), Owner: a.Principal.UserID, Target: fileaccess.Request{BindingID: storage.BindingID(adminBinding), ResourceID: "stable-canonical-id", Path: fixture.path}, Name: fixture.name, Size: 4, TokenHash: fileshare.SecretHash("synthetic-copy-share")}, time.Hour)
	if e != nil {
		t.Fatal(e)
	}
	grant := fileshare.SecretHash("synthetic-copy-grant")
	if e = shares.IssueGrant(ctx, share.ID, grant); e != nil {
		t.Fatal(e)
	}
	svc, _ := adminops.NewService(repo, nil, []string{adminBinding})
	opRun(t, svc, a, copyCommand(1, 0))
	// Product hidden flag does not revoke the existing grant or rewrite its target.
	record, principal, e := shares.Granted(ctx, share.ID, grant)
	if e != nil || principal.UserID != a.Principal.UserID || record.Target.Path != "/original.pdf" || record.Name != "original.pdf" {
		t.Fatal(record, e)
	}
	for _, name := range []string{"original.pdf", "fresh-canonical-name.pdf"} {
		fixture.name = name
		fixture.path = "/" + name
		page, e := reads.Browse(ctx, a.Principal, product.BrowseRequest{BindingID: storage.BindingID(adminBinding), Limit: 20})
		if e != nil || len(page.Items) != 1 || page.Items[0].Name == nil || *page.Items[0].Name != name {
			t.Fatal(page, e)
		}
		exact, e := reads.Resolve(ctx, a.Principal, storage.BindingID(adminBinding), fixture.path)
		if e != nil || exact.CopyID == nil || string(*exact.CopyID) != opID(500) || exact.Path == nil || *exact.Path != fixture.path {
			t.Fatal(exact, e)
		}
		raw, _ := json.Marshal(exact)
		if strings.Contains(string(raw), "Private product alias") || strings.Contains(string(raw), "hidden") {
			t.Fatal("consumer contract widened", string(raw))
		}
		d, e := svc.CopyPresentation(ctx, a, opID(500), adminBinding)
		if e != nil || d.Presentation.DisplayName != "Private product alias" || d.Presentation.Version != 1 {
			t.Fatal(d, e)
		}
	}
}
