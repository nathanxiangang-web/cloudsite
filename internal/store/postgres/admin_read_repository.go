package postgres

import (
	"context"
	"errors"
	"reflect"

	"github.com/jackc/pgx/v5"
	"github.com/nathanxiangang-web/panta/internal/adminread"
)

// AdminReadRepository is a narrow SQL projection adapter. It never scans raw
// auth.User, acquisition.Manifest or jobs.Job records into the read domain.
type adminReadDB interface {
	BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error)
}
type AdminReadRepository struct {
	db adminReadDB
	// Test hook runs after the one data query, before closing its read transaction.
	afterRead func(context.Context) error
}

var _ adminread.Reader = (*AdminReadRepository)(nil)

func NewAdminReadRepository(db adminReadDB) (*AdminReadRepository, error) {
	if db == nil || reflect.ValueOf(db).Kind() == reflect.Pointer && reflect.ValueOf(db).IsNil() {
		return nil, adminread.ErrUnavailable
	}
	return &AdminReadRepository{db: db}, nil
}
func adminReadError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return adminread.ErrUnavailable
}
func adminQuery(limit int, cursor string) bool {
	return limit >= 1 && limit <= 51 && (cursor == "" || adminread.ValidID(cursor))
}
func adminNullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}
func (r *AdminReadRepository) ListUsers(ctx context.Context, limit int, cursor string) ([]adminread.User, error) {
	if !adminQuery(limit, cursor) {
		return nil, adminread.ErrInvalid
	}
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, adminReadError(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	rows, err := tx.Query(ctx, `SELECT user_id::text,login,role,enabled,created_at,updated_at,admin_version FROM product_users WHERE ($1::uuid IS NULL OR user_id>$1::uuid) ORDER BY user_id ASC LIMIT $2`, adminNullable(cursor), limit)
	if err != nil {
		return nil, adminReadError(err)
	}
	result := make([]adminread.User, 0, limit)
	for rows.Next() {
		var item adminread.User
		if err = rows.Scan(&item.UserID, &item.Login, &item.Role, &item.Enabled, &item.CreatedAt, &item.UpdatedAt, &item.Version); err != nil {
			rows.Close()
			return nil, adminReadError(err)
		}
		result = append(result, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, adminReadError(err)
	}
	if r.afterRead != nil {
		if err = r.afterRead(ctx); err != nil {
			return nil, adminReadError(err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, adminReadError(err)
	}
	return result, nil
}
func (r *AdminReadRepository) ListDownloads(ctx context.Context, bindings []string, limit int, cursor, user string) ([]adminread.DownloadRecord, error) {
	if !adminQuery(limit, cursor) || user != "" && !adminread.ValidID(user) || len(bindings) < 1 || len(bindings) > 64 {
		return nil, adminread.ErrInvalid
	}
	for _, id := range bindings {
		if !adminread.ValidID(id) {
			return nil, adminread.ErrInvalid
		}
	}
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, adminReadError(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	// One bounded SELECT gives both sides of each pair the same MVCC snapshot.
	// Secret columns participate only in a boolean linkage check; none are returned.
	rows, err := tx.Query(ctx, `SELECT m.manifest_id::text,m.user_id::text,m.target_storage_binding_id::text,
 m.job_id::text,m.state,j.state,m.result_copy_id::text,
 COALESCE(j.claim_attempts,0),COALESCE(j.attempt_count,0),m.created_at,m.updated_at,
 CASE WHEN m.job_id IS NULL THEN m.state='PENDING' ELSE COALESCE(
 j.job_id=m.job_id AND j.job_type='ACQUISITION'
 AND j.idempotency_key='acquisition:'||m.manifest_id::text
 AND j.payload=jsonb_build_object('schema_version',1,'manifest_id',m.manifest_id::text)
 AND j.payload->>'schema_version'='1',false) END,
 COALESCE(j.max_attempts,0)
 FROM acquisition_manifests m LEFT JOIN jobs j ON j.job_id=m.job_id
 WHERE m.user_id IS NOT NULL AND m.target_storage_binding_id=ANY($1::uuid[])
 AND ($2::uuid IS NULL OR m.manifest_id>$2::uuid) AND ($3::uuid IS NULL OR m.user_id=$3::uuid)
 ORDER BY m.manifest_id ASC LIMIT $4`, bindings, adminNullable(cursor), adminNullable(user), limit)
	if err != nil {
		return nil, adminReadError(err)
	}
	result := make([]adminread.DownloadRecord, 0, limit)
	for rows.Next() {
		var item adminread.DownloadRecord
		if err = rows.Scan(&item.ManifestID, &item.OwnerUserID, &item.BindingID, &item.JobID, &item.ManifestState, &item.JobState, &item.ResultCopyID, &item.ClaimAttempts, &item.FailureCount, &item.CreatedAt, &item.UpdatedAt, &item.LinkValid, &item.MaxAttempts); err != nil {
			rows.Close()
			return nil, adminReadError(err)
		}
		result = append(result, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, adminReadError(err)
	}
	if r.afterRead != nil {
		if err = r.afterRead(ctx); err != nil {
			return nil, adminReadError(err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, adminReadError(err)
	}
	return result, nil
}
