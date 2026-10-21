package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/nathanxiangang-web/panta/internal/adminobserve"
)

type AdminObserveRepository struct {
	db        adminReadDB
	afterRead func(context.Context) error
}

func NewAdminObserveRepository(db adminReadDB) (*AdminObserveRepository, error) {
	if adminobserve.Missing(db) {
		return nil, adminobserve.ErrUnavailable
	}
	return &AdminObserveRepository{db: db}, nil
}

const observeColumns = `configured.id::text,c.storage_connection_id::text,
 CASE lower(c.provider_type) WHEN '115' THEN '115' WHEN '115drive' THEN '115' WHEN 'openlist' THEN 'openlist' WHEN 'rclone' THEN 'rclone' WHEN 'local' THEN 'local' WHEN 'mock' THEN 'mock' ELSE 'OTHER' END,
 b.status,c.status,b.updated_at,c.updated_at,(b.indexcore_root_id<>'')`
const observeJoin = ` FROM unnest($1::uuid[]) configured(id)
 LEFT JOIN storage_bindings b ON b.storage_binding_id=configured.id
 LEFT JOIN storage_connections c ON c.storage_connection_id=b.storage_connection_id `

func (r *AdminObserveRepository) read(ctx context.Context, ids []string, n int, cursor, id string) ([]adminobserve.Mapping, error) {
	if len(ids) < 1 || len(ids) > 64 || n < 1 || n > 51 || cursor != "" && !adminobserve.ValidID(cursor) || id != "" && !adminobserve.ValidID(id) {
		return nil, adminobserve.ErrInvalid
	}
	seen := map[string]bool{}
	found := false
	for _, v := range ids {
		if !adminobserve.ValidID(v) || seen[v] {
			return nil, adminobserve.ErrInvalid
		}
		seen[v] = true
		if v == id {
			found = true
		}
	}
	if id != "" && !found {
		return nil, adminobserve.ErrNotFound
	}
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, observeError(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	sql := `SELECT ` + observeColumns
	if id != "" {
		sql += `,b.indexcore_root_id`
	}
	sql += observeJoin + `WHERE ($2::uuid IS NULL OR configured.id>$2::uuid) AND ($3::uuid IS NULL OR configured.id=$3::uuid) ORDER BY configured.id ASC LIMIT $4`
	rows, err := tx.Query(ctx, sql, ids, adminNullable(cursor), adminNullable(id), n)
	if err != nil {
		return nil, observeError(err)
	}
	result := make([]adminobserve.Mapping, 0, n)
	for rows.Next() {
		var m adminobserve.Mapping
		targets := []any{&m.BindingID, &m.ConnectionID, &m.ProviderType, &m.BindingStatus, &m.ConnectionStatus, &m.BindingUpdatedAt, &m.ConnectionUpdatedAt, &m.RootConfigured}
		if id != "" {
			targets = append(targets, &m.RootID)
		}
		if err = rows.Scan(targets...); err != nil {
			rows.Close()
			return nil, observeError(err)
		}
		result = append(result, m)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, observeError(err)
	}
	if r.afterRead != nil {
		if err = r.afterRead(ctx); err != nil {
			return nil, observeError(err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, observeError(err)
	}
	return result, nil
}
func observeError(err error) error {
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	return adminobserve.ErrUnavailable
}
func (r *AdminObserveRepository) List(ctx context.Context, ids []string, n int, cursor string) ([]adminobserve.Storage, error) {
	rows, err := r.read(ctx, ids, n, cursor, "")
	if err != nil {
		return nil, err
	}
	result := make([]adminobserve.Storage, 0, len(rows))
	for _, m := range rows {
		result = append(result, m.Storage)
	}
	return result, nil
}
func (r *AdminObserveRepository) Get(ctx context.Context, ids []string, id string) (adminobserve.Mapping, error) {
	if !adminobserve.ValidID(id) {
		return adminobserve.Mapping{}, adminobserve.ErrInvalid
	}
	rows, err := r.read(ctx, ids, 1, "", id)
	if err != nil {
		return adminobserve.Mapping{}, err
	}
	if len(rows) != 1 {
		return adminobserve.Mapping{}, adminobserve.ErrUnavailable
	}
	return rows[0], nil
}
