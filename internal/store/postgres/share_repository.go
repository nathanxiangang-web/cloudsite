package postgres

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/fileshare"
	"github.com/nathanxiangang-web/panta/internal/storage"
	"time"
)

type shareDB interface {
	authDB
	Query(context.Context, string, ...any) (pgx.Rows, error)
}
type ShareRepository struct{ db shareDB }

func NewShareRepository(db shareDB) (*ShareRepository, error) {
	if db == nil {
		return nil, fileshare.ErrUnavailable
	}
	return &ShareRepository{db}, nil
}
func shareError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return fileshare.ErrDenied
	}
	if err != nil {
		return fileshare.ErrUnavailable
	}
	return nil
}

const shareColumns = `s.share_id::text,s.owner_id::text,s.binding_id::text,s.resource_id,s.canonical_path,s.file_name,s.size_bytes,s.created_at,s.expires_at,(s.revoked_at IS NULL AND s.expires_at>clock_timestamp()),s.code_hash,s.token_hash`

func scanShare(row pgx.Row) (fileshare.Record, error) {
	var r fileshare.Record
	var owner, binding string
	err := row.Scan(&r.ID, &owner, &binding, &r.Target.ResourceID, &r.Target.Path, &r.Name, &r.Size, &r.CreatedAt, &r.ExpiresAt, &r.Active, &r.CodeHash, &r.TokenHash)
	r.Owner = auth.UserID(owner)
	r.Target.BindingID = storage.BindingID(binding)
	r.Protected = len(r.CodeHash) > 0
	return r, shareError(err)
}
func (r *ShareRepository) Create(ctx context.Context, s fileshare.Record, ttl time.Duration) (fileshare.Record, error) {
	if len(s.TokenHash) != 32 || ttl < 5*time.Minute || ttl > 7*24*time.Hour {
		return fileshare.Record{}, fileshare.ErrInvalid
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fileshare.Record{}, fileshare.ErrUnavailable
	}
	defer tx.Rollback(context.Background())
	var enabled bool
	if err = tx.QueryRow(ctx, `SELECT enabled FROM product_users WHERE user_id=$1 FOR UPDATE`, string(s.Owner)).Scan(&enabled); err != nil || !enabled {
		return fileshare.Record{}, fileshare.ErrDenied
	}
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM product_file_shares WHERE owner_id=$1 AND revoked_at IS NULL AND expires_at>clock_timestamp()`, string(s.Owner)).Scan(&count); err != nil {
		return fileshare.Record{}, fileshare.ErrUnavailable
	}
	if count >= 100 {
		return fileshare.Record{}, fileshare.ErrLimit
	}
	err = tx.QueryRow(ctx, `WITH stamp AS (SELECT clock_timestamp() AS at) INSERT INTO product_file_shares(share_id,owner_id,binding_id,resource_id,canonical_path,file_name,size_bytes,token_hash,code_hash,created_at,expires_at) SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,at,at+($10*interval '1 second') FROM stamp RETURNING created_at,expires_at`, s.ID, string(s.Owner), string(s.Target.BindingID), s.Target.ResourceID, s.Target.Path, s.Name, s.Size, s.TokenHash, s.CodeHash, ttl.Seconds()).Scan(&s.CreatedAt, &s.ExpiresAt)
	if err != nil {
		return fileshare.Record{}, fileshare.ErrUnavailable
	}
	if tx.Commit(ctx) != nil {
		return fileshare.Record{}, fileshare.ErrUnavailable
	}
	s.Active = true
	return s, nil
}
func (r *ShareRepository) List(ctx context.Context, owner auth.UserID) ([]fileshare.Record, error) {
	rows, err := r.db.Query(ctx, `SELECT `+shareColumns+` FROM product_file_shares s WHERE s.owner_id=$1 ORDER BY s.created_at DESC,s.share_id LIMIT 100`, string(owner))
	if err != nil {
		return nil, fileshare.ErrUnavailable
	}
	defer rows.Close()
	result := []fileshare.Record{}
	for rows.Next() {
		item, err := scanShare(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	if rows.Err() != nil {
		return nil, fileshare.ErrUnavailable
	}
	return result, nil
}
func (r *ShareRepository) Revoke(ctx context.Context, owner auth.UserID, id string) error {
	tag, err := r.db.Exec(ctx, `UPDATE product_file_shares SET revoked_at=COALESCE(revoked_at,clock_timestamp()) WHERE share_id=$1 AND owner_id=$2`, id, string(owner))
	if err != nil {
		return fileshare.ErrUnavailable
	}
	if tag.RowsAffected() != 1 {
		return fileshare.ErrDenied
	}
	return nil
}
func (r *ShareRepository) Secret(ctx context.Context, hash []byte) (fileshare.Record, error) {
	return scanShare(r.db.QueryRow(ctx, `SELECT `+shareColumns+` FROM product_file_shares s JOIN product_users u ON u.user_id=s.owner_id WHERE s.token_hash=$1 AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp() AND u.enabled`, hash))
}
func (r *ShareRepository) IssueGrant(ctx context.Context, id string, hash []byte) error {
	if len(hash) != 32 {
		return fileshare.ErrInvalid
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fileshare.ErrUnavailable
	}
	defer tx.Rollback(context.Background())
	var exists string
	err = tx.QueryRow(ctx, `SELECT s.share_id::text FROM product_file_shares s JOIN product_users u ON u.user_id=s.owner_id WHERE s.share_id=$1 AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp() AND u.enabled FOR UPDATE OF s`, id).Scan(&exists)
	if err != nil {
		return shareError(err)
	}
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM product_file_share_grants WHERE share_id=$1 AND expires_at>clock_timestamp()`, id).Scan(&count); err != nil {
		return fileshare.ErrUnavailable
	}
	if count >= 100 {
		return fileshare.ErrLimit
	}
	tag, err := tx.Exec(ctx, `WITH stamp AS(SELECT clock_timestamp() AS at) INSERT INTO product_file_share_grants(grant_hash,share_id,created_at,expires_at) SELECT $1,s.share_id,at,s.expires_at FROM product_file_shares s,stamp WHERE s.share_id=$2 AND s.revoked_at IS NULL AND s.expires_at>at`, hash, id)
	if err != nil {
		return fileshare.ErrUnavailable
	}
	if tag.RowsAffected() != 1 {
		return fileshare.ErrDenied
	}
	return shareError(tx.Commit(ctx))
}
func (r *ShareRepository) Granted(ctx context.Context, id string, hash []byte) (fileshare.Record, auth.Principal, error) {
	record, err := scanShare(r.db.QueryRow(ctx, `SELECT `+shareColumns+` FROM product_file_shares s JOIN product_file_share_grants g ON g.share_id=s.share_id JOIN product_users u ON u.user_id=s.owner_id WHERE s.share_id=$1 AND g.grant_hash=$2 AND g.expires_at>clock_timestamp() AND s.expires_at>clock_timestamp() AND s.revoked_at IS NULL AND u.enabled`, id, hash))
	if err != nil {
		return fileshare.Record{}, auth.Principal{}, err
	}
	var p auth.Principal
	p.UserID = record.Owner
	err = r.db.QueryRow(ctx, `SELECT login,role FROM product_users WHERE user_id=$1 AND enabled`, string(record.Owner)).Scan(&p.Login, &p.Role)
	if err != nil || !p.Valid() {
		return fileshare.Record{}, auth.Principal{}, fileshare.ErrDenied
	}
	return record, p, nil
}
