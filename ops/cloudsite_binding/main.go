// Explicit operator composition; never linked into the product HTTP process.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nathanxiangang-web/panta/internal/platform/bootstrap"
	"github.com/nathanxiangang-web/panta/internal/product"
	"github.com/nathanxiangang-web/panta/internal/storage"
	"github.com/nathanxiangang-web/panta/internal/store/postgres"
	"net/url"
	"os"
	"time"
)

type plan struct {
	RootID       string `json:"root_id"`
	ConnectionID string `json:"connection_id"`
	BindingID    string `json:"binding_id"`
	SourcePath   string `json:"source_path"`
}

func validate(p plan) error {
	ids := map[string]bool{}
	for _, id := range []string{p.RootID, p.ConnectionID, p.BindingID} {
		if !product.ValidID(id) || ids[id] {
			return errors.New("operator identity rejected")
		}
		ids[id] = true
	}
	if p.SourcePath != "/" {
		return errors.New("operator source rejected")
	}
	return nil
}
func run() error {
	if len(os.Args) != 2 || os.Getenv("PANTA_CLOUDSITE_CONFIGURE_GO") != "issue94-complete-cloudsite" {
		return errors.New("operator invocation rejected")
	}
	raw, err := bootstrap.ReadProtectedFile(os.Args[1], 16384)
	if err != nil {
		return errors.New("operator plan unavailable")
	}
	defer clear(raw)
	var p plan
	if json.Unmarshal(raw, &p) != nil || validate(p) != nil {
		return errors.New("operator plan rejected")
	}
	dsn := os.Getenv("PANTA_PRODUCT_DATABASE_URL")
	u, err := url.Parse(dsn)
	if err != nil || u.Hostname() != "127.0.0.1" || u.Port() != "55440" || u.Path != "/panta_mvp_preview" {
		return errors.New("operator database rejected")
	}
	for key := range u.Query() {
		if key != "sslmode" {
			return errors.New("operator database options rejected")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return errors.New("operator database unavailable")
	}
	defer pool.Close()
	var database string
	if pool.QueryRow(ctx, "SELECT current_database()").Scan(&database) != nil || database != "panta_mvp_preview" {
		return errors.New("operator database identity rejected")
	}
	migrator, _ := postgres.NewMigrator(pool)
	status, err := migrator.Status(ctx)
	if err != nil || !status.Compatible || status.CurrentVersion != 14 || len(status.Pending) != 0 {
		return errors.New("operator schema rejected")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return errors.New("operator transaction unavailable")
	}
	defer tx.Rollback(context.Background())
	repo, _ := postgres.NewStorageRepository(tx)
	var at time.Time
	if tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&at) != nil {
		return errors.New("operator clock unavailable")
	}
	c, err := repo.GetConnection(ctx, storage.ConnectionID(p.ConnectionID))
	if errors.Is(err, storage.ErrNotFound) {
		err = repo.CreateConnection(ctx, storage.Connection{ID: storage.ConnectionID(p.ConnectionID), ProviderType: "115-observation-only", Status: storage.ConnectionStatusDisabled, CreatedAt: at, UpdatedAt: at})
	} else if err == nil && (c.Status != storage.ConnectionStatusDisabled || c.CredentialRef != nil || c.ProviderType != "115-observation-only") {
		return errors.New("operator connection conflict")
	}
	if err != nil {
		return errors.New("operator connection refused")
	}
	b, err := repo.GetBinding(ctx, storage.BindingID(p.BindingID))
	if errors.Is(err, storage.ErrNotFound) {
		err = repo.CreateBinding(ctx, storage.Binding{ID: storage.BindingID(p.BindingID), ConnectionID: storage.ConnectionID(p.ConnectionID), OpenListMountPath: "/", IndexCoreRootID: p.RootID, Status: storage.BindingStatusDisabled, CreatedAt: at, UpdatedAt: at})
	} else if err == nil && (b.ConnectionID != storage.ConnectionID(p.ConnectionID) || b.IndexCoreRootID != p.RootID || b.OpenListMountPath != "/" || b.Status != storage.BindingStatusDisabled || b.ProviderScope != nil) {
		return errors.New("operator binding conflict")
	}
	if err != nil {
		return errors.New("operator binding refused")
	}
	if tx.Commit(ctx) != nil {
		return errors.New("operator commit uncertain; inspect before retry")
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"binding_id": p.BindingID, "disabled": true, "provider_scopes": 0, "credential_refs": 0, "schema": 14})
}
func main() {
	if run() != nil {
		os.Stderr.WriteString("CloudSite mapping refused\n")
		os.Exit(1)
	}
}
