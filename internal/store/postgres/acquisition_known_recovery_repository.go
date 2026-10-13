package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/nathanxiangang-web/panta/internal/acquisition"
	"github.com/nathanxiangang-web/panta/internal/jobs"
)

type KnownRecoveryRepository struct {
	db                  providerOutcomeDB
	afterManifestUpdate func(context.Context, pgx.Tx) error // rollback test seam
}

func NewKnownRecoveryRepository(db providerOutcomeDB) (*KnownRecoveryRepository, error) {
	if db == nil {
		return nil, acquisition.ErrKnownRecoveryConflict
	}
	return &KnownRecoveryRepository{db: db}, nil
}

// ResumeKnownAcquisition is the only paired recovery mutation. It never calls a
// provider and has no authority to create or rewrite provider-task rows.
func (repository *KnownRecoveryRepository) ResumeKnownAcquisition(ctx context.Context, request acquisition.KnownRecoveryRequest) (acquisition.ProviderOutcomeResult, error) {
	if !request.Valid() {
		return acquisition.ProviderOutcomeResult{}, acquisition.ErrKnownRecoveryConflict
	}
	tx, err := repository.db.Begin(ctx)
	if err != nil {
		return acquisition.ProviderOutcomeResult{}, acquisition.ErrPersistence
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err = tx.Exec(ctx, `SET LOCAL lock_timeout = '5s'`); err != nil {
		return acquisition.ProviderOutcomeResult{}, acquisition.ErrPersistence
	}
	manifest, err := scanAcquisitionManifest(tx.QueryRow(ctx, `SELECT `+acquisitionManifestColumns+` FROM acquisition_manifests WHERE manifest_id=$1 FOR UPDATE`, string(request.Scope.ManifestID)))
	if err != nil {
		return acquisition.ProviderOutcomeResult{}, acquisition.ErrKnownRecoveryConflict
	}
	job, err := getOneJob(ctx, tx, `SELECT `+jobColumns+` FROM jobs WHERE job_id=$1 FOR UPDATE`, string(request.Scope.JobID))
	if err != nil {
		return acquisition.ProviderOutcomeResult{}, acquisition.ErrKnownRecoveryConflict
	}
	task, err := scanAcquisitionProviderTask(tx.QueryRow(ctx, `SELECT `+acquisitionProviderTaskColumns+` FROM acquisition_provider_tasks WHERE manifest_id=$1 FOR UPDATE`, string(request.Scope.ManifestID)))
	if err != nil || !request.Scope.Matches(manifest, task) || manifest.ResultCopyID != nil ||
		acquisition.ValidateLinkedAcquisitionJob(manifest.ID, job) != nil || job.ClaimAttempts != request.ExpectedClaim {
		return acquisition.ProviderOutcomeResult{}, acquisition.ErrKnownRecoveryConflict
	}
	// Lock mapping identities too; a concurrent administrator cannot swap the
	// namespace between validation and finalization. Disabled is intentional at
	// operator recovery time; execution requires ACTIVE through the normal ports.
	var connection, root, provider string
	err = tx.QueryRow(ctx, `SELECT storage_connection_id::text,indexcore_root_id FROM storage_bindings WHERE storage_binding_id=$1 FOR SHARE`, string(request.Scope.BindingID)).Scan(&connection, &root)
	if err != nil || connection != string(request.Scope.ConnectionID) || root != request.Scope.RootID {
		return acquisition.ProviderOutcomeResult{}, acquisition.ErrKnownRecoveryConflict
	}
	err = tx.QueryRow(ctx, `SELECT provider_type FROM storage_connections WHERE storage_connection_id=$1 FOR SHARE`, connection).Scan(&provider)
	if err != nil || provider != string(request.Scope.ProviderID) {
		return acquisition.ProviderOutcomeResult{}, acquisition.ErrKnownRecoveryConflict
	}
	if job.LeaseOwner != nil || job.LeaseExpiresAt != nil || job.FinishedAt != nil {
		return acquisition.ProviderOutcomeResult{}, acquisition.ErrKnownRecoveryConflict
	}
	if manifest.State == acquisition.StateActive && job.State == jobs.StateRetryWait && job.NextAttemptAt != nil {
		// Exact identity + unchanged generation, read-only replay; never renew its
		// retry deadline or refund its error budget.
		return acquisition.ProviderOutcomeResult{Manifest: manifest, Job: job}, nil
	}
	if manifest.State != acquisition.StateRecoveryRequired || job.State != jobs.StateRecoveryRequired ||
		!manifest.UpdatedAt.Equal(request.ManifestUpdatedAt) || !job.UpdatedAt.Equal(request.JobUpdatedAt) ||
		job.NextAttemptAt != nil || job.FailureCount >= job.MaxAttempts {
		return acquisition.ProviderOutcomeResult{}, acquisition.ErrKnownRecoveryConflict
	}
	var now time.Time // one database clock value for both mutations and retry
	err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now)
	if err != nil {
		return acquisition.ProviderOutcomeResult{}, acquisition.ErrPersistence
	}
	manifest, err = scanAcquisitionManifest(tx.QueryRow(ctx, `UPDATE acquisition_manifests SET state='ACTIVE',updated_at=$2 WHERE manifest_id=$1 AND state='RECOVERY_REQUIRED' AND updated_at=$3 RETURNING `+acquisitionManifestColumns, string(manifest.ID), now, request.ManifestUpdatedAt))
	if err != nil {
		return acquisition.ProviderOutcomeResult{}, acquisition.ErrKnownRecoveryConflict
	}
	if repository.afterManifestUpdate != nil {
		if err = repository.afterManifestUpdate(ctx, tx); err != nil {
			return acquisition.ProviderOutcomeResult{}, acquisition.ErrPersistence
		}
	}
	job, err = scanJob(tx.QueryRow(ctx, `UPDATE jobs AS job SET state='RETRY_WAIT',next_attempt_at=$2::timestamptz+($3*interval '1 microsecond'),updated_at=$2 WHERE job_id=$1 AND state='RECOVERY_REQUIRED' AND claim_attempts=$4 AND updated_at=$5 AND lease_owner IS NULL AND lease_expires_at IS NULL RETURNING `+updatedJobColumns, string(job.ID), now, request.RetryDelay.Microseconds(), request.ExpectedClaim, request.JobUpdatedAt))
	if err != nil {
		return acquisition.ProviderOutcomeResult{}, acquisition.ErrKnownRecoveryConflict
	}
	if err = tx.Commit(ctx); err != nil {
		return acquisition.ProviderOutcomeResult{}, acquisition.ErrPersistence
	}
	return acquisition.ProviderOutcomeResult{Manifest: manifest, Job: job, Changed: true}, nil
}
