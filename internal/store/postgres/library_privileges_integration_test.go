package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nathanxiangang-web/panta/internal/storage"
)

func TestPostgresLibraryActualRuntimeRolePrivileges(t *testing.T) {
	ctx := context.Background()
	pool := integrationPool(t, ctx)
	resetTestSchema(t, ctx, pool)
	m, _ := NewMigrator(pool)
	if _, err := m.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	id := storage.BindingID("98000000-0000-4000-8000-000000000001")
	seedStorageBinding(t, ctx, pool, id, "library-root")
	var random [24]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	role := "library187_" + hex.EncodeToString(random[:8])
	password := hex.EncodeToString(random[:])
	quoted := pgx.Identifier{role}.Sanitize()
	if _, err := pool.Exec(ctx, "CREATE ROLE "+quoted+" LOGIN PASSWORD '"+password+"' NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS"); err != nil {
		t.Fatal("restricted role creation failed")
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), "DROP OWNED BY "+quoted+"; DROP ROLE "+quoted); err != nil {
			t.Error(err)
		}
	})
	if _, err := pool.Exec(ctx, "GRANT USAGE ON SCHEMA public TO "+quoted+"; GRANT SELECT ON storage_bindings TO "+quoted+"; GRANT SELECT,INSERT,UPDATE ON product_library_snapshots TO "+quoted); err != nil {
		t.Fatal(err)
	}
	cfg := pool.Config().Copy()
	cfg.ConnConfig.User = role
	cfg.ConnConfig.Password = password
	runtime, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal("restricted connection failed")
	}
	defer runtime.Close()
	var current, session string
	if err = runtime.QueryRow(ctx, `SELECT current_user,session_user`).Scan(&current, &session); err != nil || current != role || session != role {
		t.Fatal("not a real restricted login")
	}
	repo := NewLibrarySnapshotRepository(runtime)
	if repo.CheckLibraryPermissions(ctx) == nil {
		t.Fatal("SHARE lock missing privilege was accepted")
	}
	script, err := os.ReadFile("../../../docs/library-187/runtime-grants.sql")
	if err != nil {
		t.Fatal(err)
	}
	grant := strings.ReplaceAll(string(script), `:"runtime_role"`, quoted)
	if _, err = pool.Exec(ctx, grant); err != nil {
		t.Fatal(err)
	}
	if err = repo.CheckLibraryPermissions(ctx); err != nil {
		t.Fatal(err)
	}
	s := libraryFixtureSnapshot(id, 0)
	if err = repo.SaveLibrarySnapshot(ctx, s); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := repo.LoadLibrarySnapshot(ctx, id, "library-root"); err != nil || !ok {
		t.Fatal(ok, err)
	}
	for _, sql := range []string{`DELETE FROM product_library_snapshots`, `UPDATE storage_bindings SET indexcore_root_id='forbidden'`} {
		_, err = runtime.Exec(ctx, sql)
		p, ok := err.(*pgconn.PgError)
		if !ok || p.Code != "42501" {
			t.Fatal("restricted runtime exceeded grants", err)
		}
	}
}
