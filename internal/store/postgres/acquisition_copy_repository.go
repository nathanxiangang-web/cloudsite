package postgres

import (
	"context"

	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/nathanxiangang-web/panta/internal/acquisition"
	"github.com/nathanxiangang-web/panta/internal/jobs"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

type acquisitionCopyDB interface {
	Begin(context.Context) (pgx.Tx, error)
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type AcquisitionCopyRepository struct {
	db acquisitionCopyDB
}

func NewAcquisitionCopyRepository(db acquisitionCopyDB) (*AcquisitionCopyRepository, error) {
	if db == nil {
		return nil, errors.New("acquisition copy database is required")
	}
	return &AcquisitionCopyRepository{db: db}, nil
}

func (repository *AcquisitionCopyRepository) SaveResultObjectEvidence(ctx context.Context, evidence acquisition.ResultObjectEvidence) error {
	if err := evidence.Validate(); err != nil {
		return err
	}
	_, err := repository.db.Exec(ctx, `INSERT INTO acquisition_result_object_evidence (
manifest_id, provider_connection_id, provider_object_ref, provider_parent_scope,
result_name, size_bytes, hash_algorithm, content_hash, observed_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		string(evidence.ManifestID), string(evidence.ProviderConnectionID), evidence.ProviderObjectRef,
		evidence.ProviderParentScope, evidence.ResultName, evidence.SizeBytes,
		evidence.HashAlgorithm, evidence.ContentHash, evidence.ObservedAt)
	return acquisitionCopyError("save evidence", err)
}

func (repository *AcquisitionCopyRepository) GetResultObjectEvidence(ctx context.Context, manifestID acquisition.ManifestID) (acquisition.ResultObjectEvidence, error) {
	if manifestID == "" {
		return acquisition.ResultObjectEvidence{}, acquisition.ErrInvalidResultEvidence
	}
	var evidence acquisition.ResultObjectEvidence
	var id, connectionID string
	err := repository.db.QueryRow(ctx, `SELECT manifest_id::text, provider_connection_id::text,
provider_object_ref, provider_parent_scope, result_name, size_bytes, hash_algorithm, content_hash, observed_at
FROM acquisition_result_object_evidence WHERE manifest_id=$1`, string(manifestID)).Scan(
		&id, &connectionID, &evidence.ProviderObjectRef, &evidence.ProviderParentScope,
		&evidence.ResultName, &evidence.SizeBytes, &evidence.HashAlgorithm, &evidence.ContentHash, &evidence.ObservedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return acquisition.ResultObjectEvidence{}, acquisition.ErrCopyOperationNotFound
	}
	if err != nil {
		return acquisition.ResultObjectEvidence{}, acquisitionCopyError("get evidence", err)
	}
	evidence.ManifestID = acquisition.ManifestID(id)
	evidence.ProviderConnectionID = storage.ConnectionID(connectionID)
	return evidence, nil
}

func (repository *AcquisitionCopyRepository) CreateCopyOperation(ctx context.Context, operation acquisition.CopyOperation) error {
	return repository.createCopyOperation(ctx, operation, nil)
}

func (repository *AcquisitionCopyRepository) CreateCopyOperationWithLease(ctx context.Context, operation acquisition.CopyOperation, owner string, expectedClaim int) error {
	return repository.createCopyOperation(ctx, operation, &jobs.LeaseRequest{ID: operation.JobID, Owner: owner, ExpectedClaim: expectedClaim})
}

func (repository *AcquisitionCopyRepository) createCopyOperation(ctx context.Context, operation acquisition.CopyOperation, lease *jobs.LeaseRequest) error {
	if err := operation.Validate(); err != nil {
		return err
	}
	baseline, err := json.Marshal(operation.DestinationBefore)
	if err != nil {
		return acquisition.ErrInvalidCopyOperation
	}
	tx, err := repository.db.Begin(ctx)
	if err != nil {
		return acquisitionCopyError("begin reservation", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(ctx, `SET LOCAL lock_timeout = '`+providerTaskClaimLockTimeout+`'`); err != nil {
		return acquisitionCopyError("bound reservation lock", err)
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1, hashtext($2))`,
		providerTaskFenceLockClass, string(operation.ManifestID)); err != nil {
		return acquisitionCopyError("lock reservation", err)
	}
	if lease != nil {
		if lease.ID != operation.JobID || lease.Owner == "" || lease.ExpectedClaim < 1 {
			return acquisition.ErrInvalidCopyOperation
		}
		var jobState, leaseOwner string
		var claimAttempts int
		var validExpiry bool
		if err := tx.QueryRow(ctx, `SELECT state, lease_owner, claim_attempts,
(lease_expires_at > clock_timestamp()) FROM jobs WHERE job_id=$1 FOR UPDATE`, string(operation.JobID)).Scan(
			&jobState, &leaseOwner, &claimAttempts, &validExpiry); err != nil {
			return acquisitionCopyError("check reservation lease", err)
		}
		if jobState != string(jobs.StateRunning) || leaseOwner != lease.Owner || claimAttempts != lease.ExpectedClaim || !validExpiry {
			return acquisition.ErrCopyOperationConflict
		}
	}
	var providerTasks int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM acquisition_provider_tasks WHERE manifest_id=$1`, string(operation.ManifestID)).Scan(&providerTasks); err != nil {
		return acquisitionCopyError("check provider task fence", err)
	}
	if providerTasks != 0 {
		return acquisition.ErrCopyOperationConflict
	}
	if _, err := tx.Exec(ctx, `INSERT INTO acquisition_copy_operations (
copy_operation_id, manifest_id, job_id, donor_manifest_id, provider_connection_id,
request_digest, destination_before, status, provider_result_ref, last_error_kind,
provider_accepted_at, created_at, updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		string(operation.ID), string(operation.ManifestID), string(operation.JobID),
		string(operation.DonorManifestID), string(operation.ProviderConnectionID), operation.RequestDigest,
		baseline, string(operation.Status), operation.ProviderResultRef, operation.LastErrorKind,
		operation.ProviderAcceptedAt, operation.CreatedAt, operation.UpdatedAt); err != nil {
		return acquisitionCopyError("create operation", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return acquisitionCopyError("commit reservation", err)
	}
	return nil
}

func (repository *AcquisitionCopyRepository) GetCopyOperation(ctx context.Context, manifestID acquisition.ManifestID) (acquisition.CopyOperation, error) {
	if manifestID == "" {
		return acquisition.CopyOperation{}, acquisition.ErrInvalidCopyOperation
	}
	var operation acquisition.CopyOperation
	var id, jobID, donorID, connectionID string
	var baseline []byte
	err := repository.db.QueryRow(ctx, `SELECT copy_operation_id::text, manifest_id::text, job_id::text,
donor_manifest_id::text, provider_connection_id::text, request_digest, destination_before, status,
provider_result_ref, last_error_kind, provider_accepted_at, created_at, updated_at
FROM acquisition_copy_operations WHERE manifest_id=$1`, string(manifestID)).Scan(
		&id, &operation.ManifestID, &jobID, &donorID, &connectionID, &operation.RequestDigest,
		&baseline, &operation.Status, &operation.ProviderResultRef, &operation.LastErrorKind,
		&operation.ProviderAcceptedAt, &operation.CreatedAt, &operation.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return acquisition.CopyOperation{}, acquisition.ErrCopyOperationNotFound
	}
	if err != nil {
		return acquisition.CopyOperation{}, acquisitionCopyError("get operation", err)
	}
	if err := json.Unmarshal(baseline, &operation.DestinationBefore); err != nil {
		return acquisition.CopyOperation{}, fmt.Errorf("%w: destination baseline", acquisition.ErrInvalidCopyOperation)
	}
	operation.ID = acquisition.CopyOperationID(id)
	operation.JobID = jobs.JobID(jobID)
	operation.DonorManifestID = acquisition.ManifestID(donorID)
	operation.ProviderConnectionID = storage.ConnectionID(connectionID)
	return operation, nil
}

func (repository *AcquisitionCopyRepository) UpdateCopyOperation(ctx context.Context, operation acquisition.CopyOperation, previousUpdatedAt time.Time) error {
	if err := operation.Validate(); err != nil {
		return err
	}
	tag, err := repository.db.Exec(ctx, `UPDATE acquisition_copy_operations SET
status=$2, provider_result_ref=$3, last_error_kind=$4, provider_accepted_at=$5, updated_at=$6
WHERE manifest_id=$1 AND updated_at=$7`, string(operation.ManifestID), string(operation.Status),
		operation.ProviderResultRef, operation.LastErrorKind, operation.ProviderAcceptedAt,
		operation.UpdatedAt, previousUpdatedAt)
	if err != nil {
		return acquisitionCopyError("update operation", err)
	}
	if tag.RowsAffected() != 1 {
		return acquisition.ErrCopyOperationConflict
	}
	return nil
}

func acquisitionCopyError(operation string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Code {
		case "23505":
			return fmt.Errorf("%w: %s", acquisition.ErrCopyOperationConflict, postgresError.ConstraintName)
		case "23503", "23514", "22P02":
			return fmt.Errorf("%w: %s", acquisition.ErrInvalidCopyOperation, postgresError.ConstraintName)
		}
	}
	return fmt.Errorf("acquisition copy %s: %w", operation, err)
}
