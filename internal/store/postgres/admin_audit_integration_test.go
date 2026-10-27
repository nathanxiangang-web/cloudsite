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
	"github.com/nathanxiangang-web/panta/internal/adminaudit"
	"github.com/nathanxiangang-web/panta/internal/adminops"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/producthttp"
	"github.com/nathanxiangang-web/panta/internal/storage"
	"github.com/nathanxiangang-web/panta/migrations"
)

func auditServices(t *testing.T, pool *pgxpool.Pool) (*AdminAuditRepository, *adminaudit.Service) {
	t.Helper()
	r, e := NewAdminAuditRepository(pool)
	if e != nil {
		t.Fatal(e)
	}
	s, e := adminaudit.New(r, []string{adminBinding})
	if e != nil {
		t.Fatal(e)
	}
	return r, s
}
func seedAudit(t *testing.T, ctx context.Context, pool *pgxpool.Pool, a adminops.Actor, n int, action, status, code string, target *string) {
	t.Helper()
	id := opID(600 + n)
	created := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	if _, e := pool.Exec(ctx, `INSERT INTO product_admin_actions(actor_id,request_id,action,target_id,intent_digest,created_at)VALUES($1,$2,$3,$4,decode(repeat('41',32),'hex'),$5)`, string(a.Principal.UserID), id, action, target, created); e != nil {
		t.Fatal(e)
	}
	if status != "UNKNOWN" {
		auditTerminal(t, ctx, pool, a, id, action, status, code, target)
	}
}
func auditTerminal(t *testing.T, ctx context.Context, pool *pgxpool.Pool, a adminops.Actor, id, action, status, code string, target *string) {
	t.Helper()
	at := time.Date(2026, 10, 27, 0, 1, 0, 123456000, time.UTC)
	r := adminops.Result{RequestID: id, Action: action, Status: status, Code: code, RecordedAt: at}
	if target != nil {
		r.TargetID = *target
	}
	if action == adminops.CreateUser && status == "SUCCEEDED" {
		r.TargetID = opID(777)
		r.User = &adminops.User{UserID: r.TargetID, Login: "SECRET-private-login", Enabled: true, Version: 0}
	}
	if action == adminops.ProbeAccount && status == "SUCCEEDED" {
		r.Capacity = &adminops.Capacity{Status: "UNKNOWN", Source: "115_GET_INFO", ProbedAt: at}
	}
	if action == adminops.CancelPending && status == "SUCCEEDED" {
		r.Manifest = &adminops.ManifestOutcome{State: "CANCELED", UpdatedAt: at}
	}
	raw, e := json.Marshal(r)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = pool.Exec(ctx, `INSERT INTO product_admin_action_results(actor_id,request_id,outcome,recorded_at)VALUES($1,$2,$3,$4)`, string(a.Principal.UserID), id, raw, at); e != nil {
		t.Fatal(e)
	}
}
func TestPostgresAdminAuditAllActionsUnknownAndSafeProjection(t *testing.T) {
	ctx, pool, _, a, _, _ := opFixture(t)
	r, s := auditServices(t, pool)
	owner := adminUUID(777)
	manifest := overviewSeed(t, ctx, pool, 1, adminBinding, &owner, "PENDING", "")
	share := opID(888)
	if _, e := pool.Exec(ctx, `INSERT INTO product_file_shares(share_id,owner_id,binding_id,resource_id,canonical_path,file_name,size_bytes,token_hash,created_at,expires_at)VALUES($1,$2,$3,'SECRET-resource','/SECRET-path','SECRET-name',1,decode(repeat('42',32),'hex'),now(),now()+interval '1 day')`, share, owner, adminBinding); e != nil {
		t.Fatal(e)
	}
	for i, tc := range []struct {
		action, code string
		target       *string
	}{
		{adminops.CreateUser, "USER_CREATED", nil}, {adminops.SetEnabled, "USER_STATE_CHANGED", &owner}, {adminops.RenameUser, "USER_RENAMED", &owner}, {adminops.ResetPassword, "USER_PASSWORD_RESET", &owner}, {adminops.DeleteUser, "USER_DELETED", &owner}, {adminops.RestoreUser, "USER_RESTORED", &owner}, {adminops.ProbeAccount, "PROBE_RECORDED", ptrString(adminBinding)}, {adminops.RevokeShare, "SHARE_REVOKED", &share}, {adminops.CancelPending, "PENDING_CANCELED", &manifest},
	} {
		seedAudit(t, ctx, pool, a, i, tc.action, "SUCCEEDED", tc.code, tc.target)
	}
	seedAudit(t, ctx, pool, a, 9, adminops.CreateUser, "UNKNOWN", "", nil)
	seedAudit(t, ctx, pool, a, 10, adminops.CreateUser, "REJECTED", "CONFLICT", nil)
	r.afterRead = func(ctx context.Context, tx pgx.Tx) error {
		var ro, iso string
		if e := tx.QueryRow(ctx, `SHOW transaction_read_only`).Scan(&ro); e != nil {
			return e
		}
		if e := tx.QueryRow(ctx, `SHOW transaction_isolation`).Scan(&iso); e != nil {
			return e
		}
		if ro != "on" || iso != "repeatable read" {
			t.Fatal(ro, iso)
		}
		return nil
	}
	page, e := s.Search(ctx, a, adminaudit.Filter{Limit: 50})
	if e != nil || len(page.Items) != 11 {
		t.Fatal(len(page.Items), e)
	}
	unknown := 0
	for _, item := range page.Items {
		if item.Status == "UNKNOWN" {
			unknown++
			if item.RecordedAt != nil || item.Code != "RESERVED_NO_CONFIRMED_RESULT" {
				t.Fatal(item)
			}
		}
		detail, e := s.Detail(ctx, a, item.ActorID, item.RequestID)
		if e != nil || detail.Code != item.Code {
			t.Fatal(detail, e)
		}
	}
	if unknown != 1 {
		t.Fatal("reservation not visible")
	}
	raw, _ := json.Marshal(page)
	for _, forbidden := range []string{"SECRET", "intent_digest", "target_id", "login", "password", "capacity", "source_ref", "credential", "payload", "canonical_path"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatal("unsafe audit projection", forbidden)
		}
	}
}
func ptrString(s string) *string { return &s }

func TestPostgresAdminAuditActorScopeBindingAndCursorFilters(t *testing.T) {
	ctx, pool, accounts, a, _, _ := opFixture(t)
	_, s := auditServices(t, pool)
	_, e := accounts.Provision(ctx, "other-audit-admin", "test-password", auth.RoleAdmin)
	if e != nil {
		t.Fatal(e)
	}
	login, e := accounts.Login(ctx, "other-audit-admin", "test-password")
	if e != nil {
		t.Fatal(e)
	}
	other := adminops.Actor{Principal: login.Principal, SessionDigest: auth.TokenDigest(login.Token)}
	for i := 0; i < 5; i++ {
		seedAudit(t, ctx, pool, a, i, adminops.CreateUser, "UNKNOWN", "", nil)
	}
	seedAudit(t, ctx, pool, other, 8, adminops.CreateUser, "UNKNOWN", "", nil)
	seedAudit(t, ctx, pool, a, 9, adminops.ProbeAccount, "UNKNOWN", "", ptrString(adminOutside))
	var ids []string
	filter := adminaudit.Filter{Limit: 2, Status: "UNKNOWN"}
	for {
		page, e := s.Search(ctx, a, filter)
		if e != nil {
			t.Fatal(e)
		}
		for _, v := range page.Items {
			ids = append(ids, v.RequestID)
		}
		if page.NextCursor == "" {
			break
		}
		filter.Cursor = page.NextCursor
	}
	if len(ids) != 5 {
		t.Fatal(ids)
	}
	for i := 1; i < len(ids); i++ {
		if ids[i] >= ids[i-1] {
			t.Fatal("nondeterministic tie ordering", ids)
		}
	}
	p, e := s.Search(ctx, a, adminaudit.Filter{Limit: 1})
	if e != nil {
		t.Fatal(e)
	}
	for _, f := range []adminaudit.Filter{{Limit: 1, Cursor: p.NextCursor, Action: adminops.RenameUser}, {Limit: 1, Cursor: p.NextCursor, Status: "SUCCEEDED"}, {Limit: 1, Cursor: p.NextCursor, From: time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)}} {
		if _, e = s.Search(ctx, a, f); !errors.Is(e, adminaudit.ErrInvalid) {
			t.Fatal("cursor context accepted", e)
		}
	}
	if _, e = s.Search(ctx, other, adminaudit.Filter{Limit: 1, Cursor: p.NextCursor}); !errors.Is(e, adminaudit.ErrInvalid) {
		t.Fatal("actor cursor accepted", e)
	}
	repo, _ := NewAdminAuditRepository(pool)
	changed, _ := adminaudit.New(repo, []string{adminOutside})
	if _, e = changed.Search(ctx, a, adminaudit.Filter{Limit: 1, Cursor: p.NextCursor}); !errors.Is(e, adminaudit.ErrInvalid) {
		t.Fatal("Binding cursor accepted", e)
	}
	if _, e = s.Search(ctx, a, adminaudit.Filter{Limit: 10, ActorID: string(other.Principal.UserID)}); !errors.Is(e, adminaudit.ErrForbidden) {
		t.Fatal(e)
	}
	if _, e = s.Detail(ctx, a, string(other.Principal.UserID), opID(608)); !errors.Is(e, adminaudit.ErrNotFound) {
		t.Fatal("cross-admin detail exposed", e)
	}
	if _, e = s.Detail(ctx, a, string(a.Principal.UserID), opID(609)); !errors.Is(e, adminaudit.ErrNotFound) {
		t.Fatal("outside binding exposed", e)
	}
}

func TestPostgresAdminAuditMalformedOutcomesFailClosed(t *testing.T) {
	for name, sql := range map[string]string{
		"extra-secret": `outcome||'{"password":"SECRET"}'`, "bad-code": `jsonb_set(outcome,'{code}','"Cookie=SECRET"')`, "wrong-request": `jsonb_set(outcome,'{request_id}','"99500000-0000-4000-8000-000000000999"')`, "persisted-unknown": `jsonb_set(outcome,'{status}','"UNKNOWN"')`, "wrong-time": `jsonb_set(outcome,'{recorded_at}','"never SECRET"')`, "secret-user-key": `jsonb_set(outcome,'{user,password}','"SECRET"')`, "malformed-user": `jsonb_set(outcome,'{user}','"SECRET"')`,
	} {
		t.Run(name, func(t *testing.T) {
			ctx, pool, _, a, _, _ := opFixture(t)
			_, s := auditServices(t, pool)
			seedAudit(t, ctx, pool, a, 0, adminops.CreateUser, "SUCCEEDED", "USER_CREATED", nil)
			if _, e := pool.Exec(ctx, `UPDATE product_admin_action_results SET outcome=`+sql); e != nil {
				t.Fatal(e)
			}
			if _, e := s.Search(ctx, a, adminaudit.Filter{Limit: 10}); !errors.Is(e, adminaudit.ErrUnavailable) {
				t.Fatal("invalid outcome accepted", e)
			}
		})
	}
}

func TestPostgresAdminAuditSnapshotAndFreshRevocation(t *testing.T) {
	ctx, pool, _, a, _, _ := opFixture(t)
	r, s := auditServices(t, pool)
	seedAudit(t, ctx, pool, a, 0, adminops.CreateUser, "UNKNOWN", "", nil)
	r.afterRead = func(context.Context, pgx.Tx) error {
		auditTerminal(t, ctx, pool, a, opID(600), adminops.CreateUser, "SUCCEEDED", "USER_CREATED", nil)
		return nil
	}
	p, e := s.Search(ctx, a, adminaudit.Filter{Limit: 10})
	if e != nil || len(p.Items) != 1 || p.Items[0].Status != "UNKNOWN" {
		t.Fatal("mixed snapshot", p, e)
	}
	r.afterRead = nil
	p, e = s.Search(ctx, a, adminaudit.Filter{Limit: 10})
	if e != nil || p.Items[0].Status != "SUCCEEDED" {
		t.Fatal(p, e)
	}
	r.afterRead = func(ctx context.Context, _ pgx.Tx) error {
		_, e := pool.Exec(ctx, `UPDATE product_sessions SET revoked_at=clock_timestamp() WHERE token_hash=$1`, a.SessionDigest[:])
		return e
	}
	if _, e = s.Search(ctx, a, adminaudit.Filter{Limit: 10}); !errors.Is(e, auth.ErrUnauthorized) {
		t.Fatal("in-flight revocation ignored", e)
	}
}

func TestPostgresAdminAuditLegacy16AndMissingTables(t *testing.T) {
	ctx := context.Background()
	pool := integrationPool(t, ctx)
	resetTestSchema(t, ctx, pool)
	history, e := migrations.All()
	if e != nil {
		t.Fatal(e)
	}
	for _, m := range history[:16] {
		if _, e = pool.Exec(ctx, m.SQL); e != nil {
			t.Fatal(e)
		}
	}
	id := opID(777)
	var digest [32]byte
	for i := range digest {
		digest[i] = 0x42
	}
	if _, e = pool.Exec(ctx, `INSERT INTO product_users(user_id,login,password_hash,role,enabled,created_at,updated_at)VALUES($1,'legacy-admin',decode(repeat('41',60),'hex'),'ADMIN',true,now(),now())`, id); e != nil {
		t.Fatal(e)
	}
	if _, e = pool.Exec(ctx, `INSERT INTO product_sessions(token_hash,user_id,created_at,expires_at)VALUES($1,$2,now(),now()+interval '1 day')`, digest[:], id); e != nil {
		t.Fatal(e)
	}
	a := adminops.Actor{Principal: auth.Principal{UserID: auth.UserID(id), Login: "legacy-admin", Role: auth.RoleAdmin}, SessionDigest: digest}
	_, s := auditServices(t, pool)
	seedAudit(t, ctx, pool, a, 0, adminops.CreateUser, "UNKNOWN", "", nil)
	p, e := s.Search(ctx, a, adminaudit.Filter{Limit: 10})
	if e != nil || len(p.Items) != 1 {
		t.Fatal("legacy receipt unavailable", p, e)
	}
	auditTerminal(t, ctx, pool, a, opID(600), adminops.CreateUser, "SUCCEEDED", "USER_CREATED", nil)
	if _, e = pool.Exec(ctx, `UPDATE product_admin_action_results SET outcome=outcome#-'{user,deleted}'`); e != nil {
		t.Fatal(e)
	}
	if p, e = s.Search(ctx, a, adminaudit.Filter{Limit: 10}); e != nil || p.Items[0].Status != "SUCCEEDED" {
		t.Fatal("legacy16 terminal receipt", p, e)
	}
	if _, e = pool.Exec(ctx, `DROP TABLE product_admin_action_results`); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Search(ctx, a, adminaudit.Filter{Limit: 10}); !errors.Is(e, adminaudit.ErrUnavailable) {
		t.Fatal("missing table claimed empty", e)
	}
}

func TestPostgresAdminAuditRevokedActorsAndDeletedTargets(t *testing.T) {
	for _, kind := range []string{"expired", "disabled", "demoted", "deleted", "wrong-login", "wrong-session"} {
		t.Run(kind, func(t *testing.T) {
			ctx, pool, _, a, _, _ := opFixture(t)
			_, s := auditServices(t, pool)
			seedAudit(t, ctx, pool, a, 0, adminops.CreateUser, "UNKNOWN", "", nil)
			var e error
			switch kind {
			case "expired":
				_, e = pool.Exec(ctx, `UPDATE product_sessions SET created_at=now()-interval '2 seconds',expires_at=now()-interval '1 second' WHERE user_id=$1`, string(a.Principal.UserID))
			case "disabled":
				_, e = pool.Exec(ctx, `UPDATE product_users SET enabled=false WHERE user_id=$1`, string(a.Principal.UserID))
			case "demoted":
				_, e = pool.Exec(ctx, `UPDATE product_users SET role='USER' WHERE user_id=$1`, string(a.Principal.UserID))
			case "deleted":
				_, e = pool.Exec(ctx, `UPDATE product_users SET enabled=false,deleted_at=now(),deleted_by=$1 WHERE user_id=$1`, string(a.Principal.UserID))
			case "wrong-login":
				a.Principal.Login = "mismatch"
			case "wrong-session":
				a.SessionDigest = [32]byte{3}
			}
			if e != nil {
				t.Fatal(e)
			}
			if _, e = s.Search(ctx, a, adminaudit.Filter{Limit: 10}); !errors.Is(e, auth.ErrUnauthorized) {
				t.Fatal("inactive actor read audit", e)
			}
		})
	}
	ctx, pool, accounts, a, _, _ := opFixture(t)
	_, s := auditServices(t, pool)
	u, e := accounts.Provision(ctx, "deleted-audit-target", "test-password", auth.RoleUser)
	if e != nil {
		t.Fatal(e)
	}
	id := string(u.UserID)
	seedAudit(t, ctx, pool, a, 0, adminops.SetEnabled, "SUCCEEDED", "USER_STATE_CHANGED", &id)
	if _, e = pool.Exec(ctx, `UPDATE product_users SET enabled=false,deleted_at=now(),deleted_by=$2 WHERE user_id=$1`, id, string(a.Principal.UserID)); e != nil {
		t.Fatal(e)
	}
	p, e := s.Search(ctx, a, adminaudit.Filter{Limit: 1})
	if e != nil || p.Items[0].TargetStatus != "DELETED_OR_MISSING" {
		t.Fatal(p, e)
	}
}
func TestPostgresAdminAuditSingleConnectionPoolDoesNotSelfBlock(t *testing.T) {
	ctx, pool, _, a, _, _ := opFixture(t)
	seedAudit(t, ctx, pool, a, 0, adminops.CreateUser, "UNKNOWN", "", nil)
	cfg := pool.Config()
	cfg.MaxConns = 1
	single, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer single.Close()
	_, s := auditServices(t, single)
	p, e := s.Search(ctx, a, adminaudit.Filter{Limit: 10})
	if e != nil || len(p.Items) != 1 {
		t.Fatal(p, e)
	}
}
func TestPostgresAdminAuditHTTPActualSessionNoSideEffects(t *testing.T) {
	ctx, pool, accounts, a, token, _ := opFixture(t)
	_, s := auditServices(t, pool)
	seedAudit(t, ctx, pool, a, 0, adminops.CreateUser, "UNKNOWN", "", nil)
	_, _, _, _, reads, _ := adminFixtureNoReset(t, ctx, pool)
	h, e := producthttp.NewWorkspaceHandler(accounts, "http://127.0.0.1:8180", reads, []producthttp.DisplayBinding{{ID: storage.BindingID(adminBinding), Label: "Synthetic"}}, 19, producthttp.WithAdminAuditReads(s))
	if e != nil {
		t.Fatal(e)
	}
	call := func(path, cookie string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "http://127.0.0.1:8180"+path, nil)
		req.Header.Set("Cookie", producthttp.SessionCookieName+"="+cookie)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}
	path := "/api/v1/admin/audit/actions"
	w := call(path, token)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "UNKNOWN") || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Code, w.Body.String())
	}
	auditTerminal(t, ctx, pool, a, opID(600), adminops.CreateUser, "SUCCEEDED", "USER_CREATED", nil)
	w = call(path+"/"+string(a.Principal.UserID)+"/"+opID(600), token)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "USER_CREATED") || strings.Contains(w.Body.String(), "SECRET") || strings.Contains(w.Body.String(), "login") {
		t.Fatal(w.Code, w.Body.String())
	}
	var actions, results, providers int
	if e = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM product_admin_actions),(SELECT count(*) FROM product_admin_action_results),(SELECT count(*) FROM acquisition_provider_tasks)`).Scan(&actions, &results, &providers); e != nil || actions != 1 || results != 1 || providers != 0 {
		t.Fatal("audit read side effects", actions, results, providers, e)
	}
	_, e = accounts.Provision(ctx, "ordinary-audit-viewer", "test-password", auth.RoleUser)
	if e != nil {
		t.Fatal(e)
	}
	user, e := accounts.Login(ctx, "ordinary-audit-viewer", "test-password")
	if e != nil {
		t.Fatal(e)
	}
	if w = call(path, user.Token); w.Code != 403 {
		t.Fatal("ordinary user read audit", w.Code)
	}
	if _, e = pool.Exec(ctx, `UPDATE product_sessions SET revoked_at=clock_timestamp() WHERE token_hash=$1`, a.SessionDigest[:]); e != nil {
		t.Fatal(e)
	}
	if w = call(path, token); w.Code != 401 {
		t.Fatal("revoked session read", w.Code)
	}
}
