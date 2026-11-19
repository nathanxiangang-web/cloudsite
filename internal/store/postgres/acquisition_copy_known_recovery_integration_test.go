package postgres

import (
	"bytes"
	"testing"
	"time"

	"github.com/nathanxiangang-web/panta/internal/acquisition"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

func TestPostgresKnownRecoveryRejectsCopyOperationConnectionMismatch(t *testing.T) {
	ctx, pool, recovery, request := seedKnownRecovery(t)
	if _, err := pool.Exec(ctx, `DELETE FROM acquisition_provider_tasks WHERE manifest_id=$1`, string(request.Scope.ManifestID)); err != nil {
		t.Fatal(err)
	}
	copies, err := NewAcquisitionCopyRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := copies.SaveResultObjectEvidence(ctx, acquisition.ResultObjectEvidence{
		ManifestID: request.Scope.ManifestID, ProviderConnectionID: request.Scope.ConnectionID,
		ProviderObjectRef: "donor-file", ProviderParentScope: "donor-dir", ResultName: "result.bin",
		SizeBytes: 42, HashAlgorithm: "SHA1", ContentHash: "0123456789abcdef0123456789abcdef01234567", ObservedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	operation := acquisition.CopyOperation{
		ID: acquisition.CopyOperationID(adminUUID(9001)), ManifestID: request.Scope.ManifestID, JobID: request.Scope.JobID,
		DonorManifestID: request.Scope.ManifestID, ProviderConnectionID: storage.ConnectionID(request.Scope.ConnectionID),
		RequestDigest: bytes.Repeat([]byte{1}, 32), DestinationBefore: []string{}, Status: acquisition.CopyProviderUnknown,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := copies.CreateCopyOperation(ctx, operation); err != nil {
		t.Fatal(err)
	}
	otherConnection := adminUUID(9002)
	if _, err := pool.Exec(ctx, `INSERT INTO storage_connections (
storage_connection_id,provider_type,credential_ref,status,created_at,updated_at
) SELECT $1,provider_type,credential_ref,status,clock_timestamp(),clock_timestamp()
FROM storage_connections WHERE storage_connection_id=$2`, otherConnection, string(request.Scope.ConnectionID)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE acquisition_copy_operations SET provider_connection_id=$1 WHERE manifest_id=$2`, otherConnection, string(request.Scope.ManifestID)); err != nil {
		t.Fatal(err)
	}
	if _, err := recovery.ResumeKnownAcquisition(ctx, request); err != acquisition.ErrKnownRecoveryConflict {
		t.Fatalf("mismatched CopyOperation recovery error = %v", err)
	}
}

func TestPostgresKnownRecoveryResumesCopyOperationUnknown(t *testing.T) {
	ctx, pool, recovery, request := seedKnownRecovery(t)
	if _, err := pool.Exec(ctx, `DELETE FROM acquisition_provider_tasks WHERE manifest_id=$1`, string(request.Scope.ManifestID)); err != nil {
		t.Fatal(err)
	}
	copies, err := NewAcquisitionCopyRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := copies.SaveResultObjectEvidence(ctx, acquisition.ResultObjectEvidence{
		ManifestID: request.Scope.ManifestID, ProviderConnectionID: request.Scope.ConnectionID,
		ProviderObjectRef: "donor-file", ProviderParentScope: "donor-dir", ResultName: "result.bin",
		SizeBytes: 42, HashAlgorithm: "SHA1", ContentHash: "0123456789abcdef0123456789abcdef01234567", ObservedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	operation := acquisition.CopyOperation{
		ID: acquisition.CopyOperationID(adminUUID(9001)), ManifestID: request.Scope.ManifestID, JobID: request.Scope.JobID,
		DonorManifestID: request.Scope.ManifestID, ProviderConnectionID: storage.ConnectionID(request.Scope.ConnectionID),
		RequestDigest: bytes.Repeat([]byte{1}, 32), DestinationBefore: []string{}, Status: acquisition.CopyProviderUnknown,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := copies.CreateCopyOperation(ctx, operation); err != nil {
		t.Fatal(err)
	}
	result, err := recovery.ResumeKnownAcquisition(ctx, request)
	if err != nil || !result.Changed || result.Job.State != "RETRY_WAIT" {
		t.Fatalf("resume result = %#v, error = %v", result, err)
	}
	var taskCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM acquisition_provider_tasks WHERE manifest_id=$1`, string(request.Scope.ManifestID)).Scan(&taskCount); err != nil || taskCount != 0 {
		t.Fatalf("resume created provider task: count=%d error=%v", taskCount, err)
	}
}
