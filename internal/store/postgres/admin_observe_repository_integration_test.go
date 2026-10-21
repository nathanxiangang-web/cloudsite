package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/nathanxiangang-web/panta/internal/adminobserve"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/product"
	"github.com/nathanxiangang-web/panta/internal/producthttp"
	"github.com/nathanxiangang-web/panta/internal/resourceview"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

func observeID(n int) string { return fmt.Sprintf("99300000-0000-4000-8000-%012d", n) }
func TestPostgresAdminObservePagingSnapshotAndSecrets(t *testing.T) {
	ctx, pool, db, _, _, p := adminFixture(t)
	repo, _ := NewAdminObserveRepository(db)
	ids := []string{}
	scope := []adminobserve.Display{}
	_, e := pool.Exec(ctx, `INSERT INTO storage_connections(storage_connection_id,provider_type,credential_ref,status,created_at,updated_at) VALUES($1,'115','SECRET-CREDENTIAL','ACTIVE',now(),now())`, observeID(900))
	if e != nil {
		t.Fatal(e)
	}
	for n := 1; n <= 62; n++ {
		id := observeID(n)
		status := "ACTIVE"
		if n%2 == 0 {
			status = "DISABLED"
		}
		_, e = pool.Exec(ctx, `INSERT INTO storage_bindings(storage_binding_id,storage_connection_id,openlist_mount_path,indexcore_root_id,status,created_at,updated_at,provider_scope) VALUES($1,$2,$3,$4,$5,now(),now(),'SECRET-NATIVE-DIR')`, id, observeID(900), fmt.Sprint("/SECRET-MOUNT-", n), fmt.Sprint("private-root-", n), status)
		if e != nil {
			t.Fatal(e)
		}
		if n <= 61 {
			ids = append(ids, id)
			scope = append(scope, adminobserve.Display{BindingID: id, Label: fmt.Sprint("Public ", n)})
		}
	}
	service, _ := adminobserve.NewService(repo, nil, scope)
	cursor := ""
	count := 0
	seen := map[string]bool{}
	for {
		before := db.queries
		page, e := service.Storage(ctx, p, 20, cursor)
		if e != nil {
			t.Fatal(e)
		}
		for _, row := range page.Items {
			if seen[row.BindingID] {
				t.Fatal("duplicate")
			}
			seen[row.BindingID] = true
			count++
			raw, _ := json.Marshal(row)
			for _, secret := range []string{"SECRET", "private-root", "credential_ref", "provider_scope", "mount_path"} {
				if strings.Contains(string(raw), secret) {
					t.Fatal("secret returned", secret)
				}
			}
		}
		if db.queries-before != 1 || db.options.AccessMode != pgx.ReadOnly || db.options.IsoLevel != pgx.RepeatableRead {
			t.Fatal("N+1/nonreadonly")
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	if count != 61 || seen[observeID(62)] {
		t.Fatal(count)
	}
	repo.afterRead = func(ctx context.Context) error {
		_, e := pool.Exec(ctx, `UPDATE storage_connections SET status='DISABLED',updated_at=clock_timestamp() WHERE storage_connection_id=$1`, observeID(900))
		return e
	}
	page, e := service.Storage(ctx, p, 20, "")
	if e != nil || page.Items[0].ConnectionStatus != "ACTIVE" {
		t.Fatal("mixed snapshot", e)
	}
	repo.afterRead = nil
	page, e = service.Storage(ctx, p, 20, "")
	if e != nil || page.Items[0].ConnectionStatus != "DISABLED" {
		t.Fatal(e)
	}
	repo.afterRead = func(ctx context.Context) error {
		_, e := db.tx.Exec(ctx, `UPDATE storage_connections SET status='ACTIVE'`)
		return e
	}
	if _, e = service.Storage(ctx, p, 20, ""); !errors.Is(e, adminobserve.ErrUnavailable) {
		t.Fatal("readonly write allowed", e)
	}
	repo.afterRead = nil
	m, e := repo.Get(ctx, ids, observeID(1))
	if e != nil || m.RootID != "private-root-1" {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(m)
	if strings.Contains(string(raw), "private-root") {
		t.Fatal("root serialization")
	}
	if _, e = repo.Get(ctx, ids, observeID(62)); !errors.Is(e, adminobserve.ErrNotFound) {
		t.Fatal("scope escape", e)
	}
	if _, e = repo.Get(ctx, ids, ""); !errors.Is(e, adminobserve.ErrInvalid) {
		t.Fatal("empty exact binding", e)
	}
	if _, e = pool.Exec(ctx, `UPDATE storage_connections SET provider_type='https://SECRET-provider' WHERE storage_connection_id=$1`, observeID(900)); e != nil {
		t.Fatal(e)
	}
	page, e = service.Storage(ctx, p, 20, "")
	if e != nil || page.Items[0].ProviderType != "OTHER" {
		t.Fatal("raw arbitrary provider category leaked", e)
	}
	for _, bad := range [][]string{nil, {"bad"}, {observeID(1), observeID(1)}} {
		if _, e = repo.List(ctx, bad, 21, ""); !errors.Is(e, adminobserve.ErrInvalid) {
			t.Fatal(e)
		}
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = service.Storage(canceled, p, 20, ""); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	// Fixture-only corruption. A missing configured Binding or parent is not empty success.
	if _, e = pool.Exec(ctx, `ALTER TABLE storage_bindings DROP CONSTRAINT storage_bindings_storage_connection_id_fkey`); e != nil {
		t.Fatal(e)
	}
	if _, e = pool.Exec(ctx, `UPDATE storage_bindings SET storage_connection_id=$2 WHERE storage_binding_id=$1`, observeID(1), observeID(999)); e != nil {
		t.Fatal(e)
	}
	if _, e = service.Storage(ctx, p, 20, ""); !errors.Is(e, adminobserve.ErrUnavailable) {
		t.Fatal("missing parent", e)
	}
	if _, e = pool.Exec(ctx, `DELETE FROM storage_bindings WHERE storage_binding_id=$1`, observeID(1)); e != nil {
		t.Fatal(e)
	}
	if _, e = service.Storage(ctx, p, 20, ""); !errors.Is(e, adminobserve.ErrUnavailable) {
		t.Fatal("missing binding", e)
	}
}
func TestPostgresAdminObserveHTTPActualSessionRoleChanges(t *testing.T) {
	ctx, pool, _, _, _, _ := adminFixture(t)
	repo, _ := NewAdminObserveRepository(pool)
	s, _ := adminobserve.NewService(repo, nil, []adminobserve.Display{{BindingID: adminBinding, Label: "Public"}})
	authRepo, _ := NewAuthRepository(pool)
	accounts, _ := auth.NewService(authRepo)
	p, e := accounts.Provision(ctx, "observe-admin", "test-only-password", auth.RoleAdmin)
	if e != nil {
		t.Fatal(e)
	}
	login, e := accounts.Login(ctx, p.Login, "test-only-password")
	if e != nil {
		t.Fatal(e)
	}
	topology, _ := NewStorageRepository(pool)
	catalog, _ := NewCatalogRepository(pool)
	tasks, _ := NewProductReadRepository(pool)
	physical, _ := resourceview.NewPhysicalService(productIndexStub{}, catalog)
	reads, _ := product.NewReadService(tasks, topology, productIndexStub{}, physical, catalog, []storage.BindingID{storage.BindingID(adminBinding)})
	h, e := producthttp.NewWorkspaceHandler(accounts, "http://127.0.0.1:8180", reads, []producthttp.DisplayBinding{{ID: storage.BindingID(adminBinding), Label: "Public"}}, 15, producthttp.WithAdminObservation(s))
	if e != nil {
		t.Fatal(e)
	}
	get := func(route string) int {
		r := httptest.NewRequest("GET", "http://127.0.0.1:8180/api/v1/admin/"+route, nil)
		r.AddCookie(&http.Cookie{Name: producthttp.SessionCookieName, Value: login.Token})
		r.Header.Set("X-Role", "ADMIN")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if strings.Contains(w.Body.String(), "private-root") {
			t.Fatal("root leaked")
		}
		return w.Code
	}
	if get("storage") != 200 || get("indexcore?binding_id="+adminBinding) != 503 {
		t.Fatal("admin initial access")
	}
	for _, step := range []struct {
		role    string
		enabled bool
		want    int
	}{{"USER", true, 403}, {"ADMIN", false, 401}} {
		if _, e = pool.Exec(ctx, `UPDATE product_users SET role=$2,enabled=$3,updated_at=clock_timestamp() WHERE user_id=$1`, string(p.UserID), step.role, step.enabled); e != nil {
			t.Fatal(e)
		}
		for _, route := range []string{"storage", "indexcore?binding_id=" + adminBinding} {
			if code := get(route); code != step.want {
				t.Fatal(code)
			}
		}
	}
}
