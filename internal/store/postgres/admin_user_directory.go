package postgres

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/nathanxiangang-web/panta/internal/adminread"
)

const directoryColumns = `u.user_id::text,u.login,u.role,u.enabled,u.created_at,u.updated_at,u.admin_version,u.deleted_at,u.deleted_by::text`

func scanDirectory(row pgx.Row, u *adminread.User) error {
	return row.Scan(&u.UserID, &u.Login, &u.Role, &u.Enabled, &u.CreatedAt, &u.UpdatedAt, &u.Version, &u.DeletedAt, &u.DeletedBy)
}

// Filter count deliberately excludes the cursor: it is a real total, never a
// guess from one page. Both queries observe the same read-only MVCC snapshot.
func (r *AdminReadRepository) SearchUsers(ctx context.Context, f adminread.UserFilter) (adminread.UserDirectory, error) {
	if !f.Valid() {
		return adminread.UserDirectory{}, adminread.ErrInvalid
	}
	tx, e := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		return adminread.UserDirectory{}, adminReadError(e)
	}
	defer tx.Rollback(context.Background())
	filter := ` FROM product_users u WHERE ($1='' OR CASE WHEN $3::uuid IS NOT NULL THEN u.user_id=$3::uuid ELSE position($1 in u.login)>0 END)
 AND ($2='' OR $2='ALL' OR $2='ENABLED' AND u.enabled AND u.deleted_at IS NULL OR $2='DISABLED' AND NOT u.enabled AND u.deleted_at IS NULL OR $2='DELETED' AND u.deleted_at IS NOT NULL)`
	var exact any
	if adminread.ValidID(f.Search) {
		exact = f.Search
	}
	page := adminread.UserDirectory{Items: []adminread.User{}, Scope: "FILTERED_PRODUCT_USERS"}
	if e = tx.QueryRow(ctx, `SELECT count(*)`+filter, f.Search, f.Status, exact).Scan(&page.Total); e != nil {
		return page, adminReadError(e)
	}
	rows, e := tx.Query(ctx, `SELECT `+directoryColumns+filter+` AND ($4::uuid IS NULL OR u.user_id>$4::uuid) ORDER BY u.user_id LIMIT $5`, f.Search, f.Status, exact, adminNullable(f.Cursor), f.Limit+1)
	if e != nil {
		return page, adminReadError(e)
	}
	for rows.Next() {
		var u adminread.User
		if e = scanDirectory(rows, &u); e != nil {
			break
		}
		page.Items = append(page.Items, u)
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		return page, adminReadError(e)
	}
	if len(page.Items) > f.Limit {
		page.Items = page.Items[:f.Limit]
		page.NextCursor = page.Items[f.Limit-1].UserID
	}
	if e = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&page.ReadAt); e != nil {
		return page, adminReadError(e)
	}
	if e = tx.Commit(ctx); e != nil {
		return page, adminReadError(e)
	}
	return page, nil
}
func (r *AdminReadRepository) UserDetail(ctx context.Context, id string) (adminread.UserDetail, error) {
	var u adminread.UserDetail
	if !adminread.ValidID(id) {
		return u, adminread.ErrInvalid
	}
	tx, e := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		return u, adminReadError(e)
	}
	defer tx.Rollback(context.Background())
	e = scanDirectory(tx.QueryRow(ctx, `SELECT `+directoryColumns+` FROM product_users u WHERE u.user_id=$1`, id), &u.User)
	if errors.Is(e, pgx.ErrNoRows) {
		return u, adminread.ErrNotFound
	}
	if e != nil {
		return u, adminReadError(e)
	}
	e = tx.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE state NOT IN ('READY','FAILED','CANCELED')) FROM acquisition_manifests WHERE user_id=$1`, id).Scan(&u.DownloadCount, &u.ActiveDownloads)
	if e != nil {
		return u, adminReadError(e)
	}
	if e = tx.Commit(ctx); e != nil {
		return u, adminReadError(e)
	}
	return u, nil
}
