package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/nathanxiangang-web/panta/internal/adminops"
	"github.com/nathanxiangang-web/panta/internal/adminread"
)

var _ adminops.PendingCanceler = (*AdminOperationsRepository)(nil)

// Lock order: actor -> session -> action -> owner -> provider fence -> Manifest.
// Owner precedes Manifest to match user lifecycle/admission. Activation takes
// Manifest before inserting a Job. Provider fencing precedes its Manifest FK
// lock as in ClaimProviderTask. No Job, admission or provider is ever modified.
func (r *AdminOperationsRepository) ApplyPendingCancellation(ctx context.Context, a adminops.Actor, c adminops.Command, bindings []string) (adminops.Result, error) {
	if !c.Valid() || c.Action != adminops.CancelPending || len(bindings) < 1 || len(bindings) > 64 {
		return adminops.Result{}, adminops.ErrInvalid
	}
	seen := map[string]bool{}
	for _, id := range bindings {
		if !adminread.ValidID(id) || seen[id] {
			return adminops.Result{}, adminops.ErrInvalid
		}
		seen[id] = true
	}
	tx, e := r.begin(ctx, a)
	if e != nil {
		return adminops.Result{}, e
	}
	defer adminRollback(tx)
	result, done, e := lockedAdminAction(ctx, tx, a, c)
	if e != nil || done {
		return result, e
	}
	result = rejected(c, "MANIFEST_NOT_FOUND")
	// Nonlocking scoped probe discovers lock identity only. Repeat ALL scope and
	// ownership predicates under the Manifest lock; never trust this probe as CAS.
	var owner string
	e = tx.QueryRow(ctx, `SELECT user_id::text FROM acquisition_manifests WHERE manifest_id=$1 AND user_id IS NOT NULL AND target_storage_binding_id=ANY($2::uuid[])`, c.TargetID, bindings).Scan(&owner)
	if errors.Is(e, pgx.ErrNoRows) {
		return appendAdminResult(ctx, tx, a, result)
	}
	if e != nil {
		return adminops.Result{}, e
	}
	var enabled bool
	var deleted bool
	e = tx.QueryRow(ctx, `SELECT enabled,deleted_at IS NOT NULL FROM product_users WHERE user_id=$1 FOR SHARE`, owner).Scan(&enabled, &deleted)
	if errors.Is(e, pgx.ErrNoRows) {
		return appendAdminResult(ctx, tx, a, result)
	}
	if e != nil {
		return adminops.Result{}, e
	}
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1,hashtext($2))`, providerTaskFenceLockClass, c.TargetID); e != nil {
		return adminops.Result{}, e
	}
	var actualOwner, binding string
	var copy *string
	var hasProviderResult bool
	state := &adminops.ManifestOutcome{}
	e = tx.QueryRow(ctx, `SELECT user_id::text,target_storage_binding_id::text,state,job_id::text,result_copy_id::text,updated_at,result_name IS NOT NULL FROM acquisition_manifests WHERE manifest_id=$1 AND user_id=$2 AND target_storage_binding_id=ANY($3::uuid[]) FOR UPDATE`, c.TargetID, owner, bindings).Scan(&actualOwner, &binding, &state.State, &state.JobID, &copy, &state.UpdatedAt, &hasProviderResult)
	if errors.Is(e, pgx.ErrNoRows) {
		return appendAdminResult(ctx, tx, a, result)
	}
	if e != nil {
		return adminops.Result{}, e
	}
	if r.afterPendingLock != nil {
		if e = r.afterPendingLock(ctx, tx); e != nil {
			return adminops.Result{}, e
		}
	}
	// DB time recheck AFTER potentially waiting on target/fence locks. Row locks
	// block revocation, but passage of time can still expire an actor session.
	if e = adminActor(ctx, tx, a); e != nil {
		return adminops.Result{}, e
	}
	// A corrupt foreign Job link must not disclose an unrelated Job identity in
	// a conflict receipt. The caller can re-read the exact diagnostic endpoint.
	if state.JobID != nil {
		var linked bool
		e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM jobs WHERE job_id=$1 AND job_type='ACQUISITION' AND idempotency_key='acquisition:'||$2::text AND payload=jsonb_build_object('schema_version',1,'manifest_id',$2::text) AND payload->>'schema_version'='1')`, *state.JobID, c.TargetID).Scan(&linked)
		if e != nil {
			return adminops.Result{}, e
		}
		if !linked {
			result.Code = "MANIFEST_UNAVAILABLE"
			return appendAdminResult(ctx, tx, a, result)
		}
	}
	result.Manifest = state
	result.Code = "PENDING_CHANGED"
	switch {
	case !enabled || deleted:
		result.Code = "OWNER_UNAVAILABLE"
	case actualOwner != c.Pending.OwnerUserID || binding != c.Pending.BindingID || !state.UpdatedAt.Equal(c.Pending.UpdatedAt):
	case state.State != "PENDING" || state.JobID != nil || copy != nil:
	case hasProviderResult:
		result.Code = "PENDING_NOT_ISOLATED"
	default:
		var isolated bool
		// Reject inconsistent orphan Jobs as well as correctly keyed Jobs. No raw
		// payload, source, reference or admission ledger data leaves this boolean.
		e = tx.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM acquisition_provider_tasks WHERE manifest_id=$1)
   AND NOT EXISTS(SELECT 1 FROM jobs WHERE idempotency_key='acquisition:'||$1::text OR (job_type='ACQUISITION' AND payload->>'manifest_id'=$1::text))
   AND NOT EXISTS(SELECT 1 FROM product_operations WHERE manifest_id=$1)`, c.TargetID).Scan(&isolated)
		if e != nil {
			return adminops.Result{}, e
		}
		if !isolated {
			result.Code = "PENDING_NOT_ISOLATED"
			break
		}
		e = tx.QueryRow(ctx, `UPDATE acquisition_manifests SET state='CANCELED',updated_at=GREATEST(clock_timestamp(),updated_at+interval '1 microsecond') WHERE manifest_id=$1 AND user_id=$2 AND target_storage_binding_id=$3 AND updated_at=$4 AND state='PENDING' AND job_id IS NULL AND result_copy_id IS NULL RETURNING updated_at`, c.TargetID, actualOwner, binding, c.Pending.UpdatedAt).Scan(&state.UpdatedAt)
		if e != nil {
			return adminops.Result{}, e
		}
		state.State = "CANCELED"
		result.Status = "SUCCEEDED"
		result.Code = "PENDING_CANCELED"
	}
	if r.beforeUserCommit != nil {
		if e = r.beforeUserCommit(ctx, tx); e != nil {
			return adminops.Result{}, e
		}
	}
	if e = adminActor(ctx, tx, a); e != nil {
		return adminops.Result{}, e
	}
	// Audit result and cancellation are the same commit. Any error is UNKNOWN to
	// the caller; never perform a second apply after a failed/ambiguous commit.
	return appendAdminResult(ctx, tx, a, result)
}
