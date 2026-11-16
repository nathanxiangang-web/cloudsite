package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/nathanxiangang-web/panta/internal/acquisition"
	"github.com/nathanxiangang-web/panta/internal/jobs"
	"github.com/nathanxiangang-web/panta/internal/providers/contracts"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

const acquisitionDonorManifestColumns = `
m.manifest_id::text, m.user_id::text, m.source_type, m.source_ref, m.expected_name, m.result_name,
m.result_copy_id::text, m.target_storage_binding_id, m.target_path, m.asset_id::text, m.release_id::text,
m.variant_id::text, m.job_id::text, m.state, m.created_at, m.updated_at,
m.user_space_id::text, m.target_provider_connection_id::text, m.target_provider_scope`

type AcquisitionCopyDonorRepository struct {
	db acquisitionCopyDB
}

func NewAcquisitionCopyDonorRepository(db acquisitionCopyDB) (*AcquisitionCopyDonorRepository, error) {
	if db == nil {
		return nil, errors.New("acquisition copy donor database is required")
	}
	return &AcquisitionCopyDonorRepository{db: db}, nil
}

func (repository *AcquisitionCopyDonorRepository) FindCopyDonor(ctx context.Context, sourceType, sourceRef string,
	connectionID storage.ConnectionID, excludeManifestID acquisition.ManifestID) (acquisition.CopyDonorCandidate, error) {
	var candidate acquisition.CopyDonorCandidate
	var jobType, jobState, taskProviderID, taskRef, taskState string
	var taskCreatedAt, taskUpdatedAt = candidate.Task.CreatedAt, candidate.Task.UpdatedAt
	err := repository.db.QueryRow(ctx, `SELECT `+acquisitionDonorManifestColumns+`,
j.job_type, j.state, pt.provider_id, pt.provider_task_ref, pt.state, pt.created_at, pt.updated_at,
e.provider_connection_id::text, e.provider_object_ref, e.provider_parent_scope, e.result_name,
e.size_bytes, e.hash_algorithm, e.content_hash, e.observed_at
FROM acquisition_manifests m
JOIN jobs j ON j.job_id = m.job_id
JOIN acquisition_provider_tasks pt ON pt.manifest_id = m.manifest_id AND pt.job_id = m.job_id
JOIN acquisition_result_object_evidence e ON e.manifest_id = m.manifest_id
JOIN copies c ON c.copy_id = m.result_copy_id
WHERE m.source_type = $1 AND m.source_ref = $2 AND m.target_provider_connection_id = $3
  AND m.manifest_id <> $4 AND m.state = 'READY' AND m.result_copy_id IS NOT NULL
  AND j.job_type = 'ACQUISITION' AND j.state = 'SUCCEEDED'
  AND pt.provider_id = '115' AND pt.state = 'REFERENCE_KNOWN'
  AND c.availability = 'PRESENT' AND c.storage_binding_id = m.target_storage_binding_id
ORDER BY m.updated_at DESC, m.manifest_id DESC
LIMIT 1`, sourceType, sourceRef, string(connectionID), string(excludeManifestID)).Scan(
		&candidate.Manifest.ID, &candidate.Manifest.UserID, &candidate.Manifest.SourceType, &candidate.Manifest.SourceRef,
		&candidate.Manifest.ExpectedName, &candidate.Manifest.ResultName, &candidate.Manifest.ResultCopyID,
		&candidate.Manifest.TargetStorageBindingID, &candidate.Manifest.TargetPath, &candidate.Manifest.AssetID,
		&candidate.Manifest.ReleaseID, &candidate.Manifest.VariantID, &candidate.Manifest.JobID,
		&candidate.Manifest.State, &candidate.Manifest.CreatedAt, &candidate.Manifest.UpdatedAt,
		&candidate.Manifest.UserSpaceID, &candidate.Manifest.TargetProviderConnectionID, &candidate.Manifest.TargetProviderScope,
		&jobType, &jobState, &taskProviderID, &taskRef, &taskState, &taskCreatedAt, &taskUpdatedAt,
		&candidate.Evidence.ProviderConnectionID, &candidate.Evidence.ProviderObjectRef, &candidate.Evidence.ProviderParentScope,
		&candidate.Evidence.ResultName, &candidate.Evidence.SizeBytes, &candidate.Evidence.HashAlgorithm,
		&candidate.Evidence.ContentHash, &candidate.Evidence.ObservedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return acquisition.CopyDonorCandidate{}, acquisition.ErrCopyDonorUnavailable
	}
	if err != nil {
		return acquisition.CopyDonorCandidate{}, acquisitionCopyError("find donor", err)
	}
	candidate.Job = jobs.Job{ID: *candidate.Manifest.JobID, Type: jobType, State: jobs.State(jobState)}
	candidate.Task = acquisition.ProviderTask{
		ManifestID: candidate.Manifest.ID, JobID: *candidate.Manifest.JobID, ProviderID: contracts.ProviderID(taskProviderID),
		ProviderTaskRef: taskRef, State: acquisition.ProviderTaskState(taskState),
		CreatedAt: taskCreatedAt, UpdatedAt: taskUpdatedAt,
	}
	candidate.Evidence.ManifestID = candidate.Manifest.ID
	return candidate, nil
}
