package postgres

import (
	"context"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

// AttachObservationSession is a CAS metadata transition for an explicit operator
// transaction. It preserves Binding/root/mount identities and never changes a
// Job, Manifest, Copy or an old Connection credential. The caller must create
// the new session Connection in the same transaction and hold operator fencing.
func (r *StorageRepository) AttachObservationSession(ctx context.Context, expected storage.Binding, newConnection storage.ConnectionID, scope string) error {
	mount, e := storage.NormalizeMountPath(expected.OpenListMountPath)
	if e != nil || mount != expected.OpenListMountPath || expected.ID == "" || expected.ConnectionID == "" || expected.IndexCoreRootID == "" || expected.Status != storage.BindingStatusDisabled || expected.ProviderScope != nil || newConnection == "" || newConnection == expected.ConnectionID || storage.ValidateProviderScope(&scope) != nil {
		return storage.ErrInvalidArgument
	}
	result, e := r.db.Exec(ctx, `UPDATE storage_bindings b
SET storage_connection_id=$2,provider_scope=$3,status='ACTIVE',updated_at=clock_timestamp()
FROM storage_connections old,storage_connections fresh
WHERE b.storage_binding_id=$1 AND b.storage_connection_id=$4
AND b.indexcore_root_id=$5 AND b.openlist_mount_path=$6
AND b.status='DISABLED' AND b.provider_scope IS NULL
AND old.storage_connection_id=$4 AND old.status='DISABLED'
AND old.provider_type='115-observation-only' AND old.credential_ref IS NULL
AND fresh.storage_connection_id=$2 AND fresh.status='ACTIVE'
AND fresh.provider_type='115' AND fresh.credential_ref IS NOT NULL
AND NOT EXISTS(SELECT 1 FROM acquisition_manifests m WHERE m.target_storage_binding_id=b.storage_binding_id)`, string(expected.ID), string(newConnection), scope, string(expected.ConnectionID), expected.IndexCoreRootID, mount)
	if e != nil {
		return wrapStorageWriteError("attach observation session", e)
	}
	if result.RowsAffected() != 1 {
		return storage.ErrConflict
	}
	return nil
}
