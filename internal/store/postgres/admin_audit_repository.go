package postgres

import (
	"context"
	"errors"
	"reflect"

	"github.com/jackc/pgx/v5"
	"github.com/nathanxiangang-web/panta/internal/adminaudit"
	"github.com/nathanxiangang-web/panta/internal/adminops"
	"github.com/nathanxiangang-web/panta/internal/adminread"
	"github.com/nathanxiangang-web/panta/internal/auth"
)

type adminAuditDB interface {
	BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}
type AdminAuditRepository struct {
	db        adminAuditDB
	afterRead func(context.Context, pgx.Tx) error
}

var _ adminaudit.Reader = (*AdminAuditRepository)(nil)

func NewAdminAuditRepository(db adminAuditDB) (*AdminAuditRepository, error) {
	if db == nil || reflect.ValueOf(db).Kind() == reflect.Pointer && reflect.ValueOf(db).IsNil() {
		return nil, adminaudit.ErrUnavailable
	}
	return &AdminAuditRepository{db: db}, nil
}

// No row locks/writes in the read-only transaction. Recheck from the live pool
// after the snapshot to reject revocation during a slow historical read, too.
func auditActor(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, a adminops.Actor) error {
	if a.Principal.Role != auth.RoleAdmin {
		return adminaudit.ErrForbidden
	}
	var valid bool
	e := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM product_users u JOIN product_sessions s USING(user_id)
 WHERE u.user_id=$1 AND s.token_hash=$2 AND u.login=$3 AND u.enabled AND u.role='ADMIN'
 AND to_jsonb(u)->>'deleted_at' IS NULL AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp())`, string(a.Principal.UserID), a.SessionDigest[:], a.Principal.Login).Scan(&valid)
	if e != nil {
		return adminaudit.ErrUnavailable
	}
	if !valid {
		return auth.ErrUnauthorized
	}
	return nil
}

// Only header identity, an enum code/state, timestamps and target classification
// cross this projection. Neither outcome nor any nested account/provider values
// are selected into Go. Unknown keys/type corruption are not healthy receipts.
const auditProjection = `WITH scoped AS (
 SELECT a.actor_id,a.request_id,a.action,a.target_id,a.created_at,r.recorded_at,r.request_id IS NOT NULL AS terminal,r.outcome AS o
 FROM product_admin_actions a LEFT JOIN product_admin_action_results r USING(actor_id,request_id)
 WHERE a.actor_id=$1
 AND ($3='ALL' OR a.action=$3)
 AND ($4::timestamptz IS NULL OR a.created_at>=$4) AND ($5::timestamptz IS NULL OR a.created_at<$5)
 AND ($7::uuid IS NULL OR a.request_id=$7)
 AND ($8::timestamptz IS NULL OR (a.created_at,a.request_id)<($8,$9::uuid))
 AND CASE a.action
 WHEN 'SET_COPY_PRESENTATION' THEN EXISTS(SELECT 1 FROM copies c JOIN storage_bindings b ON b.storage_binding_id=c.storage_binding_id AND b.indexcore_root_id=c.indexcore_root_id JOIN indexcore_projection_cursors pc ON pc.storage_binding_id=b.storage_binding_id WHERE c.copy_id=a.target_id AND c.storage_binding_id=ANY($2::uuid[]) AND pc.last_event_seq>0)
 WHEN 'PROBE_115' THEN EXISTS(SELECT 1 FROM storage_bindings b WHERE b.storage_binding_id=a.target_id AND b.storage_binding_id=ANY($2::uuid[]))
 WHEN 'REVOKE_SHARE' THEN EXISTS(SELECT 1 FROM product_file_shares s WHERE s.share_id=a.target_id AND s.binding_id=ANY($2::uuid[]))
 WHEN 'CANCEL_PENDING_MANIFEST' THEN EXISTS(SELECT 1 FROM acquisition_manifests m WHERE m.manifest_id=a.target_id AND m.user_id IS NOT NULL AND m.target_storage_binding_id=ANY($2::uuid[]))
 ELSE true END
), projected AS (
 SELECT actor_id,request_id,action,created_at,recorded_at,
 CASE WHEN NOT terminal THEN 'UNKNOWN' WHEN o->>'status' IN ('SUCCEEDED','REJECTED') THEN o->>'status' ELSE '' END AS status,
 CASE WHEN NOT terminal THEN 'RESERVED_NO_CONFIRMED_RESULT' WHEN o->>'code' IN (
 'USER_CREATED','USER_STATE_CHANGED','USER_RENAMED','USER_PASSWORD_RESET','USER_DELETED','USER_RESTORED','SHARE_REVOKED','PROBE_RECORDED','PENDING_CANCELED','COPY_PRESENTATION_SET',
 'CONFLICT','TARGET_FORBIDDEN','VERSION_CONFLICT','USER_STATE_CONFLICT','PASSWORD_UNCHANGED','ACTIVE_DOWNLOADS','DOWNLOAD_STATE_CONFLICT','LOGIN_CONFLICT',
 'SCOPE_REJECTED','NOT_CONNECTED','RATE_LIMITED','SHARE_NOT_FOUND','SHARE_ALREADY_REVOKED','MANIFEST_NOT_FOUND','OWNER_UNAVAILABLE','PENDING_CHANGED','PENDING_NOT_ISOLATED','MANIFEST_UNAVAILABLE','COPY_NOT_FOUND','COPY_UNAVAILABLE') THEN o->>'code' ELSE '' END AS code,
 CASE WHEN target_id IS NULL AND NOT (terminal AND o ? 'target_id') THEN 'NOT_RECORDED'
 WHEN action IN ('CREATE_USER','SET_USER_ENABLED','RENAME_USER','RESET_USER_PASSWORD','DELETE_USER','RESTORE_USER') AND NOT EXISTS(
 SELECT 1 FROM product_users u WHERE u.user_id=COALESCE(target_id,CASE WHEN o->>'target_id' ~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$' THEN (o->>'target_id')::uuid END)
 AND to_jsonb(u)->>'deleted_at' IS NULL) THEN 'DELETED_OR_MISSING' ELSE 'REDACTED' END AS target_status,
 CASE WHEN action='CANCEL_PENDING_MANIFEST' AND o->'manifest'->>'state' IN ('PENDING','ACTIVE','AWAITING_VISIBILITY','AWAITING_CANONICAL','READY','FAILED','CANCELED','RECOVERY_REQUIRED') THEN o->'manifest'->>'state' ELSE '' END AS manifest_state,
 CASE WHEN NOT terminal THEN true ELSE COALESCE(
 jsonb_typeof(o)='object' AND o->>'request_id'=request_id::text AND o->>'action'=action
 AND (target_id IS NULL OR o->>'target_id'=target_id::text)
 AND (NOT o ? 'target_id' OR jsonb_typeof(o->'target_id')='string' AND o->>'target_id' ~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$')
 AND jsonb_typeof(o->'status')='string' AND o->>'status' IN ('SUCCEEDED','REJECTED')
 AND jsonb_typeof(o->'code')='string'
 AND CASE WHEN jsonb_typeof(o->'recorded_at')='string' AND o->>'recorded_at' ~ '^\d{4}-\d{2}-\d{2}T' THEN (o->>'recorded_at')::timestamptz=recorded_at ELSE false END
 AND recorded_at>=created_at
 AND NOT EXISTS(SELECT 1 FROM jsonb_object_keys(o) k WHERE k NOT IN ('request_id','action','target_id','status','code','recorded_at','user','capacity','manifest','presentation_version'))
 AND (NOT o ? 'presentation_version' OR action='SET_COPY_PRESENTATION' AND o->>'status'='SUCCEEDED' AND jsonb_typeof(o->'presentation_version')='number' AND o->>'presentation_version' ~ '^[0-9]{1,16}$' AND (o->>'presentation_version')::numeric BETWEEN 1 AND 9007199254740991)
 AND (action<>'SET_COPY_PRESENTATION' OR o->>'status'<>'SUCCEEDED' OR o ? 'presentation_version')
 AND (NOT o ? 'user' OR action IN ('CREATE_USER','SET_USER_ENABLED','RENAME_USER','RESET_USER_PASSWORD','DELETE_USER','RESTORE_USER')
 AND jsonb_typeof(o->'user')='object'
 AND jsonb_typeof(o->'user'->'user_id')='string' AND o->'user'->>'user_id'=o->>'target_id'
 AND jsonb_typeof(o->'user'->'login')='string' AND length(o->'user'->>'login') BETWEEN 1 AND 64
 AND jsonb_typeof(o->'user'->'enabled')='boolean'
 AND jsonb_typeof(o->'user'->'version')='number' AND o->'user'->>'version' ~ '^[0-9]+$'
 AND (NOT (o->'user') ? 'deleted' OR jsonb_typeof(o->'user'->'deleted')='boolean')
 AND NOT EXISTS(SELECT 1 FROM jsonb_object_keys(o->'user') k WHERE k NOT IN ('user_id','login','enabled','version','deleted')))
 AND (NOT o ? 'capacity' OR action='PROBE_115' AND jsonb_typeof(o->'capacity')='object'
 AND jsonb_typeof(o->'capacity'->'status')='string' AND o->'capacity'->>'source'='115_GET_INFO' AND jsonb_typeof(o->'capacity'->'probed_at')='string'
 AND NOT EXISTS(SELECT 1 FROM jsonb_object_keys(o->'capacity') k WHERE k NOT IN ('status','total_bytes','used_bytes','remaining_bytes','source','probed_at')))
 AND (NOT o ? 'manifest' OR action='CANCEL_PENDING_MANIFEST' AND jsonb_typeof(o->'manifest')='object'
 AND NOT EXISTS(SELECT 1 FROM jsonb_object_keys(o->'manifest') k WHERE k NOT IN ('state','job_id','updated_at'))
 AND o->'manifest'->>'state' IN ('PENDING','ACTIVE','AWAITING_VISIBILITY','AWAITING_CANONICAL','READY','FAILED','CANCELED','RECOVERY_REQUIRED')
 AND (o->'manifest'->'job_id'='null'::jsonb OR o->'manifest'->>'job_id' ~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'))
 AND (action<>'CANCEL_PENDING_MANIFEST' OR o->>'status'<>'SUCCEEDED' OR o->'manifest'->>'state'='CANCELED' AND o->'manifest'->'job_id'='null'::jsonb)
 ,false) END AS valid
 FROM scoped
) SELECT actor_id::text,request_id::text,action,status,code,created_at,recorded_at,target_status,manifest_state,
 valid AND code<>'',statement_timestamp() FROM projected
 WHERE $6='ALL' OR status=$6 OR NOT valid OR code=''
 ORDER BY created_at DESC,request_id DESC LIMIT $10`

func (r *AdminAuditRepository) ReadAudit(ctx context.Context, a adminops.Actor, bindings []string, f adminaudit.Filter, p adminaudit.Position, request string) (adminaudit.Snapshot, error) {
	if !f.Valid() || f.ActorID != "" && f.ActorID != string(a.Principal.UserID) || len(bindings) < 1 || len(bindings) > 64 || request != "" && !adminread.ValidID(request) {
		return adminaudit.Snapshot{}, adminaudit.ErrInvalid
	}
	for _, b := range bindings {
		if !adminread.ValidID(b) {
			return adminaudit.Snapshot{}, adminaudit.ErrInvalid
		}
	}
	if f.Action == "" {
		f.Action = "ALL"
	}
	if f.Status == "" {
		f.Status = "ALL"
	}
	tx, e := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		return adminaudit.Snapshot{}, adminaudit.ErrUnavailable
	}
	defer adminRollback(tx)
	if e = auditActor(ctx, tx, a); e != nil {
		return adminaudit.Snapshot{}, e
	}
	nullableTime := func(z interface{ IsZero() bool }) any {
		if z.IsZero() {
			return nil
		}
		return z
	}
	rows, e := tx.Query(ctx, auditProjection, string(a.Principal.UserID), bindings, f.Action, nullableTime(f.From), nullableTime(f.Before), f.Status, adminNullable(request), nullableTime(p.CreatedAt), adminNullable(p.RequestID), f.Limit+1)
	if e != nil {
		return adminaudit.Snapshot{}, adminaudit.ErrUnavailable
	}
	result := adminaudit.Snapshot{Items: []adminaudit.Record{}}
	for rows.Next() {
		var row adminaudit.Record
		e = rows.Scan(&row.ActorID, &row.RequestID, &row.Action, &row.Status, &row.Code, &row.CreatedAt, &row.RecordedAt, &row.TargetStatus, &row.ManifestState, &row.Valid, &result.ReadAt)
		if e != nil {
			break
		}
		result.Items = append(result.Items, row)
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		return adminaudit.Snapshot{}, adminaudit.ErrUnavailable
	}
	if len(result.Items) == 0 {
		if e = tx.QueryRow(ctx, `SELECT statement_timestamp()`).Scan(&result.ReadAt); e != nil {
			return adminaudit.Snapshot{}, adminaudit.ErrUnavailable
		}
	}
	if r.afterRead != nil {
		if e = r.afterRead(ctx, tx); e != nil {
			return adminaudit.Snapshot{}, adminaudit.ErrUnavailable
		}
	}
	if e = tx.Commit(ctx); e != nil && !errors.Is(e, context.Canceled) {
		return adminaudit.Snapshot{}, adminaudit.ErrUnavailable
	}
	if e != nil {
		return adminaudit.Snapshot{}, e
	}
	// Release the read snapshot before the fresh auth check: a pool with one
	// connection must not wait for its own transaction to return a connection.
	if e = auditActor(ctx, r.db, a); e != nil {
		return adminaudit.Snapshot{}, e
	}
	return result, nil
}
