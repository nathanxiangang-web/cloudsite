package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/nathanxiangang-web/panta/internal/adminops"
	"github.com/nathanxiangang-web/panta/internal/adminread"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/fileaccess"
	"github.com/nathanxiangang-web/panta/internal/fileshare"
	"github.com/nathanxiangang-web/panta/internal/producthttp"
	"github.com/nathanxiangang-web/panta/migrations"
)

func TestPostgresAdminLifecycleRenamePasswordDeleteRestore(t *testing.T) {
	ctx, pool, accounts, a, _, repo := opFixture(t)
	svc, _ := adminops.NewService(repo, nil, []string{adminBinding})
	created := opRun(t, svc, a, adminops.Command{RequestID: opID(1), Action: adminops.CreateUser, Login: "lifecycle-user", Password: "original-password"})
	id := created.TargetID
	logged, e := accounts.Login(ctx, "lifecycle-user", "original-password")
	if e != nil {
		t.Fatal(e)
	}
	shares, _ := NewShareRepository(pool)
	record := fileshare.Record{ID: opID(950), Owner: auth.UserID(id), Target: fileaccess.Request{BindingID: adminBinding, ResourceID: "synthetic", Path: "/test.txt"}, Name: "test.txt", Size: 4, TokenHash: fileshare.SecretHash("synthetic-share-token")}
	share, e := shares.Create(ctx, record, time.Hour)
	if e != nil {
		t.Fatal(e)
	}
	grant := fileshare.SecretHash("synthetic-grant-token")
	if e = shares.IssueGrant(ctx, share.ID, grant); e != nil {
		t.Fatal(e)
	}
	rename := adminops.Command{RequestID: opID(2), Action: adminops.RenameUser, TargetID: id, Login: "new-login"}
	renamed := opRun(t, svc, a, rename)
	if renamed.Code != "USER_RENAMED" || renamed.User.Version != 1 || renamed.TargetID != id {
		t.Fatal(renamed)
	}
	if _, e = accounts.Authenticate(ctx, logged.Token); !errors.Is(e, auth.ErrUnauthorized) {
		t.Fatal("rename kept old session", e)
	}
	if _, e = accounts.Login(ctx, "lifecycle-user", "original-password"); !errors.Is(e, auth.ErrUnauthorized) {
		t.Fatal("old login still valid", e)
	}
	logged, e = accounts.Login(ctx, "new-login", "original-password")
	if e != nil {
		t.Fatal(e)
	}
	reset := adminops.Command{RequestID: opID(3), Action: adminops.ResetPassword, TargetID: id, ExpectedVersion: 1, Password: "replacement-password"}
	resetResult := opRun(t, svc, a, reset)
	if resetResult.Code != "USER_PASSWORD_RESET" || resetResult.User.Version != 2 {
		t.Fatal(resetResult)
	}
	if _, e = accounts.Authenticate(ctx, logged.Token); !errors.Is(e, auth.ErrUnauthorized) {
		t.Fatal("password reset kept session", e)
	}
	if _, e = accounts.Login(ctx, "new-login", "original-password"); !errors.Is(e, auth.ErrUnauthorized) {
		t.Fatal("old password works", e)
	}
	logged, e = accounts.Login(ctx, "new-login", "replacement-password")
	if e != nil {
		t.Fatal(e)
	}
	del := adminops.Command{RequestID: opID(4), Action: adminops.DeleteUser, TargetID: id, ExpectedVersion: 2}
	removed := opRun(t, svc, a, del)
	if removed.Code != "USER_DELETED" || !removed.User.Deleted || removed.User.Enabled || removed.User.Version != 3 {
		t.Fatal(removed)
	}
	replay := opRun(t, svc, a, del)
	if replay.RecordedAt != removed.RecordedAt || replay.User.Version != 3 {
		t.Fatal("delete replay changed state")
	}
	if _, e = accounts.Authenticate(ctx, logged.Token); !errors.Is(e, auth.ErrUnauthorized) {
		t.Fatal("deleted session", e)
	}
	if _, _, e = shares.Granted(ctx, share.ID, grant); !errors.Is(e, fileshare.ErrDenied) {
		t.Fatal("deleted grant usable", e)
	}
	if _, e = shares.Secret(ctx, record.TokenHash); !errors.Is(e, fileshare.ErrDenied) {
		t.Fatal("deleted share usable", e)
	}
	blocked := opRun(t, svc, a, adminops.Command{RequestID: opID(5), Action: adminops.SetEnabled, TargetID: id, ExpectedVersion: 3, Enabled: true})
	if blocked.Status != "REJECTED" {
		t.Fatal("enabled tombstone")
	}
	// Tombstone login stays unique, never reassigned to another user's history.
	if duplicate := opRun(t, svc, a, adminops.Command{RequestID: opID(6), Action: adminops.CreateUser, Login: "new-login", Password: "other-password"}); duplicate.Status != "REJECTED" {
		t.Fatal("tombstone login reused")
	}
	restore := adminops.Command{RequestID: opID(7), Action: adminops.RestoreUser, TargetID: id, ExpectedVersion: 3, Password: "replacement-password"}
	if same := opRun(t, svc, a, restore); same.Code != "PASSWORD_UNCHANGED" {
		t.Fatal("old restore password accepted")
	}
	restore.RequestID = opID(8)
	restore.Password = "restore-new-password"
	restored := opRun(t, svc, a, restore)
	if restored.Code != "USER_RESTORED" || restored.User.Enabled || restored.User.Deleted || restored.User.Version != 4 || restored.User.UserID != id {
		t.Fatal(restored)
	}
	opRun(t, svc, a, adminops.Command{RequestID: opID(9), Action: adminops.SetEnabled, TargetID: id, ExpectedVersion: 4, Enabled: true})
	if _, e = accounts.Login(ctx, "new-login", "restore-new-password"); e != nil {
		t.Fatal(e)
	}
	if _, e = accounts.Authenticate(ctx, logged.Token); !errors.Is(e, auth.ErrUnauthorized) {
		t.Fatal("restored old session")
	}
	if _, _, e = shares.Granted(ctx, share.ID, grant); !errors.Is(e, fileshare.ErrDenied) {
		t.Fatal("restored old grant")
	}
	if _, e = shares.Secret(ctx, record.TokenHash); !errors.Is(e, fileshare.ErrDenied) {
		t.Fatal("restored old share")
	}
	var intact bool
	e = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM product_file_shares WHERE share_id=$1 AND owner_id=$2 AND revoked_at IS NOT NULL) AND EXISTS(SELECT 1 FROM product_file_share_grants WHERE share_id=$1)`, share.ID, id).Scan(&intact)
	if e != nil || !intact {
		t.Fatal("history removed", e)
	}
	var raw string
	if e = pool.QueryRow(ctx, `SELECT string_agg(outcome::text,'') FROM product_admin_action_results`).Scan(&raw); e != nil {
		t.Fatal(e)
	}
	for _, secret := range []string{"original-password", "replacement-password", "restore-new-password", "password_hash", "token_hash", "synthetic-share-token"} {
		if strings.Contains(raw, secret) {
			t.Fatal("audit secret", secret)
		}
	}
}

func TestPostgresAdminLifecycleActiveDownloadsAndImmutableOwnership(t *testing.T) {
	for _, state := range []string{"PENDING", "FAILED", "CANCELED"} {
		t.Run(state, func(t *testing.T) {
			ctx, pool, _, a, _, repo := opFixture(t)
			svc, _ := adminops.NewService(repo, nil, []string{adminBinding})
			c := opRun(t, svc, a, adminops.Command{RequestID: opID(1), Action: adminops.CreateUser, Login: "history-owner", Password: "history-password"})
			// Seed a never-executed PENDING manifest. Domain transition setup is confined
			// to the disposable fixture; no provider or Job execution occurs.
			_, e := pool.Exec(ctx, `INSERT INTO acquisition_manifests(manifest_id,user_id,target_storage_binding_id,target_path,source_type,source_ref,expected_name,state,created_at,updated_at) VALUES($1,$2,$3,'/','https','https://synthetic.invalid/file','test.txt','PENDING',clock_timestamp(),clock_timestamp())`, opID(90), c.TargetID, adminBinding)
			if e != nil {
				t.Fatal(e)
			}
			if state != "PENDING" {
				if _, e = pool.Exec(ctx, `UPDATE acquisition_manifests SET state=$2 WHERE manifest_id=$1`, opID(90), state); e != nil {
					t.Fatal(e)
				}
			}
			r := opRun(t, svc, a, adminops.Command{RequestID: opID(2), Action: adminops.DeleteUser, TargetID: c.TargetID})
			if state == "PENDING" {
				if r.Code != "ACTIVE_DOWNLOADS" {
					t.Fatal(r)
				}
			} else if r.Status != "SUCCEEDED" {
				t.Fatal(r)
			}
			var owner string
			if e = pool.QueryRow(ctx, `SELECT user_id::text FROM acquisition_manifests WHERE manifest_id=$1`, opID(90)).Scan(&owner); e != nil || owner != c.TargetID {
				t.Fatal("ownership changed", e)
			}
		})
	}
}

func TestPostgresAdminLifecycleTargetAndVersionGuards(t *testing.T) {
	ctx, pool, accounts, a, _, repo := opFixture(t)
	svc, _ := adminops.NewService(repo, nil, []string{adminBinding})
	c := opRun(t, svc, a, adminops.Command{RequestID: opID(1), Action: adminops.CreateUser, Login: "rename-one", Password: "one-password"})
	opRun(t, svc, a, adminops.Command{RequestID: opID(2), Action: adminops.CreateUser, Login: "rename-two", Password: "two-password"})
	conflict := opRun(t, svc, a, adminops.Command{RequestID: opID(3), Action: adminops.RenameUser, TargetID: c.TargetID, Login: "rename-two"})
	if conflict.Code != "LOGIN_CONFLICT" {
		t.Fatal(conflict)
	}
	for i, action := range []string{adminops.RenameUser, adminops.ResetPassword, adminops.DeleteUser, adminops.RestoreUser} {
		cmd := adminops.Command{RequestID: opID(10 + i), Action: action, TargetID: string(a.Principal.UserID), Login: "new-login"}
		if action != adminops.RenameUser {
			cmd.Login = ""
		}
		if action == adminops.ResetPassword || action == adminops.RestoreUser {
			cmd.Password = "new-password"
		}
		if r := opRun(t, svc, a, cmd); r.Code != "TARGET_FORBIDDEN" {
			t.Fatal("ADMIN/self target", r)
		}
	}
	var enabled bool
	if e := pool.QueryRow(ctx, `SELECT enabled FROM product_users WHERE user_id=$1`, string(a.Principal.UserID)).Scan(&enabled); e != nil || !enabled {
		t.Fatal("last admin altered", e)
	}
	results := make(chan adminops.Result, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r, e := svc.Execute(ctx, a, adminops.Command{RequestID: opID(20 + i), Action: adminops.RenameUser, TargetID: c.TargetID, Login: fmt.Sprintf("winner-%d", i)})
			results <- r
			errs <- e
		}(i)
	}
	wg.Wait()
	wins, loses := 0, 0
	for i := 0; i < 2; i++ {
		r := <-results
		if e := <-errs; e != nil {
			t.Fatal(e)
		}
		if r.Status == "SUCCEEDED" {
			wins++
		}
		if r.Code == "VERSION_CONFLICT" {
			loses++
		}
	}
	if wins != 1 || loses != 1 {
		t.Fatal("CAS lost", wins, loses)
	}
	// No role inference from the caller: store rechecks actual actor state.
	if _, e := pool.Exec(ctx, `UPDATE product_users SET enabled=false WHERE user_id=$1`, string(a.Principal.UserID)); e != nil {
		t.Fatal(e)
	}
	if _, e := svc.Execute(ctx, a, adminops.Command{RequestID: opID(30), Action: adminops.DeleteUser, TargetID: c.TargetID, ExpectedVersion: 1}); e == nil {
		t.Fatal("stale actor permitted")
	}
	_ = accounts
}

func TestPostgresAdminLifecycleRollbackAndUnknown(t *testing.T) {
	for _, fault := range []string{"audit-failure", "backend-disconnect"} {
		t.Run(fault, func(t *testing.T) {
			ctx, pool, accounts, a, _, repo := opFixture(t)
			svc, _ := adminops.NewService(repo, nil, []string{adminBinding})
			created := opRun(t, svc, a, adminops.Command{RequestID: opID(1), Action: adminops.CreateUser, Login: "rollback-delete", Password: "rollback-password"})
			logged, e := accounts.Login(ctx, "rollback-delete", "rollback-password")
			if e != nil {
				t.Fatal(e)
			}
			c := adminops.Command{RequestID: opID(2), Action: adminops.DeleteUser, TargetID: created.TargetID}
			if fault == "audit-failure" {
				if _, e = pool.Exec(ctx, `ALTER TABLE product_admin_action_results ADD CONSTRAINT fail_lifecycle CHECK(false) NOT VALID`); e != nil {
					t.Fatal(e)
				}
			} else {
				repo.beforeUserCommit = func(ctx context.Context, tx pgx.Tx) error {
					_, e := pool.Exec(ctx, `SELECT pg_terminate_backend($1)`, tx.Conn().PgConn().PID())
					return e
				}
			}
			if _, e = svc.Execute(ctx, a, c); e == nil {
				t.Fatal("failed commit accepted")
			}
			if _, e = accounts.Authenticate(ctx, logged.Token); e != nil {
				t.Fatal("partial session revoke", e)
			}
			var clean bool
			if e = pool.QueryRow(ctx, `SELECT enabled AND deleted_at IS NULL AND deleted_by IS NULL AND admin_version=0 FROM product_users WHERE user_id=$1`, created.TargetID).Scan(&clean); e != nil || !clean {
				t.Fatal("partial deletion", e)
			}
			r, e := svc.Status(ctx, a, c.RequestID)
			if e != nil || r.Status != "UNKNOWN" {
				t.Fatal("uncertain success", r, e)
			}
			replay := opRun(t, svc, a, c)
			if replay.Status != "UNKNOWN" {
				t.Fatal("uncertain command auto-reexecuted")
			}
		})
	}
}

func TestPostgresAdminLifecycleDirectoryAndActualHTTP(t *testing.T) {
	ctx, pool, accounts, a, token, repo := opFixture(t)
	svc, _ := adminops.NewService(repo, nil, []string{adminBinding})
	for i := 0; i < 4; i++ {
		r := opRun(t, svc, a, adminops.Command{RequestID: opID(i + 1), Action: adminops.CreateUser, Login: fmt.Sprintf("search-user-%d", i), Password: "search-password"})
		if i == 0 {
			opRun(t, svc, a, adminops.Command{RequestID: opID(10), Action: adminops.DeleteUser, TargetID: r.TargetID})
		}
	}
	reader, _ := NewAdminReadRepository(pool)
	reads, _ := adminread.NewService(reader, []string{adminBinding})
	first, e := reads.SearchUsers(ctx, a.Principal, adminread.UserFilter{Limit: 2, Search: "search-user", Status: "ALL"})
	if e != nil || len(first.Items) != 2 || first.Total != 4 || first.NextCursor == "" {
		t.Fatal(first, e)
	}
	second, e := reads.SearchUsers(ctx, a.Principal, adminread.UserFilter{Limit: 2, Cursor: first.NextCursor, Search: "search-user", Status: "ALL"})
	if e != nil || second.Total != 4 || len(second.Items) != 2 || second.NextCursor != "" {
		t.Fatal(second, e)
	}
	deleted, e := reads.SearchUsers(ctx, a.Principal, adminread.UserFilter{Limit: 5, Status: "DELETED"})
	if e != nil || deleted.Total != 1 || deleted.Items[0].DeletedAt == nil {
		t.Fatal(deleted, e)
	}
	id := deleted.Items[0].UserID
	exact, e := reads.SearchUsers(ctx, a.Principal, adminread.UserFilter{Limit: 5, Search: id})
	if e != nil || exact.Total != 1 || exact.Items[0].UserID != id {
		t.Fatal(exact, e)
	}
	_, _, _, _, ownerReads, _ := adminFixtureNoReset(t, ctx, pool)
	h, e := producthttp.NewWorkspaceHandler(accounts, "http://127.0.0.1:8180", ownerReads, []producthttp.DisplayBinding{{ID: adminBinding, Label: "Fixture"}}, 17, producthttp.WithAdminReads(reads), producthttp.WithAdminOperations(svc))
	if e != nil {
		t.Fatal(e)
	}
	ordinary, e := accounts.Login(ctx, "search-user-1", "search-password")
	if e != nil {
		t.Fatal(e)
	}
	for _, path := range []string{"/api/v1/admin/users?q=search-user&status=ALL&limit=2", "/api/v1/admin/users/" + id} {
		for _, tok := range []string{"", token, ordinary.Token} {
			req := httptest.NewRequest("GET", "http://127.0.0.1:8180"+path, nil)
			if tok != "" {
				req.AddCookie(&http.Cookie{Name: producthttp.SessionCookieName, Value: tok})
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			want := 200
			if tok == "" {
				want = 401
			}
			if tok == ordinary.Token {
				want = 403
			}
			if w.Code != want {
				t.Fatal(w.Code, w.Body.String())
			}
			for _, secret := range []string{"password_hash", "token_hash", "search-password"} {
				if strings.Contains(w.Body.String(), secret) {
					t.Fatal("read leaked", secret)
				}
			}
		}
	}
	raw, _ := json.Marshal(deleted)
	if strings.Contains(string(raw), "password") {
		t.Fatal("directory leak")
	}
}

func TestPostgresAdminLifecycleAdmissionSerialization(t *testing.T) {
	f := newAdmissionFixture(t)
	users, _ := NewAuthRepository(f.pool)
	accounts, _ := auth.NewService(users)
	logged, e := accounts.Login(f.ctx, "admission-admin", "synthetic-password")
	if e != nil {
		t.Fatal(e)
	}
	a := adminops.Actor{Principal: logged.Principal, SessionDigest: auth.TokenDigest(logged.Token)}
	repo, _ := NewAdminOperationsRepository(f.pool)
	svc, _ := adminops.NewService(repo, nil, []string{string(f.input.BindingID)})
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	f.store.afterManifest = func(ctx context.Context, tx pgx.Tx) error {
		close(entered)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	admitted := make(chan error, 1)
	go func() { _, e := f.service.Admit(f.ctx, f.p, f.input); admitted <- e }()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("admission not entered")
	}
	result := make(chan adminops.Result, 1)
	errs := make(chan error, 1)
	go func() {
		r, e := svc.Execute(f.ctx, a, adminops.Command{RequestID: opID(1), Action: adminops.DeleteUser, TargetID: string(f.p.UserID)})
		result <- r
		errs <- e
	}()
	// Observe real conflicting locks before allowing product admission to commit.
	waitUserLock(t, f.ctx, f.pool)
	unblock()
	if e = <-admitted; e != nil {
		t.Fatal(e)
	}
	if e = <-errs; e != nil {
		t.Fatal(e)
	}
	if r := <-result; r.Code != "ACTIVE_DOWNLOADS" {
		t.Fatal("delete raced past admission", r)
	}
	for i, state := range []string{"ACTIVE", "AWAITING_VISIBILITY", "AWAITING_CANONICAL", "RECOVERY_REQUIRED"} {
		if _, e = f.pool.Exec(f.ctx, `UPDATE acquisition_manifests SET state=$2 WHERE user_id=$1`, string(f.p.UserID), state); e != nil {
			t.Fatal(e)
		}
		r := opRun(t, svc, a, adminops.Command{RequestID: opID(10 + i), Action: adminops.DeleteUser, TargetID: string(f.p.UserID)})
		if r.Code != "ACTIVE_DOWNLOADS" {
			t.Fatal(state, r)
		}
	}
	assertAdmissionCounts(t, f, 1, 1, 1)
}

func TestPostgresAdminLifecycleMigration16To17PreservesData(t *testing.T) {
	ctx := context.Background()
	pool := integrationPool(t, ctx)
	resetTestSchema(t, ctx, pool)
	all, e := migrations.All()
	if e != nil {
		t.Fatal(e)
	}
	for _, m := range all[:16] {
		if _, e = pool.Exec(ctx, m.SQL); e != nil {
			t.Fatal(e)
		}
	}
	_, e = pool.Exec(ctx, `INSERT INTO product_users(user_id,login,password_hash,role,enabled,created_at,updated_at) VALUES($1,'legacy-user',decode(repeat('41',60),'hex'),'USER',true,now(),now());`, opID(1))
	if e != nil {
		t.Fatal(e)
	}
	_, e = pool.Exec(ctx, `INSERT INTO product_sessions(token_hash,user_id,created_at,expires_at) VALUES(decode(repeat('42',32),'hex'),$1,now(),now()+interval '1 day')`, opID(1))
	if e != nil {
		t.Fatal(e)
	}
	var before, after string
	q := `SELECT json_build_array((SELECT row_to_json(u) FROM product_users u),(SELECT row_to_json(s) FROM product_sessions s))::text`
	if e = pool.QueryRow(ctx, q).Scan(&before); e != nil {
		t.Fatal(e)
	}
	tx, e := pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, all[16].SQL); e != nil {
		t.Fatal(e)
	}
	if e = tx.Rollback(ctx); e != nil {
		t.Fatal(e)
	}
	if e = pool.QueryRow(ctx, q).Scan(&after); e != nil || before != after {
		t.Fatal("transactional migration rollback changed legacy data", e)
	}
	if _, e = pool.Exec(ctx, all[16].SQL); e != nil {
		t.Fatal(e)
	}
	var intact bool
	e = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM product_users WHERE user_id=$1 AND password_hash=decode(repeat('41',60),'hex') AND enabled AND deleted_at IS NULL AND deleted_by IS NULL AND admin_version=0) AND EXISTS(SELECT 1 FROM product_sessions WHERE user_id=$1 AND revoked_at IS NULL)`, opID(1)).Scan(&intact)
	if e != nil || !intact {
		t.Fatal("migration changed existing account/session", e)
	}
	if _, e = pool.Exec(ctx, `UPDATE product_users SET deleted_at=now(),deleted_by=user_id WHERE user_id=$1`, opID(1)); e == nil {
		t.Fatal("enabled tombstone allowed")
	}
}

func waitUserLock(t *testing.T, ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}) {
	t.Helper()
	limit := time.After(10 * time.Second)
	for {
		var waiting bool
		e := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%FROM product_users%')`).Scan(&waiting)
		if e != nil {
			t.Fatal(e)
		}
		if waiting {
			return
		}
		select {
		case <-limit:
			t.Fatal("account contender never waited on lock")
		case <-time.After(time.Millisecond):
		}
	}
}

func TestPostgresAdminLifecycleConcurrentStaleLoginAfterReset(t *testing.T) {
	ctx, pool, accounts, a, _, repo := opFixture(t)
	svc, _ := adminops.NewService(repo, nil, []string{adminBinding})
	created := opRun(t, svc, a, adminops.Command{RequestID: opID(1), Action: adminops.CreateUser, Login: "racing-password", Password: "original-password"})
	authRepo, _ := NewAuthRepository(pool)
	old, e := authRepo.UserByLogin(ctx, "racing-password")
	if e != nil {
		t.Fatal(e)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	repo.beforeUserCommit = func(ctx context.Context, tx pgx.Tx) error {
		close(entered)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	changed := make(chan error, 1)
	go func() {
		_, e := svc.Execute(ctx, a, adminops.Command{RequestID: opID(2), Action: adminops.ResetPassword, TargetID: created.TargetID, Password: "replacement-password"})
		changed <- e
	}()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("reset not entered")
	}
	login := make(chan error, 1)
	digest := auth.TokenDigest("stale-synthetic-session")
	go func() {
		_, _, e := authRepo.CreateSession(ctx, old.ID, digest[:], old.PasswordHash, auth.SessionTTL)
		login <- e
	}()
	waitUserLock(t, ctx, pool)
	unblock()
	if e = <-changed; e != nil {
		t.Fatal(e)
	}
	if e = <-login; !errors.Is(e, auth.ErrUnauthorized) {
		t.Fatal("stale hash created session", e)
	}
	if _, e = accounts.Login(ctx, "racing-password", "replacement-password"); e != nil {
		t.Fatal(e)
	}
}
