package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/nathanxiangang-web/panta/internal/integrations/indexcore"
	"github.com/nathanxiangang-web/panta/internal/libraryview"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

func libraryFixtureSnapshot(id storage.BindingID, seq int64) libraryview.Snapshot {
	dir := false
	name := "fixture.pdf"
	path := "/fixture.pdf"
	items := []indexcore.ResourceContext{{ResourceID: "library-resource", RootID: "library-root", CanonicalPath: &path, Name: &name, Directory: &dir, Presence: indexcore.ResourcePresent}}
	raw, _ := json.Marshal(items)
	sum := sha256.Sum256(raw)
	checkpoint := ""
	if seq > 0 {
		checkpoint = strings.Repeat("a", 64)
	}
	return libraryview.Snapshot{BindingID: id, RootID: "library-root", Sequence: seq, Checkpoint: checkpoint, UpdatedAt: time.Now().UTC(), Resources: items, Revision: hex.EncodeToString(sum[:]), PreviousSequence: -1}
}
func TestPostgresLibraryMigrationPersistenceCASAndBatch(t *testing.T) {
	ctx := context.Background()
	pool := integrationPool(t, ctx)
	resetTestSchema(t, ctx, pool)
	all, _ := NewMigrator(pool)
	old := &Migrator{pool: pool, migrations: all.migrations[:20]}
	if _, err := old.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	id := storage.BindingID("98000000-0000-4000-8000-000000000001")
	seedStorageBinding(t, ctx, pool, id, "library-root")
	_, err := pool.Exec(ctx, `INSERT INTO copies(copy_id,indexcore_root_id,indexcore_resource_id,storage_binding_id,availability,created_at,updated_at) VALUES('98000000-0000-4000-8000-000000000002','library-root','library-resource',$1,'PRESENT',now(),now())`, string(id))
	if err != nil {
		t.Fatal(err)
	}
	var before, after string
	if err = pool.QueryRow(ctx, `SELECT row_to_json(c)::text FROM copies c`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err = all.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT row_to_json(c)::text FROM copies c`).Scan(&after); err != nil || before != after {
		t.Fatal("migration changed Copy", err)
	}
	cache := NewLibrarySnapshotRepository(pool)
	s := libraryFixtureSnapshot(id, 2)
	if err = cache.SaveLibrarySnapshot(ctx, s); err != nil {
		t.Fatal(err)
	}
	loaded, ok, err := cache.LoadLibrarySnapshot(ctx, id, "library-root")
	if err != nil || !ok || loaded.Sequence != 2 || libraryview.ValidateSnapshot(loaded) != nil {
		t.Fatal(loaded, ok, err)
	}
	s.Sequence = 3
	if err = cache.SaveLibrarySnapshot(ctx, s); err != nil {
		t.Fatal(err)
	}
	stale := libraryFixtureSnapshot(id, 1)
	if cache.SaveLibrarySnapshot(ctx, stale) == nil {
		t.Fatal("older cache overwrote a newer checkpoint")
	}
	stale.PreviousSequence = 3
	stale.PreviousRevision = s.Revision
	if err = cache.SaveLibrarySnapshot(ctx, stale); err != nil {
		t.Fatal("verified baseline recovery CAS", err)
	}
	copies, _ := NewCatalogRepository(pool)
	batch, err := copies.GetCopiesByPhysicalIdentity(ctx, "library-root", []string{"library-resource", "absent"})
	if err != nil || len(batch) != 1 || batch["library-resource"].IndexCoreRootID != "library-root" {
		t.Fatal(batch, err)
	}
	batch, err = copies.GetCopiesByPhysicalIdentity(ctx, "foreign-root", []string{"library-resource"})
	if err != nil || len(batch) != 0 {
		t.Fatal("identity fallback", batch, err)
	}
	if _, err = copies.GetCopiesByPhysicalIdentity(ctx, "library-root", make([]string, 51)); err == nil {
		t.Fatal("unbounded identity query")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `UPDATE storage_bindings SET indexcore_root_id='replacement-root' WHERE storage_binding_id=$1`, string(id)); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cache.SaveLibrarySnapshot(ctx, s) }()
	select {
	case <-done:
		t.Fatal("publication passed an in-progress Root rebind")
	case <-time.After(100 * time.Millisecond):
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err == nil {
		t.Fatal("old Root published after concurrent rebind")
	}
	if _, ok, err = cache.LoadLibrarySnapshot(ctx, id, "replacement-root"); err != nil || ok {
		t.Fatal("Root rebind loaded old snapshot", ok, err)
	}
	if cache.SaveLibrarySnapshot(ctx, s) == nil {
		t.Fatal("old Root published after rebind")
	}
}
