package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/nathanxiangang-web/panta/internal/libraryview"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

type LibrarySnapshotRepository struct{ db catalogDB }

func NewLibrarySnapshotRepository(db catalogDB) *LibrarySnapshotRepository {
	return &LibrarySnapshotRepository{db}
}

// CheckLibraryPermissions runs before listeners/background source reads. SHARE
// row locks require UPDATE on at least one Binding column, even without DML.
func (r *LibrarySnapshotRepository) CheckLibraryPermissions(ctx context.Context) error {
	var ok bool
	err := r.db.QueryRow(ctx, `SELECT has_schema_privilege(current_user,'public','USAGE')
 AND has_table_privilege(current_user,'public.product_library_snapshots','SELECT')
 AND has_table_privilege(current_user,'public.product_library_snapshots','INSERT')
 AND has_table_privilege(current_user,'public.product_library_snapshots','UPDATE')
 AND has_table_privilege(current_user,'public.storage_bindings','SELECT')
 AND has_any_column_privilege(current_user,'public.storage_bindings','UPDATE')`).Scan(&ok)
	if err != nil || !ok {
		return libraryview.ErrUnavailable
	}
	return nil
}
func (r *LibrarySnapshotRepository) LoadLibrarySnapshot(ctx context.Context, id storage.BindingID, root string) (libraryview.Snapshot, bool, error) {
	s := libraryview.Snapshot{BindingID: id, RootID: root}
	var raw []byte
	err := r.db.QueryRow(ctx, `SELECT journal_seq,revision,updated_at,resources,checkpoint FROM product_library_snapshots WHERE binding_id=$1 AND root_id=$2`, string(id), root).Scan(&s.Sequence, &s.Revision, &s.UpdatedAt, &raw, &s.Checkpoint)
	if errors.Is(err, pgx.ErrNoRows) {
		return s, false, nil
	}
	if err != nil {
		return s, false, err
	}
	if len(raw) > 64<<20 {
		return s, false, libraryview.ErrUnavailable
	}
	if err = json.Unmarshal(raw, &s.Resources); err != nil {
		return s, false, libraryview.ErrUnavailable
	}
	return s, true, nil
}
func (r *LibrarySnapshotRepository) SaveLibrarySnapshot(ctx context.Context, s libraryview.Snapshot) error {
	if err := libraryview.ValidateSnapshot(s); err != nil {
		return err
	}
	raw, err := json.Marshal(s.Resources)
	if err != nil || len(raw) > 64<<20 {
		return libraryview.ErrUnavailable
	}
	tag, err := r.db.Exec(ctx, `INSERT INTO product_library_snapshots(binding_id,root_id,journal_seq,revision,updated_at,resources,checkpoint)
 SELECT $1,$2,$3,$4,$5,$6,$7 FROM storage_bindings WHERE storage_binding_id=$1 AND indexcore_root_id=$2 AND status='ACTIVE' FOR SHARE
 ON CONFLICT(binding_id) DO UPDATE SET root_id=EXCLUDED.root_id,journal_seq=EXCLUDED.journal_seq,revision=EXCLUDED.revision,updated_at=EXCLUDED.updated_at,resources=EXCLUDED.resources,checkpoint=EXCLUDED.checkpoint
 WHERE product_library_snapshots.root_id<>EXCLUDED.root_id OR product_library_snapshots.journal_seq<=EXCLUDED.journal_seq OR (product_library_snapshots.journal_seq=$8 AND product_library_snapshots.revision=$9)`, string(s.BindingID), s.RootID, s.Sequence, s.Revision, s.UpdatedAt, raw, s.Checkpoint, s.PreviousSequence, s.PreviousRevision)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return libraryview.ErrUnavailable
	}
	return nil
}
