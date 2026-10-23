package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/nathanxiangang-web/panta/internal/adminread"
)

var _ adminread.OverviewReader = (*AdminReadRepository)(nil)

// A fixed-size aggregate over persisted pairs, not a sampled download page.
// Unknown states or broken links remain in the denominator and fail closed.
func (r *AdminReadRepository) ReadOverview(ctx context.Context, bindings []string, owner string) (adminread.OverviewRecord, error) {
	if len(bindings) < 1 || len(bindings) > 64 || owner != "" && !adminread.ValidID(owner) {
		return adminread.OverviewRecord{}, adminread.ErrInvalid
	}
	seen := map[string]bool{}
	for _, id := range bindings {
		if !adminread.ValidID(id) || seen[id] {
			return adminread.OverviewRecord{}, adminread.ErrInvalid
		}
		seen[id] = true
	}
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return adminread.OverviewRecord{}, adminReadError(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	result := adminread.OverviewRecord{Overview: adminread.Overview{Source: adminread.OverviewSource, Scope: adminread.OverviewScope, OwnerUserID: owner}}
	m, j := &result.Manifests, &result.Jobs
	// Match the existing admin download read contract, including exact payload
	// schema spelling (JSON numeric 1.0 must not masquerade as frozen version 1).
	err = tx.QueryRow(ctx, `WITH pairs AS (
 SELECT m.state AS ms,j.state AS js,m.job_id,
 COALESCE(m.created_at<=m.updated_at AND
 ((m.state='READY' AND m.result_copy_id IS NOT NULL) OR (m.state<>'READY' AND m.result_copy_id IS NULL)) AND
 CASE WHEN m.state='PENDING' THEN m.job_id IS NULL ELSE
 j.job_id=m.job_id AND j.job_type='ACQUISITION'
 AND j.idempotency_key='acquisition:'||m.manifest_id::text
 AND j.payload=jsonb_build_object('schema_version',1,'manifest_id',m.manifest_id::text)
 AND j.payload->>'schema_version'='1'
 AND j.claim_attempts>=0 AND j.attempt_count>=0 AND j.max_attempts>=1 AND j.attempt_count<=j.max_attempts
 AND CASE m.state
 WHEN 'ACTIVE' THEN j.state IN ('QUEUED','RUNNING','RETRY_WAIT')
 WHEN 'AWAITING_VISIBILITY' THEN j.state IN ('RUNNING','RETRY_WAIT')
 WHEN 'AWAITING_CANONICAL' THEN j.state IN ('RUNNING','RETRY_WAIT')
 WHEN 'READY' THEN j.state='SUCCEEDED' WHEN 'FAILED' THEN j.state='FAILED'
 WHEN 'CANCELED' THEN j.state='CANCELED' WHEN 'RECOVERY_REQUIRED' THEN j.state='RECOVERY_REQUIRED'
 ELSE false END END,false) AS valid
 FROM acquisition_manifests m LEFT JOIN jobs j ON j.job_id=m.job_id
 WHERE m.user_id IS NOT NULL AND m.target_storage_binding_id=ANY($1::uuid[])
 AND ($2::uuid IS NULL OR m.user_id=$2::uuid)
 ) SELECT statement_timestamp(),count(*),
 count(*) FILTER(WHERE ms='PENDING'),count(*) FILTER(WHERE ms='ACTIVE'),
 count(*) FILTER(WHERE ms='AWAITING_VISIBILITY'),count(*) FILTER(WHERE ms='AWAITING_CANONICAL'),
 count(*) FILTER(WHERE ms='READY'),count(*) FILTER(WHERE ms='FAILED'),
 count(*) FILTER(WHERE ms='CANCELED'),count(*) FILTER(WHERE ms='RECOVERY_REQUIRED'),
 count(*) FILTER(WHERE ms IN ('PENDING','ACTIVE','AWAITING_VISIBILITY','AWAITING_CANONICAL')),
 count(job_id),count(*) FILTER(WHERE js='QUEUED'),count(*) FILTER(WHERE js='RUNNING'),
 count(*) FILTER(WHERE js='RETRY_WAIT'),count(*) FILTER(WHERE js='SUCCEEDED'),
 count(*) FILTER(WHERE js='FAILED'),count(*) FILTER(WHERE js='CANCELED'),
 count(*) FILTER(WHERE js='RECOVERY_REQUIRED'),count(*) FILTER(WHERE NOT valid) FROM pairs`, bindings, adminNullable(owner)).Scan(
		&result.ReadAt, &m.Total, &m.Pending, &m.Active, &m.AwaitingVisibility, &m.AwaitingCanonical, &m.Ready, &m.Failed, &m.Canceled, &m.RecoveryRequired, &m.Incomplete,
		&j.Total, &j.Queued, &j.Running, &j.RetryWait, &j.Succeeded, &j.Failed, &j.Canceled, &j.RecoveryRequired, &result.InvalidRecords)
	if err != nil {
		return adminread.OverviewRecord{}, adminReadError(err)
	}
	if r.afterRead != nil {
		if err = r.afterRead(ctx); err != nil {
			return adminread.OverviewRecord{}, adminReadError(err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return adminread.OverviewRecord{}, adminReadError(err)
	}
	return result, nil
}
