package postgres

import (
	"context"
	"errors"
	"reflect"

	"github.com/jackc/pgx/v5"
	"github.com/nathanxiangang-web/panta/internal/acquisition"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/product"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

type productReadDB interface {
	BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error)
}
type ProductReadRepository struct {
	db productReadDB
	// Test-only hook proves concurrent paired writes cannot mix read snapshots.
	afterManifestRead func(context.Context) error
}

var _ product.TaskReader = (*ProductReadRepository)(nil)

func NewProductReadRepository(db productReadDB) (*ProductReadRepository, error) {
	if db == nil || (reflect.ValueOf(db).Kind() == reflect.Pointer && reflect.ValueOf(db).IsNil()) {
		return nil, product.ErrUnavailable
	}
	return &ProductReadRepository{db: db}, nil
}
func productBindings(ids []storage.BindingID) ([]string, error) {
	if len(ids) == 0 {
		return nil, product.ErrInvalid
	}
	values := make([]string, len(ids))
	for i, id := range ids {
		if !product.ValidID(string(id)) {
			return nil, product.ErrInvalid
		}
		values[i] = string(id)
	}
	return values, nil
}
func productReadError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return product.ErrUnavailable
}
func readProductJobs(ctx context.Context, tx pgx.Tx, manifests []acquisition.Manifest) ([]product.TaskRecord, error) {
	records := make([]product.TaskRecord, 0, len(manifests))
	for _, manifest := range manifests {
		record := product.TaskRecord{Manifest: manifest}
		if manifest.JobID != nil {
			job, err := getOneJob(ctx, tx, "SELECT "+jobColumns+" FROM jobs WHERE job_id=$1", string(*manifest.JobID))
			if err != nil {
				return nil, productReadError(err)
			}
			record.Job = &job
		}
		records = append(records, record)
	}
	return records, nil
}
func (repo *ProductReadRepository) ListTasks(ctx context.Context, user auth.UserID, bindings []storage.BindingID, limit int, cursor string) ([]product.TaskRecord, error) {
	if !product.ValidID(string(user)) || limit < 1 || limit > 51 || cursor != "" && !product.ValidID(cursor) {
		return nil, product.ErrInvalid
	}
	ids, err := productBindings(bindings)
	if err != nil {
		return nil, err
	}
	tx, err := repo.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, productReadError(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	// Both the Manifest and its Job come from this read-only repeatable snapshot.
	// Filter identity and the operator namespace before materializing any row.
	var after any
	if cursor != "" {
		after = cursor
	}
	rows, err := tx.Query(ctx, `SELECT `+acquisitionManifestColumns+` FROM acquisition_manifests WHERE user_id=$1 AND target_storage_binding_id=ANY($2::uuid[]) AND ($3::uuid IS NULL OR manifest_id>$3::uuid) ORDER BY manifest_id ASC LIMIT $4`, string(user), ids, after, limit)
	if err != nil {
		return nil, productReadError(err)
	}
	manifests := make([]acquisition.Manifest, 0, limit)
	for rows.Next() {
		manifest, err := scanAcquisitionManifest(rows)
		if err != nil {
			rows.Close()
			return nil, productReadError(err)
		}
		manifests = append(manifests, manifest)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, productReadError(err)
	}
	if repo.afterManifestRead != nil {
		if err = repo.afterManifestRead(ctx); err != nil {
			return nil, productReadError(err)
		}
	}
	records, err := readProductJobs(ctx, tx, manifests)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, productReadError(err)
	}
	return records, nil
}
func (repo *ProductReadRepository) GetTask(ctx context.Context, user auth.UserID, bindings []storage.BindingID, id acquisition.ManifestID) (product.TaskRecord, error) {
	if !product.ValidID(string(user)) || !product.ValidID(string(id)) {
		return product.TaskRecord{}, product.ErrInvalid
	}
	ids, err := productBindings(bindings)
	if err != nil {
		return product.TaskRecord{}, err
	}
	tx, err := repo.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return product.TaskRecord{}, productReadError(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	manifest, err := scanAcquisitionManifest(tx.QueryRow(ctx, `SELECT `+acquisitionManifestColumns+` FROM acquisition_manifests WHERE manifest_id=$1 AND user_id=$2 AND target_storage_binding_id=ANY($3::uuid[])`, string(id), string(user), ids))
	if errors.Is(err, pgx.ErrNoRows) {
		return product.TaskRecord{}, product.ErrNotFound
	}
	if err != nil {
		return product.TaskRecord{}, productReadError(err)
	}
	if repo.afterManifestRead != nil {
		if err = repo.afterManifestRead(ctx); err != nil {
			return product.TaskRecord{}, productReadError(err)
		}
	}
	records, err := readProductJobs(ctx, tx, []acquisition.Manifest{manifest})
	if err != nil {
		return product.TaskRecord{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return product.TaskRecord{}, productReadError(err)
	}
	return records[0], nil
}
