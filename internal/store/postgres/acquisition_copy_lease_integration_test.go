package postgres

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/nathanxiangang-web/panta/internal/acquisition"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

func TestPostgresCopyReservationRequiresCurrentLease(t *testing.T) {
	fixture := newAdmissionFixture(t)
	donor, err := fixture.service.Admit(fixture.ctx, fixture.p, fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	request := fixture.input
	request.OperationKey = "lease-requester"
	requester, err := fixture.service.Admit(fixture.ctx, fixture.other, request)
	if err != nil {
		t.Fatal(err)
	}
	repository, err := NewAcquisitionCopyRepository(fixture.pool)
	if err != nil {
		t.Fatal(err)
	}
	evidence := acquisition.ResultObjectEvidence{
		ManifestID: donor.ManifestID, ProviderConnectionID: storage.ConnectionID(fixture.input.BindingID),
		ProviderObjectRef: "donor-file", ProviderParentScope: "donor-directory", ResultName: "result.bin",
		SizeBytes: 42, HashAlgorithm: "SHA1", ContentHash: "0123456789abcdef0123456789abcdef01234567", ObservedAt: time.Now(),
	}
	if err := repository.SaveResultObjectEvidence(fixture.ctx, evidence); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if _, err := fixture.pool.Exec(fixture.ctx, `UPDATE jobs SET state='RUNNING', lease_owner='worker-a', claim_attempts=1, lease_expires_at=$2 WHERE job_id=$1`, string(requester.JobID), now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	operation := acquisition.CopyOperation{
		ID: acquisition.CopyOperationID(adminUUID(7301)), ManifestID: requester.ManifestID, JobID: requester.JobID,
		DonorManifestID: donor.ManifestID, ProviderConnectionID: storage.ConnectionID(fixture.input.BindingID),
		RequestDigest: bytes.Repeat([]byte{3}, 32), DestinationBefore: []string{}, Status: acquisition.CopyReserved,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := repository.CreateCopyOperationWithLease(fixture.ctx, operation, "worker-a", 1); err != nil {
		t.Fatal(err)
	}
	request.OperationKey = "stale-requester"
	stale, err := fixture.service.Admit(fixture.ctx, fixture.p, request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.pool.Exec(fixture.ctx, `UPDATE jobs SET state='RUNNING', lease_owner='worker-a', claim_attempts=2, lease_expires_at=$2 WHERE job_id=$1`, string(stale.JobID), now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	operation.ID = acquisition.CopyOperationID(adminUUID(7302))
	operation.ManifestID = stale.ManifestID
	operation.JobID = stale.JobID
	if err := repository.CreateCopyOperationWithLease(fixture.ctx, operation, "worker-a", 1); !errors.Is(err, acquisition.ErrCopyOperationConflict) {
		t.Fatalf("stale lease error = %v, want ErrCopyOperationConflict", err)
	}
	var count int
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT count(*) FROM acquisition_copy_operations").Scan(&count); err != nil || count != 1 {
		t.Fatalf("copy operation count = %d, error = %v", count, err)
	}
}
