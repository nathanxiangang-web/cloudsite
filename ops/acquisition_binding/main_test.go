package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/nathanxiangang-web/panta/internal/jobs"
	"github.com/nathanxiangang-web/panta/internal/storage"
	"github.com/nathanxiangang-web/panta/internal/store/postgres"
	"os"
	"testing"
)

func isNotFound(e error) bool { return errors.Is(e, storage.ErrNotFound) }
func fixture() plan {
	return plan{RootID: "98000000-0000-4000-8000-000000000101", ConnectionID: "98000000-0000-4000-8000-000000000102", BindingID: "98000000-0000-4000-8000-000000000103", ProviderScope: "opaque-test-scope", CredentialRef: "operator-fixture", MountPath: "/test-downloads", DatabaseURLFile: "/private/test.url", ExpectedDatabase: "panta_operator_activation"}
}
func TestPlanStrictAndOperatorBoundary(t *testing.T) {
	p := fixture()
	raw, _ := json.Marshal(p)
	if _, e := decode(raw); e != nil {
		t.Fatal(e)
	}
	for _, change := range []func(*plan){func(p *plan) { p.RootID = p.ConnectionID }, func(p *plan) { p.BindingID = "bad" }, func(p *plan) { p.ProviderScope = " " }, func(p *plan) { p.CredentialRef = "Cookie=secret" }, func(p *plan) { p.MountPath = "/a/../b" }, func(p *plan) { p.ExpectedDatabase = "other/db" }, func(p *plan) { p.DatabaseURLFile = "" }} {
		bad := p
		change(&bad)
		if validate(bad) == nil {
			t.Fatal("invalid plan accepted")
		}
	}
	for _, bad := range []string{`{}`, `null`, string(raw) + `{}`, `{"root_id":"one","root_id":"two"}`, string(raw[:len(raw)-1]) + `,"secret":"forbidden"}`} {
		if _, e := decode([]byte(bad)); e == nil {
			t.Fatal("ambiguous plan accepted")
		}
	}
	t.Setenv("PANTA_CONFIGURE_ACQUISITION_GO", "")
	if run() == nil {
		t.Fatal("operator guard bypass")
	}
}
func TestPostgresNewAcquisitionMappingAtomicReplayAndQueueGuard(t *testing.T) {
	dsn := os.Getenv("PANTA_TEST_OPERATOR_DATABASE_URL")
	if dsn == "" {
		t.Skip("dedicated owned operator database required")
	}
	ctx := context.Background()
	pool, e := postgres.Open(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	m, _ := postgres.NewMigrator(pool)
	if _, e = m.Apply(ctx); e != nil {
		t.Fatal(e)
	}
	p := fixture()
	if e = configure(ctx, pool, p); e != nil {
		t.Fatal(e)
	}
	repo, _ := postgres.NewStorageRepository(pool)
	c, e := repo.GetConnection(ctx, storage.ConnectionID(p.ConnectionID))
	if e != nil || c.Status != storage.ConnectionStatusActive {
		t.Fatal("missing active connection")
	}
	b, e := repo.GetBinding(ctx, storage.BindingID(p.BindingID))
	if e != nil || b.ProviderScope == nil || *b.ProviderScope != p.ProviderScope || b.IndexCoreRootID != p.RootID {
		t.Fatal("binding identity mismatch")
	}
	if configure(ctx, pool, p) == nil {
		t.Fatal("replay must refuse rather than adopt/change")
	}
	conflict := p
	conflict.RootID = "98000000-0000-4000-8000-000000000104"
	conflict.ConnectionID = "98000000-0000-4000-8000-000000000105"
	if configure(ctx, pool, conflict) == nil {
		t.Fatal("duplicate Binding accepted")
	}
	if _, e = repo.GetConnection(ctx, storage.ConnectionID(conflict.ConnectionID)); e != storage.ErrNotFound && !isNotFound(e) {
		t.Fatal("failed Binding left a Connection")
	}
	oldConn := storage.Connection{ID: "98000000-0000-4000-8000-000000000110", ProviderType: "115-observation-only", Status: storage.ConnectionStatusDisabled, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt}
	oldBinding := storage.Binding{ID: "98000000-0000-4000-8000-000000000111", ConnectionID: oldConn.ID, IndexCoreRootID: "98000000-0000-4000-8000-000000000112", OpenListMountPath: "/", Status: storage.BindingStatusDisabled, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt}
	if e = repo.CreateConnection(ctx, oldConn); e != nil {
		t.Fatal(e)
	}
	if e = repo.CreateBinding(ctx, oldBinding); e != nil {
		t.Fatal(e)
	}
	promote := p
	promote.ConnectionID = "98000000-0000-4000-8000-000000000113"
	promote.BindingID = string(oldBinding.ID)
	promote.RootID = oldBinding.IndexCoreRootID
	promote.MountPath = "/"
	promote.PreviousConnectionID = string(oldConn.ID)
	if e = configure(ctx, pool, promote); e != nil {
		t.Fatal("observation promotion refused", e)
	}
	bound, e := repo.GetBinding(ctx, oldBinding.ID)
	if e != nil || bound.ID != oldBinding.ID || bound.IndexCoreRootID != oldBinding.IndexCoreRootID || bound.OpenListMountPath != "/" || bound.ConnectionID != storage.ConnectionID(promote.ConnectionID) || bound.Status != storage.BindingStatusActive {
		t.Fatal("promotion reinterpreted existing identities")
	}
	previous, e := repo.GetConnection(ctx, oldConn.ID)
	if e != nil || previous.Status != storage.ConnectionStatusDisabled || previous.CredentialRef != nil {
		t.Fatal("old credential changed")
	}
	if e = repo.AttachObservationSession(ctx, oldBinding, storage.ConnectionID(promote.ConnectionID), promote.ProviderScope); !errors.Is(e, storage.ErrConflict) {
		t.Fatal("stale CAS accepted")
	}
	promote.ConnectionID = "98000000-0000-4000-8000-000000000114"
	if configure(ctx, pool, promote) == nil {
		t.Fatal("active observation adopted")
	}
	if _, e = repo.GetConnection(ctx, storage.ConnectionID(promote.ConnectionID)); !isNotFound(e) {
		t.Fatal("failed promotion left a Connection")
	}
	wrong := p
	wrong.ExpectedDatabase = "not_this_database"
	if configure(ctx, pool, wrong) == nil {
		t.Fatal("wrong database accepted")
	}
	j, _ := postgres.NewJobRepository(pool)
	if _, e = j.Create(ctx, jobs.CreateRequest{ID: "98000000-0000-4000-8000-000000000109", Type: "TEST_GUARD", Payload: json.RawMessage(`{}`), MaxAttempts: 1}); e != nil {
		t.Fatal(e)
	}
	fresh := conflict
	fresh.BindingID = "98000000-0000-4000-8000-000000000106"
	if configure(ctx, pool, fresh) == nil {
		t.Fatal("nonempty queue accepted")
	}
	if _, e = repo.GetConnection(ctx, storage.ConnectionID(fresh.ConnectionID)); !isNotFound(e) {
		t.Fatal("queue refusal wrote mapping")
	}
}
