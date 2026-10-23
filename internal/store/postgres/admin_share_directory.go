package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/nathanxiangang-web/panta/internal/adminread"
)

const shareDirectoryColumns = `s.share_id::text,s.owner_id::text,s.binding_id::text,s.canonical_path,s.file_name,s.size_bytes,s.created_at,s.expires_at,(s.revoked_at IS NULL AND s.expires_at>now())`

// SearchShares is a keyset page over product_file_shares plus a real DB count.
// now() is the transaction start in Repeatable Read, so the count and the page
// share one expiry snapshot; hashes never enter the SELECT list.
func (r *AdminReadRepository) SearchShares(ctx context.Context, f adminread.ShareFilter) (adminread.ShareDirectory, error) {
	if !f.Valid() {
		return adminread.ShareDirectory{}, adminread.ErrInvalid
	}
	tx, e := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		return adminread.ShareDirectory{}, adminReadError(e)
	}
	defer tx.Rollback(context.Background())
	filter := ` FROM product_file_shares s WHERE ($1::uuid IS NULL OR s.owner_id=$1::uuid)
 AND ($2='' OR $2='ALL'
  OR $2='ACTIVE' AND s.revoked_at IS NULL AND s.expires_at>now()
  OR $2='REVOKED' AND s.revoked_at IS NOT NULL
  OR $2='EXPIRED' AND s.revoked_at IS NULL AND s.expires_at<=now())`
	page := adminread.ShareDirectory{Items: []adminread.Share{}, Scope: "FILTERED_PRODUCT_SHARES"}
	if e = tx.QueryRow(ctx, `SELECT count(*)`+filter, adminNullable(f.Owner), f.Status).Scan(&page.Total); e != nil {
		return page, adminReadError(e)
	}
	rows, e := tx.Query(ctx, `SELECT `+shareDirectoryColumns+filter+` AND ($3::uuid IS NULL OR s.share_id>$3::uuid) ORDER BY s.share_id LIMIT $4`, adminNullable(f.Owner), f.Status, adminNullable(f.Cursor), f.Limit+1)
	if e != nil {
		return page, adminReadError(e)
	}
	for rows.Next() {
		var item adminread.Share
		if e = rows.Scan(&item.ShareID, &item.OwnerID, &item.BindingID, &item.CanonicalPath, &item.FileName, &item.SizeBytes, &item.CreatedAt, &item.ExpiresAt, &item.Active); e != nil {
			break
		}
		page.Items = append(page.Items, item)
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
		page.NextCursor = page.Items[f.Limit-1].ShareID
	}
	if e = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&page.ReadAt); e != nil {
		return page, adminReadError(e)
	}
	if e = tx.Commit(ctx); e != nil {
		return page, adminReadError(e)
	}
	return page, nil
}
