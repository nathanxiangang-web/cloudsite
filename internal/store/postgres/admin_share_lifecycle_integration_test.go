package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nathanxiangang-web/panta/internal/adminops"
	"github.com/nathanxiangang-web/panta/internal/adminread"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/fileaccess"
	"github.com/nathanxiangang-web/panta/internal/fileshare"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

// seedShare creates one active share for owner and returns its record. ttl is
// passed through to the real repository, so the DB owns all time stamps.
func seedShare(t *testing.T, ctx context.Context, repo *ShareRepository, id string, owner auth.UserID, token string) fileshare.Record {
	t.Helper()
	record := fileshare.Record{
		ID:        id,
		Owner:     owner,
		Target:    fileaccess.Request{BindingID: storage.BindingID(adminBinding), ResourceID: "synthetic", Path: "/share.txt"},
		Name:      "share.txt",
		Size:      4,
		TokenHash: fileshare.SecretHash(token),
	}
	share, e := repo.Create(ctx, record, time.Hour)
	if e != nil {
		t.Fatal(e)
	}
	return share
}

// TestPostgresAdminShareRevokeLifecycleAndHistory exercises the whole-site
// administrator revocation: success, idempotent replay, already-revoked,
// not-found, and the fact that both Open (grant) and Redeem (secret + issue)
// paths are blocked while all durable history survives.
func TestPostgresAdminShareRevokeLifecycleAndHistory(t *testing.T) {
	ctx, pool, accounts, a, _, repo := opFixture(t)
	svc, _ := adminops.NewService(repo, nil, []string{adminBinding})
	owner, e := accounts.Provision(ctx, "share-revoke-owner", "owner-password-123", auth.RoleUser)
	if e != nil {
		t.Fatal(e)
	}
	shares, _ := NewShareRepository(pool)
	token := "revoke-share-token"
	share := seedShare(t, ctx, shares, opID(700), owner.UserID, token)
	grant := fileshare.SecretHash("existing-grant-token")
	if e = shares.IssueGrant(ctx, share.ID, grant); e != nil {
		t.Fatal(e)
	}
	// Positive control: before revocation both paths resolve.
	if _, _, e = shares.Granted(ctx, share.ID, grant); e != nil {
		t.Fatal("pre-revoke grant", e)
	}
	if _, e = shares.Secret(ctx, share.TokenHash); e != nil {
		t.Fatal("pre-revoke secret", e)
	}

	revoke := adminops.Command{RequestID: opID(1), Action: adminops.RevokeShare, TargetID: share.ID}
	first := opRun(t, svc, a, revoke)
	if first.Status != "SUCCEEDED" || first.Code != "SHARE_REVOKED" || first.TargetID != share.ID {
		t.Fatal("revoke", first)
	}
	// Open path (Granted) is blocked; Redeem path (Secret and IssueGrant) is blocked.
	if _, _, e = shares.Granted(ctx, share.ID, grant); !errors.Is(e, fileshare.ErrDenied) {
		t.Fatal("revoked grant still opens", e)
	}
	if _, e = shares.Secret(ctx, share.TokenHash); !errors.Is(e, fileshare.ErrDenied) {
		t.Fatal("revoked share still redeems", e)
	}
	if e = shares.IssueGrant(ctx, share.ID, fileshare.SecretHash("new-grant")); !errors.Is(e, fileshare.ErrDenied) {
		t.Fatal("revoked share issued new grant", e)
	}
	// Idempotent replay returns the recorded terminal outcome unchanged.
	replay := opRun(t, svc, a, revoke)
	if replay.Code != "SHARE_REVOKED" || !replay.RecordedAt.Equal(first.RecordedAt) {
		t.Fatal("non-idempotent revoke", replay)
	}
	// A fresh request id against the same share reports already-revoked.
	again := opRun(t, svc, a, adminops.Command{RequestID: opID(2), Action: adminops.RevokeShare, TargetID: share.ID})
	if again.Status != "REJECTED" || again.Code != "SHARE_ALREADY_REVOKED" {
		t.Fatal("already-revoked", again)
	}
	// Unknown share is not-found, never forbidden or silent success.
	missing := opRun(t, svc, a, adminops.Command{RequestID: opID(3), Action: adminops.RevokeShare, TargetID: opID(999)})
	if missing.Status != "REJECTED" || missing.Code != "SHARE_NOT_FOUND" {
		t.Fatal("not-found", missing)
	}
	// History is durable: the grant row and the revoked share survive untouched.
	var intact bool
	e = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM product_file_shares WHERE share_id=$1 AND revoked_at IS NOT NULL AND owner_id=$2) AND (SELECT count(*) FROM product_file_share_grants WHERE share_id=$1)=1`, share.ID, string(owner.UserID)).Scan(&intact)
	if e != nil || !intact {
		t.Fatal("history removed", e)
	}
	// Unrelated tables are unaffected by a share-only revocation.
	var ownerState, actionCount string
	if e = pool.QueryRow(ctx, `SELECT enabled::text FROM product_users WHERE user_id=$1`, string(owner.UserID)).Scan(&ownerState); e != nil || ownerState != "true" {
		t.Fatal("owner mutated", e)
	}
	if e = pool.QueryRow(ctx, `SELECT count(*)::text FROM product_admin_actions WHERE action='REVOKE_SHARE'`).Scan(&actionCount); e != nil || actionCount != "3" {
		t.Fatal("audit intent count", e, actionCount)
	}
	// The audit outcome never captures authentication secrets.
	var raw string
	if e = pool.QueryRow(ctx, `SELECT string_agg(outcome::text,'') FROM product_admin_action_results`).Scan(&raw); e != nil {
		t.Fatal(e)
	}
	for _, secret := range []string{"token_hash", "code_hash", "revoke-share-token", "existing-grant-token"} {
		if strings.Contains(raw, secret) {
			t.Fatal("audit secret", secret)
		}
	}
}

// TestPostgresAdminShareDirectoryPaginationFiltersAndTotal checks the read
// projection: DB-aggregated total, keyset pages, owner/status filters, and the
// absence of any hash material in the wire projection.
func TestPostgresAdminShareDirectoryPaginationFiltersAndTotal(t *testing.T) {
	ctx, pool, accounts, a, _, _ := opFixture(t)
	ownerA, e := accounts.Provision(ctx, "share-dir-owner-a", "owner-a-password-123", auth.RoleUser)
	if e != nil {
		t.Fatal(e)
	}
	ownerB, e := accounts.Provision(ctx, "share-dir-owner-b", "owner-b-password-123", auth.RoleUser)
	if e != nil {
		t.Fatal(e)
	}
	shares, _ := NewShareRepository(pool)
	// ownerA: 3 active, 1 revoked, 1 expired. ownerB: 1 active.
	for n := 1; n <= 3; n++ {
		seedShare(t, ctx, shares, opID(800+n), ownerA.UserID, "dir-a-active-"+string(rune('0'+n)))
	}
	revoked := seedShare(t, ctx, shares, opID(810), ownerA.UserID, "dir-a-revoked")
	if e = shares.Revoke(ctx, ownerA.UserID, revoked.ID); e != nil {
		t.Fatal(e)
	}
	expired := seedShare(t, ctx, shares, opID(811), ownerA.UserID, "dir-a-expired")
	if _, e = pool.Exec(ctx, `UPDATE product_file_shares SET created_at=clock_timestamp()-interval '2 hours',expires_at=clock_timestamp()-interval '1 hour' WHERE share_id=$1`, expired.ID); e != nil {
		t.Fatal(e)
	}
	seedShare(t, ctx, shares, opID(812), ownerB.UserID, "dir-b-active")

	reader, _ := NewAdminReadRepository(pool)
	reads, _ := adminread.NewService(reader, []string{adminBinding})

	all, e := reads.SearchShares(ctx, a.Principal, adminread.ShareFilter{Limit: 50, Status: "ALL"})
	if e != nil || all.Total != 6 || len(all.Items) != 6 || all.Scope != "FILTERED_PRODUCT_SHARES" {
		t.Fatal("all", all.Total, len(all.Items), e)
	}
	active, e := reads.SearchShares(ctx, a.Principal, adminread.ShareFilter{Limit: 50, Status: "ACTIVE"})
	if e != nil || active.Total != 4 {
		t.Fatal("active", active.Total, e)
	}
	for _, item := range active.Items {
		if !item.Active {
			t.Fatal("active filter returned inactive", item)
		}
	}
	revokedPage, e := reads.SearchShares(ctx, a.Principal, adminread.ShareFilter{Limit: 50, Status: "REVOKED"})
	if e != nil || revokedPage.Total != 1 || revokedPage.Items[0].ShareID != revoked.ID {
		t.Fatal("revoked", revokedPage, e)
	}
	expiredPage, e := reads.SearchShares(ctx, a.Principal, adminread.ShareFilter{Limit: 50, Status: "EXPIRED"})
	if e != nil || expiredPage.Total != 1 || expiredPage.Items[0].ShareID != expired.ID {
		t.Fatal("expired", expiredPage, e)
	}
	owned, e := reads.SearchShares(ctx, a.Principal, adminread.ShareFilter{Limit: 50, Owner: string(ownerA.UserID), Status: "ALL"})
	if e != nil || owned.Total != 5 {
		t.Fatal("owner filter", owned.Total, e)
	}
	for _, item := range owned.Items {
		if item.OwnerID != string(ownerA.UserID) {
			t.Fatal("owner filter leaked", item)
		}
	}
	// Keyset pagination over the whole site, stable and complete.
	seen := map[string]bool{}
	cursor, pages := "", 0
	for {
		page, e := reads.SearchShares(ctx, a.Principal, adminread.ShareFilter{Limit: 2, Cursor: cursor, Status: "ALL"})
		if e != nil {
			t.Fatal(e)
		}
		if page.Total != 6 {
			t.Fatal("page total", page.Total)
		}
		for _, item := range page.Items {
			if seen[item.ShareID] {
				t.Fatal("duplicate share across pages", item.ShareID)
			}
			seen[item.ShareID] = true
		}
		pages++
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
		if pages > 6 {
			t.Fatal("pagination did not terminate")
		}
	}
	if len(seen) != 6 {
		t.Fatal("pagination missed shares", len(seen))
	}
	// The wire projection must never carry authentication hashes.
	raw, _ := json.Marshal(all)
	if strings.Contains(string(raw), "token_hash") || strings.Contains(string(raw), "code_hash") {
		t.Fatal("directory leaked hashes")
	}
}

func TestPostgresAdminShareRollbackKeepsGrantAndUnknownIntent(t *testing.T) {
	ctx, pool, accounts, actor, _, repo := opFixture(t)
	owner, e := accounts.Provision(ctx, "share-rollback-owner", "isolated-password", auth.RoleUser)
	if e != nil {
		t.Fatal(e)
	}
	shares, _ := NewShareRepository(pool)
	share := seedShare(t, ctx, shares, opID(720), owner.UserID, "rollback-token")
	grant := fileshare.SecretHash("rollback-grant")
	if e = shares.IssueGrant(ctx, share.ID, grant); e != nil {
		t.Fatal(e)
	}
	command := adminops.Command{RequestID: opID(721), Action: adminops.RevokeShare, TargetID: share.ID}
	svc, _ := adminops.NewService(repo, nil, []string{adminBinding})
	repo.beforeUserCommit = func(context.Context, pgx.Tx) error { return errors.New("isolated commit failure") }
	if _, e = svc.Execute(ctx, actor, command); !errors.Is(e, adminops.ErrUnavailable) {
		t.Fatal("failure not surfaced", e)
	}
	var revoked, outcome bool
	if e = pool.QueryRow(ctx, `SELECT revoked_at IS NOT NULL,EXISTS(SELECT 1 FROM product_admin_action_results WHERE actor_id=$2 AND request_id=$3) FROM product_file_shares WHERE share_id=$1`, share.ID, string(actor.Principal.UserID), command.RequestID).Scan(&revoked, &outcome); e != nil || revoked || outcome {
		t.Fatal("partial revoke committed", revoked, outcome, e)
	}
	if _, _, e = shares.Granted(ctx, share.ID, grant); e != nil {
		t.Fatal("grant revoked despite rollback", e)
	}
	// A reserved but unconfirmed operation is never automatically applied again.
	repo.beforeUserCommit = nil
	result, e := svc.Execute(ctx, actor, command)
	if e != nil || result.Status != "UNKNOWN" || result.Code != "RESERVED_NO_CONFIRMED_RESULT" {
		t.Fatal("intent replay", result, e)
	}
	if _, _, e = shares.Granted(ctx, share.ID, grant); e != nil {
		t.Fatal("replay mutated share", e)
	}
}

func TestPostgresAdminShareRechecksSessionAndIntentBeforeWrite(t *testing.T) {
	ctx, pool, accounts, actor, token, repo := opFixture(t)
	owner, e := accounts.Provision(ctx, "share-session-owner", "isolated-password", auth.RoleUser)
	if e != nil {
		t.Fatal(e)
	}
	shares, _ := NewShareRepository(pool)
	share := seedShare(t, ctx, shares, opID(730), owner.UserID, "session-token")
	command := adminops.Command{RequestID: opID(731), Action: adminops.RevokeShare, TargetID: share.ID}
	if _, fresh, e := repo.Reserve(ctx, actor, command, []string{adminBinding}); e != nil || !fresh {
		t.Fatal("reserve", fresh, e)
	}
	changed := command
	changed.TargetID = opID(732)
	if _, e = repo.ApplyShare(ctx, actor, changed); !errors.Is(e, adminops.ErrConflict) {
		t.Fatal("changed intent accepted", e)
	}
	if e = accounts.Logout(ctx, token); e != nil {
		t.Fatal(e)
	}
	if _, e = repo.ApplyShare(ctx, actor, command); !errors.Is(e, auth.ErrUnauthorized) {
		t.Fatal("revoked session accepted", e)
	}
	var unchanged bool
	if e = pool.QueryRow(ctx, `SELECT revoked_at IS NULL AND NOT EXISTS(SELECT 1 FROM product_admin_action_results WHERE actor_id=$2 AND request_id=$3) FROM product_file_shares WHERE share_id=$1`, share.ID, string(actor.Principal.UserID), command.RequestID).Scan(&unchanged); e != nil || !unchanged {
		t.Fatal("unauthorized mutation", unchanged, e)
	}
}

func TestPostgresAdminShareConcurrentRevokesSerializeAtShareRow(t *testing.T) {
	ctx, pool, accounts, actor, _, first := opFixture(t)
	owner, e := accounts.Provision(ctx, "share-lock-owner", "isolated-password", auth.RoleUser)
	if e != nil {
		t.Fatal(e)
	}
	shares, _ := NewShareRepository(pool)
	share := seedShare(t, ctx, shares, opID(740), owner.UserID, "concurrent-token")
	entered, release := make(chan struct{}), make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	first.beforeUserCommit = func(ctx context.Context, _ pgx.Tx) error {
		close(entered)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	second, _ := NewAdminOperationsRepository(pool)
	svcA, _ := adminops.NewService(first, nil, []string{adminBinding})
	svcB, _ := adminops.NewService(second, nil, []string{adminBinding})
	type outcome struct {
		result adminops.Result
		err    error
	}
	resultA, resultB := make(chan outcome, 1), make(chan outcome, 1)
	go func() {
		r, e := svcA.Execute(ctx, actor, adminops.Command{RequestID: opID(741), Action: adminops.RevokeShare, TargetID: share.ID})
		resultA <- outcome{r, e}
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first revoke never held its transaction")
	}
	go func() {
		r, e := svcB.Execute(ctx, actor, adminops.Command{RequestID: opID(742), Action: adminops.RevokeShare, TargetID: share.ID})
		resultB <- outcome{r, e}
	}()
	// Observe a real blocked SELECT FOR UPDATE, not a sleep-based ordering guess.
	deadline := time.Now().Add(5 * time.Second)
	for {
		var waiting bool
		if e = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE 'SELECT revoked_at FROM product_file_shares%')`).Scan(&waiting); e != nil {
			t.Fatal(e)
		}
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("second revoke did not wait on share row")
		}
		time.Sleep(10 * time.Millisecond)
	}
	close(release)
	for index, ch := range []chan outcome{resultA, resultB} {
		select {
		case got := <-ch:
			want := "SHARE_REVOKED"
			if index == 1 {
				want = "SHARE_ALREADY_REVOKED"
			}
			if got.err != nil || got.result.Code != want {
				t.Fatal("serialized outcome", got.result, got.err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("revoke did not finish")
		}
	}
	var receipts int
	if e = pool.QueryRow(ctx, `SELECT count(*) FROM product_admin_action_results WHERE actor_id=$1`, string(actor.Principal.UserID)).Scan(&receipts); e != nil || receipts != 2 {
		t.Fatal("missing terminal audit", receipts, e)
	}
}
