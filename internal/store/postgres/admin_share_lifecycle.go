package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/nathanxiangang-web/panta/internal/adminops"
)

var _ adminops.ShareApplier = (*AdminOperationsRepository)(nil)

// revokeShare runs inside the authorized action transaction. Locking the share
// row serializes concurrent administrator revocations and owner revocations,
// while the audit intent/outcome bookkeeping stays owned by lockedAdminAction.
func (r *AdminOperationsRepository) revokeShare(ctx context.Context, tx pgx.Tx, c adminops.Command) (adminops.Result, error) {
	// Site-wide administration has no non-manageable share target, so TARGET_FORBIDDEN
	// remains a reserved code (rejected default) and is never returned today.
	result := rejected(c, "TARGET_FORBIDDEN")
	var revoked *time.Time
	e := tx.QueryRow(ctx, `SELECT revoked_at FROM product_file_shares WHERE share_id=$1 FOR UPDATE`, c.TargetID).Scan(&revoked)
	if errors.Is(e, pgx.ErrNoRows) {
		result.Code = "SHARE_NOT_FOUND"
		return result, nil
	}
	if e != nil {
		return result, e
	}
	if revoked != nil {
		result.Code = "SHARE_ALREADY_REVOKED"
		return result, nil
	}
	// Database time, COALESCE keeps the first revoke authoritative and idempotent.
	if _, e = tx.Exec(ctx, `UPDATE product_file_shares SET revoked_at=COALESCE(revoked_at,clock_timestamp()) WHERE share_id=$1`, c.TargetID); e != nil {
		return result, e
	}
	result.Status = "SUCCEEDED"
	result.Code = "SHARE_REVOKED"
	return result, nil
}

// ApplyShare revokes any site-local share by id. Grants remain durable history
// but can never bypass the revoked parent; no hash is read or returned.
func (r *AdminOperationsRepository) ApplyShare(ctx context.Context, a adminops.Actor, c adminops.Command) (adminops.Result, error) {
	if !c.Valid() || c.Action != adminops.RevokeShare {
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
	if result, e = r.revokeShare(ctx, tx, c); e != nil {
		return adminops.Result{}, e
	}
	if r.beforeUserCommit != nil {
		if e := r.beforeUserCommit(ctx, tx); e != nil {
			return adminops.Result{}, e
		}
	}
	return appendAdminResult(ctx, tx, a, result)
}
