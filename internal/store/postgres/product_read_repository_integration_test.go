package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nathanxiangang-web/panta/internal/acquisition"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/integrations/indexcore"
	"github.com/nathanxiangang-web/panta/internal/jobs"
	"github.com/nathanxiangang-web/panta/internal/product"
	"github.com/nathanxiangang-web/panta/internal/producthttp"
	"github.com/nathanxiangang-web/panta/internal/resourceview"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

type productIndexStub struct{}

func (productIndexStub) Browse(context.Context, indexcore.BrowseRequest) (indexcore.ResourcePage, error) {
	return indexcore.ResourcePage{Items: []indexcore.ResourceContext{}}, nil
}
func (productIndexStub) Resolve(context.Context, indexcore.ResolveRequest) (indexcore.ResolveResult, error) {
	return indexcore.ResolveResult{}, nil
}
func TestPostgresProductReadsOwnershipSnapshotAndHTTP(t *testing.T) {
	ctx := context.Background()
	pool := integrationPool(t, ctx)
	resetTestSchema(t, ctx, pool)
	m, _ := NewMigrator(pool)
	if _, err := m.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	authRepo, _ := NewAuthRepository(pool)
	accounts, _ := auth.NewService(authRepo)
	a, err := accounts.Provision(ctx, "product-a", "test-only-password-123", auth.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	b, err := accounts.Provision(ctx, "product-b", "test-only-password-123", auth.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	binding := storage.BindingID("96000000-0000-4000-8000-000000000001")
	otherBinding := storage.BindingID("96000000-0000-4000-8000-000000000002")
	seedStorageBinding(t, ctx, pool, binding, "product-root")
	seedStorageBinding(t, ctx, pool, otherBinding, "other-root")
	manifests, _ := NewAcquisitionManifestRepository(pool)
	first := acquisition.ManifestID("96000000-0000-4000-8000-000000000010")
	second := acquisition.ManifestID("96000000-0000-4000-8000-000000000011")
	third := acquisition.ManifestID("96000000-0000-4000-8000-000000000012")
	theirs := acquisition.ManifestID("96000000-0000-4000-8000-000000000013")
	legacy := acquisition.ManifestID("96000000-0000-4000-8000-000000000014")
	outside := acquisition.ManifestID("96000000-0000-4000-8000-000000000015")
	now := time.Now().UTC()
	ownerA := acquisition.UserID(a.UserID)
	ownerB := acquisition.UserID(b.UserID)
	for _, item := range []struct {
		id      acquisition.ManifestID
		user    *acquisition.UserID
		binding storage.BindingID
	}{{first, &ownerA, binding}, {second, &ownerA, binding}, {third, &ownerA, binding}, {theirs, &ownerB, binding}, {legacy, nil, binding}, {outside, &ownerA, otherBinding}} {
		err := manifests.CreateManifest(ctx, acquisition.Manifest{ID: item.id, UserID: item.user, TargetStorageBindingID: item.binding, TargetPath: "/", SourceType: "https", SourceRef: "https://SECRET.invalid/provider-source", State: acquisition.StatePending, CreatedAt: now, UpdatedAt: now})
		if err != nil {
			t.Fatal(err)
		}
	}
	reader, err := NewProductReadRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	topology, _ := NewStorageRepository(pool)
	catalogRepo, _ := NewCatalogRepository(pool)
	physical, _ := resourceview.NewPhysicalService(productIndexStub{}, catalogRepo)
	service, err := product.NewReadService(reader, topology, productIndexStub{}, physical, catalogRepo, []storage.BindingID{binding})
	if err != nil {
		t.Fatal(err)
	}
	page, err := service.Tasks(ctx, a, 2, "")
	if err != nil || len(page.Items) != 2 || page.NextCursor != string(second) {
		t.Fatal("owner page", page, err)
	}
	tail, err := service.Tasks(ctx, a, 2, page.NextCursor)
	if err != nil || len(tail.Items) != 1 || tail.Items[0].ManifestID != third || tail.NextCursor != "" {
		t.Fatal("owner continuation", tail, err)
	}
	for _, id := range []acquisition.ManifestID{theirs, legacy, outside} {
		if _, err := service.Task(ctx, a, id); !errors.Is(err, product.ErrNotFound) {
			t.Fatal("unowned task exposed", id, err)
		}
	}
	if _, err := service.Task(ctx, b, first); !errors.Is(err, product.ErrNotFound) {
		t.Fatal("admin implicitly adopted owner")
	}
	if _, err := service.Tasks(ctx, a, 51, ""); !errors.Is(err, product.ErrInvalid) {
		t.Fatal("unbounded list")
	}
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := service.Tasks(cancelCtx, a, 2, ""); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost", err)
	}

	activationRepo, _ := NewAcquisitionActivationRepository(pool)
	activation, _ := acquisition.NewActivationService(activationRepo)
	jobID := jobs.JobID("96000000-0000-4000-8000-000000000020")
	if _, err = activation.Activate(ctx, acquisition.ActivateRequest{ManifestID: first, JobID: jobID, MaxAttempts: 1}); err != nil {
		t.Fatal(err)
	}
	// Commit a paired writer after the snapshot materializes Manifest, before
	// its Job query. A read must still return the old consistent ACTIVE/QUEUED.
	reader.afterManifestRead = func(ctx context.Context) error {
		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback(context.Background()) }()
		if _, err = tx.Exec(ctx, `UPDATE acquisition_manifests SET state='FAILED',updated_at=clock_timestamp() WHERE manifest_id=$1`, string(first)); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE jobs SET state='FAILED',finished_at=clock_timestamp(),updated_at=clock_timestamp() WHERE job_id=$1`, string(jobID)); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	old, err := service.Task(ctx, a, first)
	if err != nil || old.ManifestState != acquisition.StateActive || old.JobState == nil || *old.JobState != jobs.StateQueued {
		t.Fatal("mixed task snapshot", old, err)
	}
	reader.afterManifestRead = nil
	current, err := service.Task(ctx, a, first)
	if err != nil || current.ManifestState != acquisition.StateFailed || current.JobState == nil || *current.JobState != jobs.StateFailed {
		t.Fatal("new paired snapshot", current, err)
	}

	handler, err := producthttp.NewReadHandler(accounts, "http://127.0.0.1:8180", service)
	if err != nil {
		t.Fatal(err)
	}
	login, err := accounts.Login(ctx, "product-a", "test-only-password-123")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path, token string
		want        int
	}{{"/api/v1/tasks", "", 401}, {"/api/v1/tasks", login.Token, 200}, {"/api/v1/tasks/" + string(theirs), login.Token, 404}, {"/api/v1/tasks?user_id=" + string(b.UserID), login.Token, 400}, {"/api/v1/resources?binding_id=" + string(binding), login.Token, 200}, {"/api/v1/resources/resolve?binding_id=" + string(binding) + "&path=/missing", login.Token, 404}} {
		r := httptest.NewRequest("GET", "http://127.0.0.1:8180"+tc.path, nil)
		if tc.token != "" {
			r.AddCookie(&http.Cookie{Name: producthttp.SessionCookieName, Value: tc.token})
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatal(tc.path, w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), "SECRET") || strings.Contains(w.Body.String(), "source_ref") || strings.Contains(w.Body.String(), "password") {
			t.Fatal("unsafe DTO")
		}
	}
	// Actual repository linkage read does not bypass application consistency.
	if _, err = pool.Exec(ctx, `UPDATE jobs SET payload='{}' WHERE job_id=$1`, string(jobID)); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Task(ctx, a, first); !errors.Is(err, product.ErrUnavailable) {
		t.Fatal("corrupt pair exposed", err)
	}
	var before, after int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM acquisition_manifests`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	_, _ = service.Tasks(ctx, a, 2, "")
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM acquisition_manifests`).Scan(&after); err != nil || before != after {
		t.Fatal("read mutated intent")
	}
	bytes, err := json.Marshal(page)
	if err != nil || strings.Contains(string(bytes), "SECRET") {
		t.Fatal("source leak")
	}
	var copies int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM copies`).Scan(&copies); err != nil || copies != 0 {
		t.Fatal("read created Copy")
	}
}
