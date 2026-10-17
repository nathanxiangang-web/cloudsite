// Command acquisition_binding provisions only new logical runtime mappings.
// It never reads a provider cookie, submits a source, starts a Worker or scans.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nathanxiangang-web/panta/internal/platform/bootstrap"
	"github.com/nathanxiangang-web/panta/internal/product"
	"github.com/nathanxiangang-web/panta/internal/storage"
	"github.com/nathanxiangang-web/panta/internal/store/postgres"
	"io"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

var refused = errors.New("new acquisition mapping refused")
var reference = regexp.MustCompile(`^[a-zA-Z0-9._-]{1,128}$`)

type plan struct {
	RootID               string `json:"root_id"`
	ConnectionID         string `json:"connection_id"`
	BindingID            string `json:"binding_id"`
	ProviderScope        string `json:"provider_scope"`
	CredentialRef        string `json:"credential_ref"`
	MountPath            string `json:"mount_path"`
	DatabaseURLFile      string `json:"database_url_file"`
	ExpectedDatabase     string `json:"expected_database"`
	PreviousConnectionID string `json:"previous_connection_id,omitempty"`
}

func validate(p plan) error {
	ids := map[string]bool{}
	for _, id := range []string{p.RootID, p.ConnectionID, p.BindingID} {
		if !product.ValidID(id) || ids[id] {
			return refused
		}
		ids[id] = true
	}
	if p.PreviousConnectionID != "" && (!product.ValidID(p.PreviousConnectionID) || ids[p.PreviousConnectionID]) {
		return refused
	}
	mount, e := storage.NormalizeMountPath(p.MountPath)
	if e != nil || mount != p.MountPath || storage.ValidateProviderScope(&p.ProviderScope) != nil || !reference.MatchString(p.CredentialRef) || !reference.MatchString(p.ExpectedDatabase) || p.DatabaseURLFile == "" {
		return refused
	}
	return nil
}
func decode(raw []byte) (plan, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	start, e := d.Token()
	if e != nil || start != json.Delim('{') {
		return plan{}, refused
	}
	seen := map[string]bool{}
	for d.More() {
		v, e := d.Token()
		key, ok := v.(string)
		if e != nil || !ok || seen[key] {
			return plan{}, refused
		}
		seen[key] = true
		var value json.RawMessage
		if d.Decode(&value) != nil {
			return plan{}, refused
		}
	}
	if _, e = d.Token(); e != nil {
		return plan{}, refused
	}
	if _, e = d.Token(); e != io.EOF {
		return plan{}, refused
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	var p plan
	if d.Decode(&p) != nil || validate(p) != nil {
		return plan{}, refused
	}
	return p, nil
}
func configure(ctx context.Context, pool *pgxpool.Pool, p plan) error {
	if validate(p) != nil {
		return refused
	}
	var db string
	if pool.QueryRow(ctx, "SELECT current_database()").Scan(&db) != nil || db != p.ExpectedDatabase {
		return refused
	}
	m, e := postgres.NewMigrator(pool)
	if e != nil {
		return refused
	}
	s, e := m.Status(ctx)
	if e != nil || !s.Compatible || s.CurrentVersion != postgres.CurrentSchemaVersion {
		return refused
	}
	tx, e := pool.Begin(ctx)
	if e != nil {
		return refused
	}
	defer tx.Rollback(context.Background())
	if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext('cloudsite-new-acquisition-mapping'))"); e != nil {
		return refused
	}
	var count int
	if tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM acquisition_manifests)+(SELECT count(*) FROM jobs)+(SELECT count(*) FROM acquisition_provider_tasks)+(SELECT count(*) FROM product_operations)`).Scan(&count) != nil || count != 0 {
		return refused
	}
	r, _ := postgres.NewStorageRepository(tx)
	if _, e = r.GetConnection(ctx, storage.ConnectionID(p.ConnectionID)); !errors.Is(e, storage.ErrNotFound) {
		return refused
	}
	var previous storage.Binding
	if p.PreviousConnectionID != "" {
		previous, e = r.GetBinding(ctx, storage.BindingID(p.BindingID))
		if e != nil || previous.ConnectionID != storage.ConnectionID(p.PreviousConnectionID) || previous.IndexCoreRootID != p.RootID || previous.OpenListMountPath != p.MountPath || previous.Status != storage.BindingStatusDisabled || previous.ProviderScope != nil {
			return refused
		}
	}
	var at time.Time
	if tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&at) != nil {
		return refused
	}
	if r.CreateConnection(ctx, storage.Connection{ID: storage.ConnectionID(p.ConnectionID), ProviderType: "115", CredentialRef: &p.CredentialRef, Status: storage.ConnectionStatusActive, CreatedAt: at, UpdatedAt: at}) != nil {
		return refused
	}
	if p.PreviousConnectionID == "" {
		if r.CreateBinding(ctx, storage.Binding{ID: storage.BindingID(p.BindingID), ConnectionID: storage.ConnectionID(p.ConnectionID), ProviderScope: &p.ProviderScope, OpenListMountPath: p.MountPath, IndexCoreRootID: p.RootID, Status: storage.BindingStatusActive, CreatedAt: at, UpdatedAt: at}) != nil {
			return refused
		}
	} else if r.AttachObservationSession(ctx, previous, storage.ConnectionID(p.ConnectionID), p.ProviderScope) != nil {
		return refused
	}
	if tx.Commit(ctx) != nil {
		return refused
	}
	return nil
}
func run() error {
	if len(os.Args) != 2 || os.Getenv("PANTA_CONFIGURE_ACQUISITION_GO") != "new-logical-mapping" {
		return refused
	}
	raw, e := bootstrap.ReadProtectedFile(os.Args[1], 16384)
	if e != nil {
		return refused
	}
	defer clear(raw)
	p, e := decode(raw)
	if e != nil {
		return refused
	}
	raw, e = bootstrap.ReadProtectedFile(p.DatabaseURLFile, 4096)
	if e != nil {
		return refused
	}
	defer clear(raw)
	dsn := strings.TrimSpace(string(raw))
	u, e := url.Parse(dsn)
	if e != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || strings.TrimPrefix(u.Path, "/") != p.ExpectedDatabase || u.Hostname() == "" {
		return refused
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, e := postgres.Open(ctx, dsn)
	if e != nil {
		return refused
	}
	defer pool.Close()
	return configure(ctx, pool, p)
}
func main() {
	if run() != nil {
		os.Stderr.WriteString("NEW_ACQUISITION_MAPPING_REFUSED_INSPECT_BEFORE_RETRY\n")
		os.Exit(1)
	}
	os.Stdout.WriteString("NEW_LOGICAL_CONNECTION_AND_BINDING_CREATED_NO_DOWNLOAD\n")
}
