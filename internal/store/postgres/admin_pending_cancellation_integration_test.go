package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nathanxiangang-web/panta/internal/acquisition"
	"github.com/nathanxiangang-web/panta/internal/adminops"
	"github.com/nathanxiangang-web/panta/internal/adminread"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/jobs"
	"github.com/nathanxiangang-web/panta/internal/producthttp"
	"github.com/nathanxiangang-web/panta/internal/storage"
	"github.com/nathanxiangang-web/panta/migrations"
)

func pendingInput(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id string, n int) adminops.Command {
	t.Helper()
	c := adminops.Command{Action: adminops.CancelPending, TargetID: id, RequestID: opID(n), Pending: &adminops.PendingPrecondition{}}
	if e := pool.QueryRow(ctx, `SELECT user_id::text,target_storage_binding_id::text,updated_at FROM acquisition_manifests WHERE manifest_id=$1`, id).Scan(&c.Pending.OwnerUserID, &c.Pending.BindingID, &c.Pending.UpdatedAt); e != nil {
		t.Fatal(e)
	}
	return c
}
func pendingAssert(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id, state string, wantJobs, wantResults int) {
	t.Helper()
	var got string
	var n int
	if e := pool.QueryRow(ctx, `SELECT state FROM acquisition_manifests WHERE manifest_id=$1`, id).Scan(&got); e != nil || got != state {
		t.Fatal("state", got, e)
	}
	for _, check := range []struct {
		sql  string
		want int
	}{{`SELECT count(*) FROM jobs`, wantJobs}, {`SELECT count(*) FROM acquisition_provider_tasks`, 0}, {`SELECT count(*) FROM product_admin_action_results`, wantResults}} {
		if e := pool.QueryRow(ctx, check.sql).Scan(&n); e != nil || n != check.want {
			t.Fatal(check.sql, n, check.want, e)
		}
	}
}

// Barrier observes a real blocked database backend, not elapsed sleep time.
func pendingBlocked(t *testing.T, ctx context.Context, pool *pgxpool.Pool, pid uint32) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		var blocked bool
		e := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1::int=ANY(pg_blocking_pids(pid)))`, int(pid)).Scan(&blocked)
		if e != nil {
			t.Fatal(e)
		}
		if blocked {
			return
		}
		select {
		case <-deadline.C:
			t.Fatal("no competing database lock waiter")
		case <-tick.C:
		}
	}
}
func pendingBarrier(entered chan<- uint32, release <-chan struct{}) func(context.Context, pgx.Tx) error {
	return func(ctx context.Context, tx pgx.Tx) error {
		entered <- tx.Conn().PgConn().PID()
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func TestPostgresAdminPendingCancelReceiptReplayAndReads(t *testing.T) {
	ctx, pool, accounts, a, _, repo := opFixture(t)
	u, e := accounts.Provision(ctx, "pending-owner", "test-password", auth.RoleUser)
	if e != nil {
		t.Fatal(e)
	}
	owner := string(u.UserID)
	id := overviewSeed(t, ctx, pool, 1, adminBinding, &owner, "PENDING", "")
	c := pendingInput(t, ctx, pool, id, 1)
	svc, _ := adminops.NewService(repo, nil, []string{adminBinding})
	r := opRun(t, svc, a, c)
	if r.Code != "PENDING_CANCELED" || r.Status != "SUCCEEDED" || r.Manifest == nil || r.Manifest.State != "CANCELED" || r.Manifest.JobID != nil || !r.Manifest.UpdatedAt.After(c.Pending.UpdatedAt) {
		t.Fatal(r)
	}
	first, _ := json.Marshal(r)
	replay := opRun(t, svc, a, c)
	second, _ := json.Marshal(replay)
	if string(first) != string(second) {
		t.Fatal("terminal receipt changed")
	}
	status, e := svc.Status(ctx, a, c.RequestID)
	raw, _ := json.Marshal(status)
	if e != nil || string(raw) != string(first) {
		t.Fatal("status not exact", e)
	}
	changed := c
	p := *c.Pending
	p.UpdatedAt = p.UpdatedAt.Add(time.Microsecond)
	changed.Pending = &p
	if _, e = svc.Execute(ctx, a, changed); !errors.Is(e, adminops.ErrConflict) {
		t.Fatal("different intent accepted", e)
	}
	rd, _ := NewAdminReadRepository(pool)
	reads, _ := adminread.NewService(rd, []string{adminBinding})
	detail, e := reads.DownloadDetail(ctx, a.Principal, id)
	if e != nil || detail.ManifestState != "CANCELED" || detail.JobID != nil || detail.MaxAttempts != 0 {
		t.Fatal(detail, e)
	}
	page, e := reads.SearchDownloads(ctx, a.Principal, adminread.DownloadFilter{Limit: 10, Status: "CANCELED"})
	if e != nil || len(page.Items) != 1 {
		t.Fatal(page, e)
	}
	overview, e := reads.Overview(ctx, a.Principal, owner)
	if e != nil || overview.Manifests.Canceled != 1 || overview.Manifests.CanceledWithoutJob != 1 || overview.Jobs.Total != 0 || overview.Manifests.Incomplete != 0 {
		t.Fatal(overview, e)
	}
	pr, _ := NewProductReadRepository(pool)
	record, e := pr.GetTask(ctx, u.UserID, []storage.BindingID{adminBinding}, acquisition.ManifestID(id))
	if e != nil || record.Job != nil || record.Manifest.State != acquisition.StateCanceled {
		t.Fatal("owner persistence read", e)
	}
	for _, secret := range []string{"SECRET", "source_ref", "target_path", "provider_task_ref", "payload", "credential"} {
		if strings.Contains(string(first), secret) {
			t.Fatal("receipt leak", secret)
		}
	}
	pendingAssert(t, ctx, pool, id, "CANCELED", 0, 1)
}

func TestPostgresAdminPendingActivationRaces(t *testing.T) {
	for _, cancelFirst := range []bool{false, true} {
		name := "activation-wins"
		if cancelFirst {
			name = "cancellation-wins"
		}
		t.Run(name, func(t *testing.T) {
			ctx, pool, _, a, _, repo := opFixture(t)
			owner := adminUUID(777)
			id := overviewSeed(t, ctx, pool, 1, adminBinding, &owner, "PENDING", "")
			c := pendingInput(t, ctx, pool, id, 1)
			svc, _ := adminops.NewService(repo, nil, []string{adminBinding})
			ar, _ := NewAcquisitionActivationRepository(pool)
			act, _ := acquisition.NewActivationService(ar)
			entered, release := make(chan uint32, 1), make(chan struct{})
			defer func() {
				select {
				case <-release:
				default:
					close(release)
				}
			}()
			if cancelFirst {
				repo.afterPendingLock = pendingBarrier(entered, release)
			} else {
				ar.afterJobInsert = pendingBarrier(entered, release)
			}
			type out struct {
				r adminops.Result
				e error
			}
			cancelDone := make(chan out, 1)
			actDone := make(chan error, 1)
			cancelRun := func() { r, e := svc.Execute(ctx, a, c); cancelDone <- out{r, e} }
			actRun := func() {
				_, e := act.Activate(ctx, acquisition.ActivateRequest{ManifestID: acquisition.ManifestID(id), JobID: jobs.JobID(opID(80)), MaxAttempts: 3})
				actDone <- e
			}
			if cancelFirst {
				go cancelRun()
			} else {
				go actRun()
			}
			pid := <-entered
			if cancelFirst {
				go actRun()
			} else {
				go cancelRun()
			}
			pendingBlocked(t, ctx, pool, pid)
			close(release)
			cr, ae := <-cancelDone, <-actDone
			if cr.e != nil {
				t.Fatal(cr.e)
			}
			if cancelFirst {
				if cr.r.Status != "SUCCEEDED" || !errors.Is(ae, acquisition.ErrInvalidManifestState) {
					t.Fatal(cr, ae)
				}
				pendingAssert(t, ctx, pool, id, "CANCELED", 0, 1)
			} else {
				if ae != nil || cr.r.Code != "PENDING_CHANGED" || cr.r.Manifest == nil || cr.r.Manifest.State != "ACTIVE" || cr.r.Manifest.JobID == nil {
					t.Fatal(cr, ae)
				}
				pendingAssert(t, ctx, pool, id, "ACTIVE", 1, 1)
			}
		})
	}
}

type pendingActivator struct {
	real    *acquisition.ActivationService
	entered chan struct{}
	release chan struct{}
}

func (b pendingActivator) Activate(ctx context.Context, r acquisition.ActivateRequest) (acquisition.ActivateResult, error) {
	close(b.entered)
	select {
	case <-b.release:
		return b.real.Activate(ctx, r)
	case <-ctx.Done():
		return acquisition.ActivateResult{}, ctx.Err()
	}
}
func TestPostgresAdminPendingSubmitGapAndReplayCannotResurrect(t *testing.T) {
	ctx, pool, accounts, a, _, repo := opFixture(t)
	u, e := accounts.Provision(ctx, "submit-owner", "test-password", auth.RoleUser)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = pool.Exec(ctx, `UPDATE storage_bindings SET provider_scope='test-scope' WHERE storage_binding_id=$1`, adminBinding); e != nil {
		t.Fatal(e)
	}
	topology, _ := NewStorageRepository(pool)
	catalog, _ := NewCatalogRepository(pool)
	manifests, _ := NewAcquisitionManifestRepository(pool)
	creator, _ := acquisition.NewService(topology, catalog, manifests)
	ar, _ := NewAcquisitionActivationRepository(pool)
	act, _ := acquisition.NewActivationService(ar)
	jr, _ := NewJobRepository(pool)
	b := pendingActivator{act, make(chan struct{}), make(chan struct{})}
	submit, _ := acquisition.NewSubmissionService(creator, manifests, b, jr)
	owner := acquisition.UserID(u.UserID)
	req := acquisition.SubmitRequest{ManifestID: acquisition.ManifestID(opID(90)), JobID: jobs.JobID(opID(91)), MaxAttempts: 3, Source: "https://fixture.invalid/file", UserID: &owner, TargetStorageBindingID: adminBinding, TargetPath: "/"}
	done := make(chan error, 1)
	go func() { _, e := submit.Submit(ctx, req); done <- e }()
	<-b.entered
	c := pendingInput(t, ctx, pool, string(req.ManifestID), 1)
	svc, _ := adminops.NewService(repo, nil, []string{adminBinding})
	r := opRun(t, svc, a, c)
	close(b.release)
	if r.Status != "SUCCEEDED" || !errors.Is(<-done, acquisition.ErrSubmissionUncertain) {
		t.Fatal("in-flight submit did not fail closed", r)
	}
	replay, _ := acquisition.NewSubmissionService(creator, manifests, act, jr)
	if _, e = replay.Submit(ctx, req); !errors.Is(e, acquisition.ErrSubmissionConflict) {
		t.Fatal("resurrected canceled intent", e)
	}
	pendingAssert(t, ctx, pool, string(req.ManifestID), "CANCELED", 0, 1)
}

func TestPostgresAdminPendingRejectsUnsafeTargets(t *testing.T) {
	for _, kind := range []string{"disabled-owner", "deleted-owner", "missing-owner", "outside", "unowned", "stale-tuple", "wrong-owner", "wrong-binding", "active", "failed", "recovery", "already-canceled", "orphan-key", "orphan-payload", "provider", "provider-result", "admission", "malformed-job"} {
		t.Run(kind, func(t *testing.T) {
			ctx, pool, _, a, _, repo := opFixture(t)
			owner := adminUUID(777)
			id := overviewSeed(t, ctx, pool, 1, adminBinding, &owner, "PENDING", "")
			c := pendingInput(t, ctx, pool, id, 1)
			sql := ""
			args := []any{owner}
			want := "PENDING_CHANGED"
			switch kind {
			case "disabled-owner":
				sql = `UPDATE product_users SET enabled=false WHERE user_id=$1`
				want = "OWNER_UNAVAILABLE"
			case "deleted-owner":
				sql = `UPDATE product_users SET deleted_at=now(),deleted_by=$1,enabled=false WHERE user_id=$1`
				want = "OWNER_UNAVAILABLE"
			case "missing-owner":
				sql = `DELETE FROM product_users WHERE user_id=$1`
				want = "MANIFEST_NOT_FOUND"
			case "outside":
				sql = `UPDATE acquisition_manifests SET target_storage_binding_id=$2 WHERE manifest_id=$1`
				args = []any{id, adminOutside}
				want = "MANIFEST_NOT_FOUND"
			case "unowned":
				sql = `UPDATE acquisition_manifests SET user_id=NULL WHERE manifest_id=$1`
				args = []any{id}
				want = "MANIFEST_NOT_FOUND"
			case "stale-tuple":
				c.Pending.UpdatedAt = c.Pending.UpdatedAt.Add(-time.Microsecond)
			case "wrong-owner":
				c.Pending.OwnerUserID = opID(300)
			case "wrong-binding":
				c.Pending.BindingID = adminOutside
			case "active", "failed", "recovery", "already-canceled":
				state, job := "ACTIVE", "QUEUED"
				if kind == "failed" {
					state, job = "FAILED", "FAILED"
				}
				if kind == "recovery" {
					state, job = "RECOVERY_REQUIRED", "RECOVERY_REQUIRED"
				}
				if kind == "already-canceled" {
					state, job = "CANCELED", "CANCELED"
				}
				ar, _ := NewAcquisitionActivationRepository(pool)
				act, _ := acquisition.NewActivationService(ar)
				if _, e := act.Activate(ctx, acquisition.ActivateRequest{ManifestID: acquisition.ManifestID(id), JobID: jobs.JobID(opID(80)), MaxAttempts: 3}); e != nil {
					t.Fatal(e)
				}
				sql = `WITH j AS(UPDATE jobs SET state=$3 WHERE job_id=$2) UPDATE acquisition_manifests SET state=$4 WHERE manifest_id=$1`
				args = []any{id, opID(80), job, state}
			case "orphan-key", "orphan-payload", "provider", "malformed-job":
				key, payload := "unrelated", `{"schema_version":1,"manifest_id":"`+id+`"}`
				if kind == "orphan-key" {
					key = "acquisition:" + id
					payload = `{}`
				}
				if _, e := pool.Exec(ctx, `INSERT INTO jobs(job_id,job_type,payload,state,idempotency_key,max_attempts,claim_attempts,attempt_count,created_at,updated_at) VALUES($1,'ACQUISITION',$2,'QUEUED',$3,3,0,0,now(),now())`, opID(80), payload, key); e != nil {
					t.Fatal(e)
				}
				want = "PENDING_NOT_ISOLATED"
				if kind == "provider" {
					sql = `INSERT INTO acquisition_provider_tasks(manifest_id,job_id,provider_id,state,created_at,updated_at) VALUES($1,$2,'115','START_RESERVED',now(),now())`
					args = []any{id, opID(80)}
				}
				if kind == "malformed-job" {
					sql = `UPDATE acquisition_manifests SET state='FAILED',job_id=$2 WHERE manifest_id=$1`
					args = []any{id, opID(80)}
					want = "MANIFEST_UNAVAILABLE"
				}
			case "provider-result":
				sql = `UPDATE acquisition_manifests SET result_name='SECRET-provider-result' WHERE manifest_id=$1`
				args = []any{id}
				want = "PENDING_NOT_ISOLATED"
			case "admission":
				if _, e := pool.Exec(ctx, `INSERT INTO product_operation_quotas(user_id,operation_limit,concurrency_limit) VALUES($1,100,15)`, owner); e != nil {
					t.Fatal(e)
				}
				sql = `INSERT INTO product_operations(user_id,operation_key,intent_digest,manifest_id,reserved_job_id,max_attempts,created_at,original_source,execution_source,source_policy) VALUES($1,'fixture',decode(repeat('41',32),'hex'),$2,$3,5,now(),'SECRET','SECRET','source-exact-v1')`
				args = []any{owner, id, opID(80)}
				want = "PENDING_NOT_ISOLATED"
			}
			if sql != "" {
				if _, e := pool.Exec(ctx, sql, args...); e != nil {
					t.Fatal(e)
				}
			}
			var before []byte
			if e := pool.QueryRow(ctx, `SELECT to_jsonb(m) FROM acquisition_manifests m WHERE manifest_id=$1`, id).Scan(&before); e != nil {
				t.Fatal(e)
			}
			svc, _ := adminops.NewService(repo, nil, []string{adminBinding})
			r := opRun(t, svc, a, c)
			if r.Status != "REJECTED" || r.Code != want {
				t.Fatal(r, want)
			}
			if (want == "MANIFEST_NOT_FOUND" || want == "MANIFEST_UNAVAILABLE") && r.Manifest != nil {
				t.Fatal("hidden target projection")
			}
			var after []byte
			if e := pool.QueryRow(ctx, `SELECT to_jsonb(m) FROM acquisition_manifests m WHERE manifest_id=$1`, id).Scan(&after); e != nil || string(before) != string(after) {
				t.Fatal("rejected target changed", e)
			}
		})
	}
}

// Failure before/at commit must never leave either one half of the outcome.
type pendingFailDB struct {
	*pgxpool.Pool
	afterCommit bool
}
type pendingFailTx struct {
	pgx.Tx
	afterCommit bool
}

func (d pendingFailDB) Begin(ctx context.Context) (pgx.Tx, error) {
	tx, e := d.Pool.Begin(ctx)
	return pendingFailTx{tx, d.afterCommit}, e
}
func (tx pendingFailTx) Commit(ctx context.Context) error {
	if tx.afterCommit {
		if e := tx.Tx.Commit(ctx); e != nil {
			return e
		}
	}
	return errors.New("injected ambiguous commit SECRET")
}
func TestPostgresAdminPendingRollbackAndAmbiguousCommitStatusOnly(t *testing.T) {
	for _, mode := range []string{"before-receipt", "receipt-insert", "commit-before", "commit-after"} {
		t.Run(mode, func(t *testing.T) {
			ctx, pool, _, a, _, repo := opFixture(t)
			owner := adminUUID(777)
			id := overviewSeed(t, ctx, pool, 1, adminBinding, &owner, "PENDING", "")
			c := pendingInput(t, ctx, pool, id, 1)
			switch mode {
			case "before-receipt":
				repo.beforeUserCommit = func(context.Context, pgx.Tx) error { return errors.New("injected failure SECRET") }
			case "receipt-insert":
				if _, e := pool.Exec(ctx, `CREATE FUNCTION reject_pending_receipt() RETURNS trigger LANGUAGE plpgsql AS 'BEGIN RAISE EXCEPTION ''injected receipt failure''; END'; CREATE TRIGGER reject_pending_receipt BEFORE INSERT ON product_admin_action_results FOR EACH ROW EXECUTE FUNCTION reject_pending_receipt()`); e != nil {
					t.Fatal(e)
				}
			default:
				repo, _ = NewAdminOperationsRepository(pendingFailDB{pool, mode == "commit-after"})
			}
			// Reserve using the real store, otherwise a synthetic Reserve commit error
			// would test the wrong boundary and never enter Apply.
			reserve, _ := NewAdminOperationsRepository(pool)
			if _, fresh, e := reserve.Reserve(ctx, a, c, []string{adminBinding}); e != nil || !fresh {
				t.Fatal(e)
			}
			if _, e := repo.ApplyPendingCancellation(ctx, a, c, []string{adminBinding}); e == nil {
				t.Fatal("fault not returned")
			}
			status, e := reserve.Status(ctx, a, c.RequestID)
			if e != nil {
				t.Fatal(e)
			}
			state, wantResults := "PENDING", 0
			if mode == "commit-after" {
				state, wantResults = "CANCELED", 1
				if status.Status != "SUCCEEDED" {
					t.Fatal(status)
				}
			} else {
				if status.Status != "UNKNOWN" {
					t.Fatal(status)
				}
			}
			svc, _ := adminops.NewService(reserve, nil, []string{adminBinding})
			r := opRun(t, svc, a, c)
			if r.Status != status.Status {
				t.Fatal("reserved action replayed")
			}
			pendingAssert(t, ctx, pool, id, state, 0, wantResults)
		})
	}
}

func TestPostgresAdminPendingActorSessionRecheckedAndConcurrentCancel(t *testing.T) {
	for _, kind := range []string{"revoked", "expired", "mismatched", "disabled", "demoted", "deleted", "expired-after-lock"} {
		t.Run(kind, func(t *testing.T) {
			ctx, pool, _, a, _, repo := opFixture(t)
			owner := adminUUID(777)
			id := overviewSeed(t, ctx, pool, 1, adminBinding, &owner, "PENDING", "")
			c := pendingInput(t, ctx, pool, id, 1)
			if _, fresh, e := repo.Reserve(ctx, a, c, []string{adminBinding}); e != nil || !fresh {
				t.Fatal(e)
			}
			sql := ""
			args := []any{string(a.Principal.UserID)}
			switch kind {
			case "revoked":
				sql = `UPDATE product_sessions SET revoked_at=now() WHERE user_id=$1`
			case "expired":
				sql = `UPDATE product_sessions SET created_at=now()-interval '2 seconds',expires_at=now()-interval '1 second' WHERE user_id=$1`
			case "mismatched":
				a.Principal.Login = "wrong"
			case "disabled":
				sql = `UPDATE product_users SET enabled=false WHERE user_id=$1`
			case "demoted":
				sql = `UPDATE product_users SET role='USER' WHERE user_id=$1`
			case "deleted":
				sql = `UPDATE product_users SET enabled=false,deleted_at=now(),deleted_by=$1 WHERE user_id=$1`
			case "expired-after-lock":
				repo.afterPendingLock = func(ctx context.Context, tx pgx.Tx) error {
					if _, e := tx.Exec(ctx, `UPDATE product_sessions SET expires_at=clock_timestamp()+interval '100 milliseconds' WHERE user_id=$1`, string(a.Principal.UserID)); e != nil {
						return e
					}
					tick := time.NewTicker(5 * time.Millisecond)
					defer tick.Stop()
					for {
						var expired bool
						if e := tx.QueryRow(ctx, `SELECT expires_at<=clock_timestamp() FROM product_sessions WHERE user_id=$1`, string(a.Principal.UserID)).Scan(&expired); e != nil {
							return e
						}
						if expired {
							return nil
						}
						select {
						case <-tick.C:
						case <-ctx.Done():
							return ctx.Err()
						}
					}
				}
			}
			if sql != "" {
				if _, e := pool.Exec(ctx, sql, args...); e != nil {
					t.Fatal(e)
				}
			}
			if _, e := repo.ApplyPendingCancellation(ctx, a, c, []string{adminBinding}); !errors.Is(e, auth.ErrUnauthorized) && !errors.Is(e, adminops.ErrForbidden) {
				t.Fatal("actor bypass", e)
			}
			pendingAssert(t, ctx, pool, id, "PENDING", 0, 0)
		})
	}
	t.Run("two-administrators", func(t *testing.T) {
		ctx, pool, accounts, a, _, repo := opFixture(t)
		_, e := accounts.Provision(ctx, "second-admin", "test-password", auth.RoleAdmin)
		if e != nil {
			t.Fatal(e)
		}
		login, e := accounts.Login(ctx, "second-admin", "test-password")
		if e != nil {
			t.Fatal(e)
		}
		other := adminops.Actor{Principal: login.Principal, SessionDigest: auth.TokenDigest(login.Token)}
		owner := adminUUID(777)
		id := overviewSeed(t, ctx, pool, 1, adminBinding, &owner, "PENDING", "")
		c := pendingInput(t, ctx, pool, id, 1)
		svc, _ := adminops.NewService(repo, nil, []string{adminBinding})
		entered, release := make(chan uint32, 1), make(chan struct{})
		repo.afterPendingLock = pendingBarrier(entered, release)
		type out struct {
			r adminops.Result
			e error
		}
		done := make(chan out, 2)
		run := func(a adminops.Actor) { r, e := svc.Execute(ctx, a, c); done <- out{r, e} }
		go run(a)
		pid := <-entered
		go run(other)
		pendingBlocked(t, ctx, pool, pid)
		close(release)
		one, two := <-done, <-done
		if one.e != nil || two.e != nil || !((one.r.Status == "SUCCEEDED" && two.r.Code == "PENDING_CHANGED") || (two.r.Status == "SUCCEEDED" && one.r.Code == "PENDING_CHANGED")) {
			t.Fatal(one, two)
		}
		pendingAssert(t, ctx, pool, id, "CANCELED", 0, 2)
	})
}

// A duplicate arriving while the original apply is in flight observes UNKNOWN;
// it cannot enter Apply a second time. Different intent must not reuse the key.
func TestPostgresAdminPendingDuplicateInFlight(t *testing.T) {
	for _, changed := range []bool{false, true} {
		name := "same-intent"
		if changed {
			name = "different-intent"
		}
		t.Run(name, func(t *testing.T) {
			ctx, pool, _, a, _, repo := opFixture(t)
			owner := adminUUID(777)
			id := overviewSeed(t, ctx, pool, 1, adminBinding, &owner, "PENDING", "")
			c := pendingInput(t, ctx, pool, id, 1)
			svc, _ := adminops.NewService(repo, nil, []string{adminBinding})
			entered, release := make(chan uint32, 1), make(chan struct{})
			defer func() {
				select {
				case <-release:
				default:
					close(release)
				}
			}()
			repo.afterPendingLock = pendingBarrier(entered, release)
			type out struct {
				r adminops.Result
				e error
			}
			done := make(chan out, 1)
			go func() { r, e := svc.Execute(ctx, a, c); done <- out{r, e} }()
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("original did not acquire Manifest lock")
			}
			duplicate := c
			precondition := *c.Pending
			duplicate.Pending = &precondition
			if changed {
				duplicate.Pending.UpdatedAt = duplicate.Pending.UpdatedAt.Add(-time.Microsecond)
			}
			r, e := svc.Execute(ctx, a, duplicate)
			if changed {
				if !errors.Is(e, adminops.ErrConflict) {
					t.Fatal("intent reuse accepted", r, e)
				}
			} else if e != nil || r.Status != "UNKNOWN" {
				t.Fatal("in-flight request reapplied", r, e)
			}
			close(release)
			original := <-done
			if original.e != nil || original.r.Status != "SUCCEEDED" {
				t.Fatal(original)
			}
			status, e := svc.Status(ctx, a, c.RequestID)
			if e != nil || status.Code != "PENDING_CANCELED" || !status.RecordedAt.Equal(original.r.RecordedAt) {
				t.Fatal(status, e)
			}
			pendingAssert(t, ctx, pool, id, "CANCELED", 0, 1)
		})
	}
}

// Real HTTP auth -> domain -> PostgreSQL -> persisted receipt, without a fake
// operations port or provider. No other caller's action status is discoverable.
func TestPostgresAdminPendingHTTPActualSessionAndReceipt(t *testing.T) {
	ctx, pool, accounts, a, token, repo := opFixture(t)
	owner := adminUUID(777)
	id := overviewSeed(t, ctx, pool, 1, adminBinding, &owner, "PENDING", "")
	c := pendingInput(t, ctx, pool, id, 1)
	svc, _ := adminops.NewService(repo, nil, []string{adminBinding})
	_, _, _, _, reads, _ := adminFixtureNoReset(t, ctx, pool)
	h, e := producthttp.NewWorkspaceHandler(accounts, "http://127.0.0.1:8180", reads, []producthttp.DisplayBinding{{ID: storage.BindingID(adminBinding), Label: "Synthetic"}}, 16, producthttp.WithAdminOperations(svc))
	if e != nil {
		t.Fatal(e)
	}
	raw, e := json.Marshal(map[string]any{"request_id": c.RequestID, "expected_owner_user_id": c.Pending.OwnerUserID, "expected_binding_id": c.Pending.BindingID, "expected_updated_at": c.Pending.UpdatedAt})
	if e != nil {
		t.Fatal(e)
	}
	path := "/api/v1/admin/downloads/" + id + "/cancel-pending"
	call := func(method, path, token string, payload []byte) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "http://127.0.0.1:8180"+path, strings.NewReader(string(payload)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://127.0.0.1:8180")
		req.Header.Set("Cookie", producthttp.SessionCookieName+"="+token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}
	w := call("POST", path, token, raw)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "PENDING_CANCELED") || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Code, w.Body.String())
	}
	first := w.Body.String()
	for _, method := range []string{"POST", "GET"} {
		target := path
		if method == "GET" {
			target = "/api/v1/admin/actions/" + c.RequestID
		}
		w = call(method, target, token, raw)
		if w.Code != 200 || w.Body.String() != first {
			t.Fatal("receipt not stable", method, w.Code, w.Body.String())
		}
	}
	_, e = accounts.Provision(ctx, "receipt-admin", "test-password", auth.RoleAdmin)
	if e != nil {
		t.Fatal(e)
	}
	other, e := accounts.Login(ctx, "receipt-admin", "test-password")
	if e != nil {
		t.Fatal(e)
	}
	if w = call("GET", "/api/v1/admin/actions/"+c.RequestID, other.Token, nil); w.Code != 404 {
		t.Fatal("other actor receipt exposed", w.Code, w.Body.String())
	}
	if _, e = pool.Exec(ctx, `UPDATE product_sessions SET revoked_at=clock_timestamp() WHERE token_hash=$1`, a.SessionDigest[:]); e != nil {
		t.Fatal(e)
	}
	if w = call("POST", path, token, raw); w.Code != 401 {
		t.Fatal("revoked session", w.Code, w.Body.String())
	}
	pendingAssert(t, ctx, pool, id, "CANCELED", 0, 1)
}

func TestPostgresAdminPendingMigration18To19PreservesAuditAndRollsBack(t *testing.T) {
	ctx := context.Background()
	pool := integrationPool(t, ctx)
	resetTestSchema(t, ctx, pool)
	all, e := migrations.All()
	if e != nil {
		t.Fatal(e)
	}
	for _, m := range all[:18] {
		if _, e = pool.Exec(ctx, m.SQL); e != nil {
			t.Fatal(m.Version, e)
		}
	}
	if _, e = pool.Exec(ctx, `INSERT INTO product_users(user_id,login,password_hash,role,enabled,created_at,updated_at) VALUES($1,'legacy-admin',decode(repeat('41',60),'hex'),'ADMIN',true,now(),now())`, opID(1)); e != nil {
		t.Fatal(e)
	}
	insert := `INSERT INTO product_admin_actions(actor_id,request_id,action,target_id,intent_digest,created_at) VALUES($1,$2,$3,$4,decode(repeat('42',32),'hex'),now())`
	if _, e = pool.Exec(ctx, insert, opID(1), opID(2), adminops.RevokeShare, opID(3)); e != nil {
		t.Fatal(e)
	}
	if _, e = pool.Exec(ctx, `INSERT INTO product_admin_action_results(actor_id,request_id,outcome,recorded_at) VALUES($1,$2,'{"status":"SUCCEEDED","code":"SHARE_REVOKED"}',now())`, opID(1), opID(2)); e != nil {
		t.Fatal(e)
	}
	snapshot := `SELECT json_build_array((SELECT row_to_json(u) FROM product_users u),(SELECT row_to_json(a) FROM product_admin_actions a),(SELECT row_to_json(r) FROM product_admin_action_results r))::text`
	var before, after string
	if e = pool.QueryRow(ctx, snapshot).Scan(&before); e != nil {
		t.Fatal(e)
	}
	if _, e = pool.Exec(ctx, insert, opID(1), opID(4), adminops.CancelPending, opID(3)); e == nil {
		t.Fatal("schema18 accepted new action")
	}
	tx, e := pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, all[18].SQL); e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(ctx, insert, opID(1), opID(4), adminops.CancelPending, opID(3)); e != nil {
		t.Fatal(e)
	}
	if e = tx.Rollback(ctx); e != nil {
		t.Fatal(e)
	}
	if e = pool.QueryRow(ctx, snapshot).Scan(&after); e != nil || before != after {
		t.Fatal("migration rollback changed legacy audit", e)
	}
	if _, e = pool.Exec(ctx, insert, opID(1), opID(4), adminops.CancelPending, opID(3)); e == nil {
		t.Fatal("allowlist survived rollback")
	}
	if _, e = pool.Exec(ctx, all[18].SQL); e != nil {
		t.Fatal(e)
	}
	if e = pool.QueryRow(ctx, snapshot).Scan(&after); e != nil || before != after {
		t.Fatal("upgrade changed audit history", e)
	}
	if _, e = pool.Exec(ctx, insert, opID(1), opID(4), adminops.CancelPending, opID(3)); e != nil {
		t.Fatal("schema19 rejected new action", e)
	}
}
