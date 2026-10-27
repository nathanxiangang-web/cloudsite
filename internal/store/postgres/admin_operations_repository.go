package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/nathanxiangang-web/panta/internal/adminops"
	"github.com/nathanxiangang-web/panta/internal/auth"
)

type AdminOperationsRepository struct {
	db               authDB
	beforeUserCommit func(context.Context, pgx.Tx) error
	afterPendingLock func(context.Context, pgx.Tx) error // Isolated concurrency test seam only.
}

func NewAdminOperationsRepository(db authDB) (*AdminOperationsRepository, error) {
	if db == nil || reflect.ValueOf(db).Kind() == reflect.Pointer && reflect.ValueOf(db).IsNil() {
		return nil, adminops.ErrUnavailable
	}
	return &AdminOperationsRepository{db: db}, nil
}

var _ adminops.Store = (*AdminOperationsRepository)(nil)

// Account first, session second. Target is USER-only, so two administrators
// never lock each other's target rows. Login creation uses the same account lock.
func adminActor(ctx context.Context, tx pgx.Tx, a adminops.Actor) error {
	var login, role string
	var enabled bool
	e := tx.QueryRow(ctx, `SELECT login,role,enabled FROM product_users WHERE user_id=$1 AND deleted_at IS NULL FOR SHARE`, string(a.Principal.UserID)).Scan(&login, &role, &enabled)
	if errors.Is(e, pgx.ErrNoRows) {
		return auth.ErrUnauthorized
	}
	if e != nil {
		return e
	}
	if !enabled || role != "ADMIN" || a.Principal.Role != auth.RoleAdmin || login != a.Principal.Login {
		return adminops.ErrForbidden
	}
	var valid bool
	e = tx.QueryRow(ctx, `SELECT expires_at>clock_timestamp() AND revoked_at IS NULL FROM product_sessions WHERE token_hash=$1 AND user_id=$2 FOR SHARE`, a.SessionDigest[:], string(a.Principal.UserID)).Scan(&valid)
	if errors.Is(e, pgx.ErrNoRows) || e == nil && !valid {
		return auth.ErrUnauthorized
	}
	return e
}
func (r *AdminOperationsRepository) begin(ctx context.Context, a adminops.Actor) (pgx.Tx, error) {
	tx, e := r.db.Begin(ctx)
	if e != nil {
		return nil, adminops.ErrUnavailable
	}
	if e = adminActor(ctx, tx, a); e != nil {
		_ = tx.Rollback(context.Background())
		return nil, e
	}
	return tx, nil
}
func adminRollback(tx pgx.Tx) { _ = tx.Rollback(context.Background()) }
func actionResult(ctx context.Context, tx pgx.Tx, a adminops.Actor, id string) (adminops.Result, error) {
	var result adminops.Result
	var target *string
	var raw []byte
	e := tx.QueryRow(ctx, `SELECT a.request_id::text,a.action,a.target_id::text,a.created_at,r.outcome FROM product_admin_actions a LEFT JOIN product_admin_action_results r USING(actor_id,request_id) WHERE a.actor_id=$1 AND a.request_id=$2`, string(a.Principal.UserID), id).Scan(&result.RequestID, &result.Action, &target, &result.RecordedAt, &raw)
	if errors.Is(e, pgx.ErrNoRows) {
		return result, adminops.ErrNotFound
	}
	if e != nil {
		return result, e
	}
	if raw != nil {
		e = json.Unmarshal(raw, &result)
		return result, e
	}
	if target != nil {
		result.TargetID = *target
	}
	result.Status = "UNKNOWN"
	result.Code = "RESERVED_NO_CONFIRMED_RESULT"
	return result, nil
}
func appendAdminResult(ctx context.Context, tx pgx.Tx, a adminops.Actor, result adminops.Result) (adminops.Result, error) {
	if e := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&result.RecordedAt); e != nil {
		return adminops.Result{}, e
	}
	raw, e := json.Marshal(result)
	if e != nil {
		return adminops.Result{}, e
	}
	_, e = tx.Exec(ctx, `INSERT INTO product_admin_action_results(actor_id,request_id,outcome,recorded_at) VALUES($1,$2,$3,$4)`, string(a.Principal.UserID), result.RequestID, raw, result.RecordedAt)
	if e == nil {
		e = tx.Commit(ctx)
	}
	return result, e
}
func rejected(c adminops.Command, code string) adminops.Result {
	return adminops.Result{RequestID: c.RequestID, Action: c.Action, TargetID: c.TargetID, Status: "REJECTED", Code: code}
}
func (r *AdminOperationsRepository) Reserve(ctx context.Context, a adminops.Actor, c adminops.Command, bindings []string) (adminops.Result, bool, error) {
	if !c.Valid() {
		return adminops.Result{}, false, adminops.ErrInvalid
	}
	tx, e := r.begin(ctx, a)
	if e != nil {
		return adminops.Result{}, false, e
	}
	defer adminRollback(tx)
	digest := c.Digest()
	var target any
	if c.TargetID != "" {
		target = c.TargetID
	}
	tag, e := tx.Exec(ctx, `INSERT INTO product_admin_actions(actor_id,request_id,action,target_id,intent_digest,created_at) VALUES($1,$2,$3,$4,$5,clock_timestamp()) ON CONFLICT DO NOTHING`, string(a.Principal.UserID), c.RequestID, c.Action, target, digest[:])
	if e != nil {
		return adminops.Result{}, false, e
	}
	if tag.RowsAffected() == 0 {
		var stored []byte
		e = tx.QueryRow(ctx, `SELECT intent_digest FROM product_admin_actions WHERE actor_id=$1 AND request_id=$2`, string(a.Principal.UserID), c.RequestID).Scan(&stored)
		if e != nil {
			return adminops.Result{}, false, e
		}
		if string(stored) != string(digest[:]) {
			return adminops.Result{}, false, adminops.ErrConflict
		}
		result, e := actionResult(ctx, tx, a, c.RequestID)
		return result, false, e
	}
	var reservedTarget *adminops.ProbeTarget
	if c.Action == adminops.ProbeAccount {
		allowed := false
		for _, id := range bindings {
			if id == c.TargetID {
				allowed = true
			}
		}
		code := ""
		if !allowed {
			code = "SCOPE_REJECTED"
		}
		target, e := probeMapping(ctx, tx, c.TargetID)
		if e != nil {
			code = "NOT_CONNECTED"
		}
		if code == "" {
			// Atomic DB-time CAS means parallel processes cannot double-probe a connection.
			tag, e = tx.Exec(ctx, `INSERT INTO product_admin_probe_cooldowns(connection_id,next_allowed_at) VALUES($1,clock_timestamp()+interval '60 seconds') ON CONFLICT(connection_id) DO UPDATE SET next_allowed_at=EXCLUDED.next_allowed_at WHERE product_admin_probe_cooldowns.next_allowed_at<=clock_timestamp()`, target.ConnectionID)
			if e != nil {
				return adminops.Result{}, false, e
			}
			if tag.RowsAffected() != 1 {
				code = "RATE_LIMITED"
			}
		}
		if code != "" {
			result, e := appendAdminResult(ctx, tx, a, rejected(c, code))
			return result, false, e
		}
		reservedTarget = &target
	}
	result, e := actionResult(ctx, tx, a, c.RequestID)
	result.ProbeTarget = reservedTarget
	if e == nil {
		e = tx.Commit(ctx)
	}
	return result, e == nil, e
}
func lockedAdminAction(ctx context.Context, tx pgx.Tx, a adminops.Actor, c adminops.Command) (adminops.Result, bool, error) {
	var digest []byte
	e := tx.QueryRow(ctx, `SELECT intent_digest FROM product_admin_actions WHERE actor_id=$1 AND request_id=$2 FOR UPDATE`, string(a.Principal.UserID), c.RequestID).Scan(&digest)
	d := c.Digest()
	if e != nil {
		return adminops.Result{}, false, e
	}
	if string(digest) != string(d[:]) {
		return adminops.Result{}, false, adminops.ErrConflict
	}
	result, e := actionResult(ctx, tx, a, c.RequestID)
	return result, result.Status != "UNKNOWN", e
}
func (r *AdminOperationsRepository) ApplyUser(ctx context.Context, a adminops.Actor, c adminops.Command) (adminops.Result, error) {
	if !c.Valid() || c.Action == adminops.ProbeAccount || c.Action == adminops.CancelPending {
		return adminops.Result{}, adminops.ErrInvalid
	}
	tx, e := r.begin(ctx, a)
	if e != nil {
		return adminops.Result{}, e
	}
	defer adminRollback(tx)
	result, done, e := lockedAdminAction(ctx, tx, a, c)
	if e != nil || done {
		return result, e
	}
	result = rejected(c, "CONFLICT")
	if c.Action == adminops.CreateUser {
		// Existing auth validation, hashing and UUID provisioning; transaction-bound
		// store ensures account insertion and audit outcome cannot commit separately.
		// Constraint conflicts must rollback only the insertion, not the audit intent.
		save, e := tx.Begin(ctx)
		if e != nil {
			return adminops.Result{}, e
		}
		child, _ := NewAuthRepository(save)
		svc, e := auth.NewService(child)
		if e != nil {
			return adminops.Result{}, e
		}
		p, e := svc.Register(ctx, c.Login, c.Password)
		if e == nil {
			if e = save.Commit(ctx); e != nil {
				return adminops.Result{}, e
			}
			result.Status = "SUCCEEDED"
			result.Code = "USER_CREATED"
			result.TargetID = string(p.UserID)
			result.User = &adminops.User{UserID: string(p.UserID), Login: p.Login, Enabled: true, Version: 0}
		} else {
			_ = save.Rollback(ctx)
			if !errors.Is(e, auth.ErrConflict) {
				return adminops.Result{}, e
			}
		}
	} else {
		result, e = r.changeUser(ctx, tx, a, c)
		if e != nil {
			return adminops.Result{}, e
		}
	}
	if r.beforeUserCommit != nil {
		if e := r.beforeUserCommit(ctx, tx); e != nil {
			return adminops.Result{}, e
		}
	}
	return appendAdminResult(ctx, tx, a, result)
}
func probeMapping(ctx context.Context, tx pgx.Tx, id string) (adminops.ProbeTarget, error) {
	var t adminops.ProbeTarget
	e := tx.QueryRow(ctx, `SELECT b.storage_binding_id::text,c.storage_connection_id::text,c.credential_ref FROM storage_bindings b JOIN storage_connections c ON c.storage_connection_id=b.storage_connection_id WHERE b.storage_binding_id=$1 AND b.status='ACTIVE' AND c.status='ACTIVE' AND c.provider_type='115' AND c.credential_ref IS NOT NULL AND c.credential_ref<>'' FOR SHARE OF b,c`, id).Scan(&t.BindingID, &t.ConnectionID, &t.CredentialRef)
	return t, e
}
func (r *AdminOperationsRepository) ProbeTarget(ctx context.Context, a adminops.Actor, id string) (adminops.ProbeTarget, error) {
	tx, e := r.begin(ctx, a)
	if e != nil {
		return adminops.ProbeTarget{}, e
	}
	defer adminRollback(tx)
	return probeMapping(ctx, tx, id)
}
func (r *AdminOperationsRepository) CompleteProbe(ctx context.Context, a adminops.Actor, c adminops.Command, target adminops.ProbeTarget, capacity adminops.Capacity) (adminops.Result, error) {
	if !c.Valid() || c.Action != adminops.ProbeAccount {
		return adminops.Result{}, adminops.ErrInvalid
	}
	tx, e := r.begin(ctx, a)
	if e != nil {
		return adminops.Result{}, e
	}
	defer adminRollback(tx)
	result, done, e := lockedAdminAction(ctx, tx, a, c)
	if e != nil || done {
		return result, e
	}
	current, e := probeMapping(ctx, tx, c.TargetID)
	if e != nil || current != target {
		capacity = adminops.Capacity{Status: "UNKNOWN", Source: "115_GET_INFO", ProbedAt: time.Now().UTC()}
	}
	result.Status = "SUCCEEDED"
	result.Code = "PROBE_RECORDED"
	result.Capacity = &capacity
	return appendAdminResult(ctx, tx, a, result)
}
func (r *AdminOperationsRepository) Status(ctx context.Context, a adminops.Actor, id string) (adminops.Result, error) {
	tx, e := r.begin(ctx, a)
	if e != nil {
		return adminops.Result{}, e
	}
	defer adminRollback(tx)
	return actionResult(ctx, tx, a, id)
}
