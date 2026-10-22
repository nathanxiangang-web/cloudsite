package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/nathanxiangang-web/panta/internal/adminops"
	"golang.org/x/crypto/bcrypt"
)

// changeUser runs inside the authorized action transaction. The target account
// lock also serializes website session creation, product admission and shares.
func (r *AdminOperationsRepository) changeUser(ctx context.Context, tx pgx.Tx, a adminops.Actor, c adminops.Command) (adminops.Result, error) {
	result := rejected(c, "TARGET_FORBIDDEN")
	var u adminops.User
	var deleted *time.Time
	var hash []byte
	e := tx.QueryRow(ctx, `SELECT user_id::text,login,enabled,admin_version,deleted_at,password_hash FROM product_users WHERE user_id=$1 AND role='USER' AND user_id<>$2 FOR UPDATE`, c.TargetID, string(a.Principal.UserID)).Scan(&u.UserID, &u.Login, &u.Enabled, &u.Version, &deleted, &hash)
	defer clear(hash)
	if errors.Is(e, pgx.ErrNoRows) {
		return result, nil
	}
	if e != nil {
		return result, e
	}
	if u.Version != c.ExpectedVersion {
		result.Code = "VERSION_CONFLICT"
		return result, nil
	}
	if (deleted != nil) != (c.Action == adminops.RestoreUser) {
		result.Code = "USER_STATE_CONFLICT"
		return result, nil
	}
	code := "USER_STATE_CHANGED"
	var newHash []byte
	defer func() { clear(newHash) }()
	switch c.Action {
	case adminops.SetEnabled:
		u.Enabled = c.Enabled
	case adminops.RenameUser:
		u.Login = c.Login
		code = "USER_RENAMED"
	case adminops.ResetPassword, adminops.RestoreUser:
		if c.Action == adminops.RestoreUser && bcrypt.CompareHashAndPassword(hash, []byte(c.Password)) == nil {
			result.Code = "PASSWORD_UNCHANGED"
			return result, nil
		}
		newHash, e = bcrypt.GenerateFromPassword([]byte(c.Password), bcrypt.DefaultCost)
		if e != nil {
			return result, e
		}
		code = "USER_PASSWORD_RESET"
		if c.Action == adminops.RestoreUser {
			u.Enabled = false
			code = "USER_RESTORED"
		}
	case adminops.DeleteUser:
		// No implicit cancellation/retry of active or uncertain provider side effects.
		// Concurrent product submission holds FOR SHARE on this same account first.
		var pending bool
		e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM acquisition_manifests WHERE user_id=$1 AND state NOT IN ('READY','FAILED','CANCELED'))`, u.UserID).Scan(&pending)
		if e != nil {
			return result, e
		}
		if pending {
			result.Code = "ACTIVE_DOWNLOADS"
			return result, nil
		}
		u.Enabled = false
		code = "USER_DELETED"
	default:
		return result, adminops.ErrInvalid
	}
	// Savepoint preserves an audited duplicate-login rejection without aborting
	// the outer intent/outcome transaction or leaking constraint details.
	save, e := tx.Begin(ctx)
	if e != nil {
		return result, e
	}
	defer save.Rollback(context.Background())
	e = save.QueryRow(ctx, `UPDATE product_users SET login=$2,enabled=$3,
 password_hash=COALESCE($4::bytea,password_hash),admin_version=admin_version+1,
 deleted_at=CASE WHEN $5='DELETE_USER' THEN clock_timestamp() WHEN $5='RESTORE_USER' THEN NULL ELSE deleted_at END,
 deleted_by=CASE WHEN $5='DELETE_USER' THEN $6::uuid WHEN $5='RESTORE_USER' THEN NULL ELSE deleted_by END,
 updated_at=clock_timestamp() WHERE user_id=$1 AND admin_version=$7 RETURNING admin_version`, u.UserID, u.Login, u.Enabled, newHash, c.Action, string(a.Principal.UserID), c.ExpectedVersion).Scan(&u.Version)
	if e != nil {
		_ = save.Rollback(ctx)
		var pe *pgconn.PgError
		if errors.As(e, &pe) && pe.Code == "23505" {
			result.Code = "LOGIN_CONFLICT"
			return result, nil
		}
		return result, e
	}
	if e = save.Commit(ctx); e != nil {
		return result, e
	}
	if _, e = tx.Exec(ctx, `UPDATE product_sessions SET revoked_at=clock_timestamp() WHERE user_id=$1 AND revoked_at IS NULL`, u.UserID); e != nil {
		return result, e
	}
	if c.Action == adminops.DeleteUser {
		// Grants remain durable history but can never bypass the revoked parent.
		if _, e = tx.Exec(ctx, `UPDATE product_file_shares SET revoked_at=clock_timestamp() WHERE owner_id=$1 AND revoked_at IS NULL`, u.UserID); e != nil {
			return result, e
		}
	}
	u.Deleted = c.Action == adminops.DeleteUser
	result.Status = "SUCCEEDED"
	result.Code = code
	result.User = &u
	return result, nil
}
