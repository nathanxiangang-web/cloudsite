package postgres

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/nathanxiangang-web/panta/internal/adminread"
)

var _ adminread.DownloadDiagnosticReader = (*AdminReadRepository)(nil)

func (r *AdminReadRepository) ReadDownloadDetail(ctx context.Context, bindings []string, id string) (adminread.DownloadDetailRecord, error) {
	if !adminread.ValidID(id) || len(bindings) < 1 || len(bindings) > 64 {
		return adminread.DownloadDetailRecord{}, adminread.ErrInvalid
	}
	for _, b := range bindings {
		if !adminread.ValidID(b) {
			return adminread.DownloadDetailRecord{}, adminread.ErrInvalid
		}
	}
	tx, e := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		return adminread.DownloadDetailRecord{}, adminReadError(e)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var result adminread.DownloadDetailRecord
	d := &result.DownloadDetail
	d.Source = "CLOUDSITE_POSTGRES_MANIFEST_JOB_COPY"
	var copyID, bindingID, availability *string
	var c adminread.DownloadCopy
	var ownerValid bool
	// One coherent SELECT; raw last_error/payload/paths never leave the database.
	// READY is historical; a linked Copy may legitimately be REMOVED now.
	rows, e := tx.Query(ctx, `SELECT `+adminDownloadColumns+`,statement_timestamp(),
 j.created_at,j.updated_at,j.started_at,j.finished_at,j.next_attempt_at,
 COALESCE(j.last_error IS NOT NULL AND j.last_error<>'',false),
 CASE WHEN j.last_error IS NULL OR j.last_error='' THEN 'NONE'
 WHEN j.last_error IN ('POLL_TRANSPORT_TIMEOUT','POLL_TRANSPORT_OTHER','POLL_HTTP_STATUS','POLL_DECODE','POLL_PROVIDER_TEMPORARY','POLL_PERMANENT_OR_UNKNOWN') THEN j.last_error ELSE 'RECORDED_REDACTED' END,
 c.copy_id::text,c.storage_binding_id::text,c.availability,COALESCE(c.updated_at,'0001-01-01 00:00:00+00'::timestamptz),
 EXISTS(SELECT 1 FROM product_users u WHERE u.user_id=m.user_id)
 FROM acquisition_manifests m LEFT JOIN jobs j ON j.job_id=m.job_id LEFT JOIN copies c ON c.copy_id=m.result_copy_id
 WHERE m.manifest_id=$1 AND m.user_id IS NOT NULL AND m.target_storage_binding_id=ANY($2::uuid[])`, id, bindings)
	if e != nil {
		return result, adminReadError(e)
	}
	if !rows.Next() {
		e = rows.Err()
		rows.Close()
		if e != nil {
			return result, adminReadError(e)
		}
		return result, adminread.ErrDownloadNotFound
	}
	e = rows.Scan(&d.ManifestID, &d.OwnerUserID, &d.BindingID, &d.JobID, &d.ManifestState, &d.JobState, &d.ResultCopyID, &d.ClaimAttempts, &d.FailureCount, &d.CreatedAt, &d.UpdatedAt, &result.LinkValid, &d.MaxAttempts, &d.ReadAt, &d.JobCreatedAt, &d.JobUpdatedAt, &d.StartedAt, &d.FinishedAt, &d.NextAttemptAt, &d.ErrorRecorded, &d.ErrorCode, &copyID, &bindingID, &availability, &c.UpdatedAt, &ownerValid)
	if e != nil {
		rows.Close()
		return result, adminReadError(e)
	}
	if rows.Next() {
		rows.Close()
		return result, adminread.ErrUnavailable
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return result, adminReadError(e)
	}
	result.LinkValid = result.LinkValid && ownerValid
	if copyID != nil && bindingID != nil && availability != nil {
		c.CopyID = *copyID
		c.BindingID = *bindingID
		c.Availability = *availability
		d.Copy = &c
	}
	if r.afterRead != nil {
		if e = r.afterRead(ctx); e != nil {
			return result, adminReadError(e)
		}
	}
	if e = tx.Commit(ctx); e != nil {
		return result, adminReadError(e)
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return result, ctx.Err()
	}
	return result, nil
}
