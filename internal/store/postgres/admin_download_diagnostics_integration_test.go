package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/nathanxiangang-web/panta/internal/adminread"
	"strings"
	"testing"
)

func TestPostgresAdminDownloadDetailStatesScopeAndRedaction(t *testing.T) {
	ctx, pool, db, repo, s, p := adminFixture(t)
	owner := string(p.UserID)
	for n, pair := range [][2]string{{"PENDING", ""}, {"ACTIVE", "QUEUED"}, {"ACTIVE", "RUNNING"}, {"ACTIVE", "RETRY_WAIT"}, {"AWAITING_VISIBILITY", "RUNNING"}, {"AWAITING_CANONICAL", "RETRY_WAIT"}, {"READY", "SUCCEEDED"}, {"FAILED", "FAILED"}, {"CANCELED", "CANCELED"}, {"RECOVERY_REQUIRED", "RECOVERY_REQUIRED"}} {
		mid := overviewSeed(t, ctx, pool, n, adminBinding, &owner, pair[0], pair[1])
		before := db.queries
		d, e := s.DownloadDetail(ctx, p, mid)
		if e != nil || d.ManifestState != pair[0] || d.ReadAt.IsZero() || db.queries-before != 1 || db.options.AccessMode != pgx.ReadOnly || db.options.IsoLevel != pgx.RepeatableRead {
			t.Fatal(d, e)
		}
		raw, _ := json.Marshal(d)
		for _, secret := range []string{"SECRET", "source_ref", "target_path", "payload", "last_error", "lease_owner", "provider_ref"} {
			if strings.Contains(string(raw), secret) {
				t.Fatal("leak", secret, string(raw))
			}
		}
		if pair[0] == "READY" && (d.Copy == nil || d.Copy.Availability != "REMOVED" || d.Copy.BindingID != adminBinding) {
			t.Fatal("lost historical copy", d)
		}
		filtered, e := s.SearchDownloads(ctx, p, adminread.DownloadFilter{Limit: 20, Status: pair[0], Owner: owner})
		if e != nil || len(filtered.Items) == 0 {
			t.Fatal("state filter", pair, e)
		}
		for _, item := range filtered.Items {
			if item.ManifestState != pair[0] {
				t.Fatal("mixed filter", item)
			}
		}
	}
	for n, group := range []struct {
		binding string
		owner   *string
	}{{adminOutside, &owner}, {adminBinding, nil}} {
		id := overviewSeed(t, ctx, pool, 80+n, group.binding, group.owner, "PENDING", "")
		if _, e := s.DownloadDetail(ctx, p, id); !errors.Is(e, adminread.ErrDownloadNotFound) {
			t.Fatal("scope leak", e)
		}
	}
	if _, e := s.DownloadDetail(ctx, p, adminUUID(9999)); !errors.Is(e, adminread.ErrDownloadNotFound) {
		t.Fatal(e)
	}
	id := adminUUID(1007)
	job := adminUUID(2007)
	for _, sql := range []string{`UPDATE jobs SET payload='{}' WHERE job_id=$1`, `UPDATE jobs SET job_type='OTHER' WHERE job_id=$1`, `UPDATE jobs SET state='CANCELED' WHERE job_id=$1`, `UPDATE jobs SET attempt_count=9 WHERE job_id=$1`} {
		if _, e := pool.Exec(ctx, sql, job); e != nil {
			t.Fatal(e)
		}
		if _, e := s.DownloadDetail(ctx, p, id); !errors.Is(e, adminread.ErrUnavailable) {
			t.Fatal("corrupt link", e)
		}
		if _, e := pool.Exec(ctx, `UPDATE jobs SET payload=jsonb_build_object('schema_version',1,'manifest_id',$2::text),job_type='ACQUISITION',state='FAILED',attempt_count=1 WHERE job_id=$1`, job, id); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := pool.Exec(ctx, `UPDATE copies SET storage_binding_id=$1 WHERE copy_id=$2`, adminOutside, adminUUID(3006)); e != nil {
		t.Fatal(e)
	}
	if _, e := s.DownloadDetail(ctx, p, adminUUID(1006)); !errors.Is(e, adminread.ErrUnavailable) {
		t.Fatal("wrong Copy binding", e)
	}
	repo.afterRead = func(ctx context.Context) error { _, e := db.tx.Exec(ctx, `UPDATE jobs SET attempt_count=0`); return e }
	if _, e := s.DownloadDetail(ctx, p, id); !errors.Is(e, adminread.ErrUnavailable) {
		t.Fatal("read only", e)
	}
	repo.afterRead = func(context.Context) error { return errors.New("SECRET") }
	if _, e := s.DownloadDetail(ctx, p, id); !errors.Is(e, adminread.ErrUnavailable) {
		t.Fatal("query hook", e)
	}
	repo.afterRead = nil
	if _, e := pool.Exec(ctx, `UPDATE acquisition_manifests SET user_id=$1 WHERE manifest_id=$2`, adminUUID(999), id); e != nil {
		t.Fatal(e)
	}
	if _, e := s.DownloadDetail(ctx, p, id); !errors.Is(e, adminread.ErrUnavailable) {
		t.Fatal("dangling owner", e)
	}
}
func TestPostgresAdminDownloadFiltersPaginationAndSnapshot(t *testing.T) {
	ctx, pool, db, repo, s, p := adminFixture(t)
	owner, other := string(p.UserID), adminUUID(2)
	for n := 0; n < 25; n++ {
		u := &owner
		if n%2 == 1 {
			u = &other
		}
		overviewSeed(t, ctx, pool, n, adminBinding, u, "PENDING", "")
	}
	id := overviewSeed(t, ctx, pool, 90, adminBinding, &owner, "ACTIVE", "QUEUED")
	f := adminread.DownloadFilter{Limit: 4, Owner: owner, Status: "PENDING"}
	seen := map[string]bool{}
	for {
		page, e := s.SearchDownloads(ctx, p, f)
		if e != nil {
			t.Fatal(e)
		}
		for _, d := range page.Items {
			if d.OwnerUserID != owner || d.ManifestState != "PENDING" || seen[d.ManifestID] {
				t.Fatal(d)
			}
			seen[d.ManifestID] = true
		}
		if page.NextCursor == "" {
			break
		}
		f.Cursor = page.NextCursor
	}
	if len(seen) != 13 {
		t.Fatal(len(seen))
	}
	empty, e := s.SearchDownloads(ctx, p, adminread.DownloadFilter{Limit: 20, Status: "READY"})
	if e != nil || len(empty.Items) != 0 {
		t.Fatal(empty, e)
	}
	repo.afterRead = func(ctx context.Context) error {
		tx, e := pool.Begin(ctx)
		if e != nil {
			return e
		}
		defer tx.Rollback(context.Background())
		if _, e = tx.Exec(ctx, `UPDATE acquisition_manifests SET state='FAILED',updated_at=clock_timestamp() WHERE manifest_id=$1`, id); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE jobs SET state='FAILED',updated_at=clock_timestamp(),last_error='POLL_HTTP_STATUS' WHERE job_id=$1`, adminUUID(2090)); e != nil {
			return e
		}
		return tx.Commit(ctx)
	}
	before := db.queries
	d, e := s.DownloadDetail(ctx, p, id)
	if e != nil || d.ManifestState != "ACTIVE" || *d.JobState != "QUEUED" || db.queries-before != 1 {
		t.Fatal("torn snapshot", d, e)
	}
	repo.afterRead = nil
	d, e = s.DownloadDetail(ctx, p, id)
	if e != nil || d.ManifestState != "FAILED" || *d.JobState != "FAILED" || d.ErrorCode != "POLL_HTTP_STATUS" {
		t.Fatal(d, e)
	}
	failed, e := s.SearchDownloads(ctx, p, adminread.DownloadFilter{Limit: 20, Status: "FAILED"})
	if e != nil || len(failed.Items) != 1 || failed.Items[0].ManifestID != id {
		t.Fatal(failed, e)
	}
}
