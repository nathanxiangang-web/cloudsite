package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nathanxiangang-web/panta/internal/adminread"
)

// All writes below are synthetic fixtures in the opt-in disposable test DB.
func overviewSeed(t *testing.T, ctx context.Context, pool *pgxpool.Pool, n int, binding string, owner *string, ms, js string) string {
	t.Helper()
	mid, jid := adminUUID(1000+n), adminUUID(2000+n)
	if owner != nil {
		if _, e := pool.Exec(ctx, `INSERT INTO product_users(user_id,login,password_hash,role,enabled,created_at,updated_at) VALUES($1,$2,decode(repeat('41',60),'hex'),'USER',true,now(),now()) ON CONFLICT DO NOTHING`, *owner, "fixture-"+*owner); e != nil {
			t.Fatal(e)
		}
	}
	var job, copy any
	if js != "" {
		job = jid
		_, e := pool.Exec(ctx, `INSERT INTO jobs(job_id,job_type,payload,state,idempotency_key,max_attempts,claim_attempts,attempt_count,next_attempt_at,lease_owner,lease_expires_at,last_error,created_at,updated_at)
 VALUES($1,'ACQUISITION',jsonb_build_object('schema_version',1,'manifest_id',$2::text),$3,'acquisition:'||$2::text,3,2,1,
 CASE WHEN $3='RETRY_WAIT' THEN now() ELSE NULL END,CASE WHEN $3='RUNNING' THEN 'synthetic-worker' ELSE NULL END,
 CASE WHEN $3='RUNNING' THEN now()+interval '1 minute' ELSE NULL END,'Cookie=SECRET',now(),now())`, jid, mid, js)
		if e != nil {
			t.Fatal(e)
		}
	}
	if ms == "READY" {
		copy = adminUUID(3000 + n)
		if _, e := pool.Exec(ctx, `INSERT INTO copies(copy_id,indexcore_root_id,indexcore_resource_id,storage_binding_id,availability,created_at,updated_at) VALUES($1,'fixture-root',$2,$3,'REMOVED',now(),now())`, copy, mid, binding); e != nil {
			t.Fatal(e)
		}
	}
	_, e := pool.Exec(ctx, `INSERT INTO acquisition_manifests(manifest_id,user_id,source_type,source_ref,target_storage_binding_id,target_path,job_id,state,result_copy_id,created_at,updated_at)
 VALUES($1,$2,'https','https://SECRET.invalid/token=SECRET',$3,'/private/SECRET',$4,$5,$6,now(),now())`, mid, owner, binding, job, ms, copy)
	if e != nil {
		t.Fatal(e)
	}
	return mid
}
func TestPostgresAdminOverviewAllStatesScopeAndTrueZero(t *testing.T) {
	ctx, pool, db, _, s, p := adminFixture(t)
	empty, e := s.Overview(ctx, p, "")
	if e != nil || empty.Manifests.Total != 0 || empty.Jobs.Total != 0 || empty.ReadAt.IsZero() {
		t.Fatal(empty, e)
	}
	owner, other := string(p.UserID), adminUUID(2)
	for n, pair := range [][2]string{{"PENDING", ""}, {"ACTIVE", "QUEUED"}, {"ACTIVE", "RUNNING"}, {"ACTIVE", "RETRY_WAIT"}, {"AWAITING_VISIBILITY", "RUNNING"}, {"AWAITING_CANONICAL", "RETRY_WAIT"}, {"READY", "SUCCEEDED"}, {"FAILED", "FAILED"}, {"CANCELED", "CANCELED"}, {"RECOVERY_REQUIRED", "RECOVERY_REQUIRED"}} {
		overviewSeed(t, ctx, pool, n, adminBinding, &owner, pair[0], pair[1])
	}
	// More than a page; the extra owner contributes no Job for these PENDING rows.
	for n := 10; n < 70; n++ {
		overviewSeed(t, ctx, pool, n, adminBinding, &other, "PENDING", "")
	}
	overviewSeed(t, ctx, pool, 80, adminOutside, &owner, "PENDING", "")
	overviewSeed(t, ctx, pool, 81, adminBinding, nil, "PENDING", "")
	// Unrelated / orphan generic jobs are not product-owned acquisition pairs.
	if _, e = pool.Exec(ctx, `INSERT INTO jobs(job_id,job_type,payload,state,max_attempts,claim_attempts,created_at,updated_at) VALUES($1,'OTHER','{}','QUEUED',3,0,now(),now())`, adminUUID(9000)); e != nil {
		t.Fatal(e)
	}
	o, e := s.Overview(ctx, p, "")
	if e != nil || o.Manifests.Total != 70 || o.Manifests.Pending != 61 || o.Manifests.Incomplete != 66 || o.Jobs.Total != 9 || o.Jobs.Queued != 1 || o.Jobs.Running != 2 || o.Jobs.RetryWait != 2 || o.Manifests.RecoveryRequired != 1 {
		t.Fatal(o, e)
	}
	f, e := s.Overview(ctx, p, owner)
	if e != nil || f.Manifests.Total != 10 || f.Manifests.Incomplete != 6 || f.Manifests.Ready != 1 || f.Manifests.Failed != 1 || f.Manifests.Canceled != 1 || f.Jobs.Succeeded != 1 || f.Jobs.Failed != 1 || f.Jobs.Canceled != 1 || f.Jobs.RecoveryRequired != 1 || f.OwnerUserID != owner {
		t.Fatal(f, e)
	}
	zero, e := s.Overview(ctx, p, adminUUID(99))
	if e != nil || zero.Manifests.Total != 0 {
		t.Fatal(zero, e)
	}
	if db.options.AccessMode != pgx.ReadOnly || db.options.IsoLevel != pgx.RepeatableRead {
		t.Fatal("unsafe transaction", db.options)
	}
	raw, _ := json.Marshal(o)
	for _, secret := range []string{"SECRET", "source_ref", "payload", "password", "token", "provider_ref", "target_path", "result_copy_id"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("unsafe DTO", secret)
		}
	}
}
func TestPostgresAdminOverviewConcurrentSnapshot(t *testing.T) {
	ctx, pool, _, repo, s, p := adminFixture(t)
	owner := string(p.UserID)
	id := overviewSeed(t, ctx, pool, 1, adminBinding, &owner, "ACTIVE", "QUEUED")
	readReached, release := make(chan struct{}), make(chan struct{})
	repo.afterRead = func(ctx context.Context) error {
		close(readReached)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	type outcome struct {
		o   adminread.Overview
		err error
	}
	done := make(chan outcome, 1)
	go func() { o, e := s.Overview(ctx, p, ""); done <- outcome{o, e} }()
	select {
	case <-readReached:
	case failed := <-done:
		t.Fatal("reader failed before barrier", failed.err)
	}
	tx, e := pool.Begin(ctx)
	if e != nil {
		close(release)
		t.Fatal(e)
	}
	defer tx.Rollback(context.Background())
	_, e = tx.Exec(ctx, `UPDATE acquisition_manifests SET state='FAILED',updated_at=clock_timestamp() WHERE manifest_id=$1`, id)
	if e == nil {
		_, e = tx.Exec(ctx, `UPDATE jobs SET state='FAILED',updated_at=clock_timestamp() WHERE job_id=(SELECT job_id FROM acquisition_manifests WHERE manifest_id=$1)`, id)
	}
	if e == nil {
		e = tx.Commit(ctx)
	}
	close(release)
	first := <-done
	repo.afterRead = nil
	if e != nil {
		t.Fatal(e)
	}
	if first.err != nil || first.o.Manifests.Active != 1 || first.o.Jobs.Queued != 1 || first.o.Manifests.Failed != 0 {
		t.Fatal("torn snapshot", first)
	}
	next, e := s.Overview(ctx, p, "")
	if e != nil || next.Manifests.Active != 0 || next.Jobs.Queued != 0 || next.Manifests.Failed != 1 || next.Jobs.Failed != 1 || next.ReadAt.Before(first.o.ReadAt) {
		t.Fatal(next, e)
	}
}
func TestPostgresAdminOverviewCorruptionIsUnavailable(t *testing.T) {
	ctx, pool, _, _, s, p := adminFixture(t)
	owner := string(p.UserID)
	id := overviewSeed(t, ctx, pool, 1, adminBinding, &owner, "ACTIVE", "QUEUED")
	for _, sql := range []string{
		`UPDATE jobs SET payload='{}' WHERE job_id=(SELECT job_id FROM acquisition_manifests WHERE manifest_id=$1)`,
		`UPDATE jobs SET payload=payload||'{"extra":true}'::jsonb WHERE job_id=(SELECT job_id FROM acquisition_manifests WHERE manifest_id=$1)`,
		`UPDATE jobs SET payload=jsonb_set(payload,'{schema_version}','1.0'::jsonb) WHERE job_id=(SELECT job_id FROM acquisition_manifests WHERE manifest_id=$1)`,
		`UPDATE jobs SET job_type='OTHER' WHERE job_id=(SELECT job_id FROM acquisition_manifests WHERE manifest_id=$1)`,
		`UPDATE jobs SET idempotency_key='wrong' WHERE job_id=(SELECT job_id FROM acquisition_manifests WHERE manifest_id=$1)`,
		`UPDATE jobs SET attempt_count=4 WHERE job_id=(SELECT job_id FROM acquisition_manifests WHERE manifest_id=$1)`,
		`UPDATE jobs SET state='SUCCEEDED' WHERE job_id=(SELECT job_id FROM acquisition_manifests WHERE manifest_id=$1)`,
	} {
		if _, e := pool.Exec(ctx, sql, id); e != nil {
			t.Fatal(e)
		}
		if _, e := s.Overview(ctx, p, ""); !errors.Is(e, adminread.ErrUnavailable) {
			t.Fatal("broken pair hidden", e)
		}
		if _, e := pool.Exec(ctx, `UPDATE jobs SET payload=jsonb_build_object('schema_version',1,'manifest_id',$1::text),job_type='ACQUISITION',idempotency_key='acquisition:'||$1::text,attempt_count=1,state='QUEUED' WHERE job_id=(SELECT job_id FROM acquisition_manifests WHERE manifest_id=$1)`, id); e != nil {
			t.Fatal(e)
		}
	}
	for _, timestamp := range []string{"0001-01-01 00:00:00+00", "infinity", "-infinity"} {
		if _, e := pool.Exec(ctx, `UPDATE acquisition_manifests SET created_at=$2::timestamptz,updated_at=$2::timestamptz WHERE manifest_id=$1`, id, timestamp); e != nil {
			t.Fatal(e)
		}
		if _, e := s.Overview(ctx, p, ""); !errors.Is(e, adminread.ErrUnavailable) {
			t.Fatal("invalid timestamp accepted", timestamp, e)
		}
	}
	if _, e := pool.Exec(ctx, `UPDATE acquisition_manifests SET created_at=now(),updated_at=now() WHERE manifest_id=$1`, id); e != nil {
		t.Fatal(e)
	}
	// Emulate drifted/legacy data only in this isolated DB: future unknown state.
	if _, e := pool.Exec(ctx, `ALTER TABLE acquisition_manifests DROP CONSTRAINT acquisition_manifests_state_check; UPDATE acquisition_manifests SET state='UNKNOWN'`); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Overview(ctx, p, ""); !errors.Is(e, adminread.ErrUnavailable) {
		t.Fatal("unknown state fabricated zero", e)
	}
	if _, e := pool.Exec(ctx, `UPDATE acquisition_manifests SET state='ACTIVE'; ALTER TABLE acquisition_manifests DROP CONSTRAINT acquisition_manifests_job_id_fkey`); e != nil {
		t.Fatal(e)
	}
	if _, e := pool.Exec(ctx, `UPDATE acquisition_manifests SET job_id=$1`, adminUUID(9999)); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Overview(ctx, p, ""); !errors.Is(e, adminread.ErrUnavailable) {
		t.Fatal("missing job hidden", e)
	}
}
func TestPostgresAdminOverviewReadAndCommitFailure(t *testing.T) {
	ctx, pool, db, repo, s, p := adminFixture(t)
	repo.afterRead = func(ctx context.Context) error {
		_, e := db.tx.Exec(ctx, `UPDATE product_users SET enabled=false`)
		return e
	}
	if _, e := s.Overview(ctx, p, ""); !errors.Is(e, adminread.ErrUnavailable) {
		t.Fatal("read-only write/failure", e)
	}
	repo.afterRead = func(ctx context.Context) error { return db.tx.Rollback(ctx) }
	if _, e := s.Overview(ctx, p, ""); !errors.Is(e, adminread.ErrUnavailable) {
		t.Fatal("commit failure fabricated zero", e)
	}
	repo.afterRead = nil
	if _, e := pool.Exec(ctx, `ALTER TABLE acquisition_manifests RENAME TO unavailable_manifests`); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Overview(ctx, p, ""); !errors.Is(e, adminread.ErrUnavailable) {
		t.Fatal("query failure fabricated zero", e)
	}
}
