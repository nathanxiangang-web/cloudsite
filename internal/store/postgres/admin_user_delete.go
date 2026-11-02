package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/nathanxiangang-web/panta/internal/adminops"
)

// deleteAccount runs after the actor, account/version and terminal-download
// guards in changeUser. The caller owns the action/outcome transaction. Historical
// Manifest/Job/provider rows and ADMIN audit results are never deleted here.
func deleteAccount(ctx context.Context, tx pgx.Tx, c adminops.Command, u adminops.User) (adminops.Result, error) {
	result := rejected(c, "TARGET_FORBIDDEN")
	save, err := tx.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer save.Rollback(context.Background())
	for _, statement := range []string{
		`DELETE FROM product_sessions WHERE user_id=$1`,
		`DELETE FROM product_file_share_grants WHERE share_id IN (SELECT share_id FROM product_file_shares WHERE owner_id=$1)`,
		`DELETE FROM product_file_shares WHERE owner_id=$1`,
		`DELETE FROM product_operations WHERE user_id=$1`,
		`DELETE FROM product_operation_quotas WHERE user_id=$1`,
		`DELETE FROM product_users WHERE user_id=$1`,
	} {
		if _, err = save.Exec(ctx, statement, u.UserID); err != nil {
			_ = save.Rollback(ctx)
			var constraint *pgconn.PgError
			if errors.As(err, &constraint) && constraint.Code == "23503" {
				// Unexpected references must reject atomically, not erase some
				// account data and leave an unconfirmed partial deletion.
				return result, nil
			}
			return result, err
		}
	}
	if err = save.Commit(ctx); err != nil {
		return result, err
	}
	u.Enabled = false
	u.Deleted = true
	u.Version++
	result.Status = "SUCCEEDED"
	result.Code = "USER_DELETED"
	result.User = &u // Receipt only; this is not a remaining/restorable account.
	return result, nil
}
