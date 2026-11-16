package postgres

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/nathanxiangang-web/panta/internal/acquisition"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

func TestPostgresAcquisitionCopyEvidenceAndOperationCAS(t *testing.T) {
	fixture := newAdmissionFixture(t)
	donor, err := fixture.service.Admit(fixture.ctx, fixture.p, fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	request := fixture.input
	request.OperationKey = "requester"
	requester, err := fixture.service.Admit(fixture.ctx, fixture.other, request)
	if err != nil {
		t.Fatal(err)
	}
	repository, err := NewAcquisitionCopyRepository(fixture.pool)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	evidence := acquisition.ResultObjectEvidence{
		ManifestID: donor.ManifestID, ProviderConnectionID: storage.ConnectionID(fixture.input.BindingID),
		ProviderObjectRef: "donor-file", ProviderParentScope: "donor-directory", ResultName: "result.bin",
		SizeBytes: 42, HashAlgorithm: "SHA1", ContentHash: "0123456789abcdef0123456789abcdef01234567", ObservedAt: now,
	}
	if err := repository.SaveResultObjectEvidence(fixture.ctx, evidence); err != nil {
		t.Fatal(err)
	}
	gotEvidence, err := repository.GetResultObjectEvidence(fixture.ctx, evidence.ManifestID)
	if err != nil || gotEvidence.ProviderObjectRef != evidence.ProviderObjectRef || gotEvidence.ContentHash != evidence.ContentHash {
		t.Fatalf("evidence = %#v, error = %v", gotEvidence, err)
	}
	operation := acquisition.CopyOperation{
		ID: acquisition.CopyOperationID(adminUUID(7101)), ManifestID: requester.ManifestID, JobID: requester.JobID,
		DonorManifestID: donor.ManifestID, ProviderConnectionID: storage.ConnectionID(fixture.input.BindingID),
		RequestDigest: bytes.Repeat([]byte{7}, 32), DestinationBefore: []string{}, Status: acquisition.CopyReserved,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := repository.CreateCopyOperation(fixture.ctx, operation); err != nil {
		t.Fatal(err)
	}
	got, err := repository.GetCopyOperation(fixture.ctx, operation.ManifestID)
	if err != nil || got.ID != operation.ID || got.DestinationBefore == nil || len(got.DestinationBefore) != 0 {
		t.Fatalf("operation = %#v, error = %v", got, err)
	}
	resultRef := "copied-file"
	got.Status = acquisition.CopyProviderConfirmed
	got.ProviderResultRef = &resultRef
	got.UpdatedAt = now.Add(time.Second)
	if err := repository.UpdateCopyOperation(fixture.ctx, got, now); err != nil {
		t.Fatal(err)
	}
	got.Status = acquisition.CopyProviderUnknown
	if err := repository.UpdateCopyOperation(fixture.ctx, got, now); !errors.Is(err, acquisition.ErrCopyOperationConflict) {
		t.Fatalf("stale update error = %v, want ErrCopyOperationConflict", err)
	}
}

func TestPostgresAcquisitionCopyRejectsInconsistentState(t *testing.T) {
	fixture := newAdmissionFixture(t)
	repository, err := NewAcquisitionCopyRepository(fixture.pool)
	if err != nil {
		t.Fatal(err)
	}
	operation := acquisition.CopyOperation{
		ID: acquisition.CopyOperationID(adminUUID(7102)), ManifestID: "invalid", JobID: "invalid",
		DonorManifestID: "invalid", ProviderConnectionID: "invalid", RequestDigest: make([]byte, 32),
		DestinationBefore: []string{}, Status: acquisition.CopyProviderConfirmed,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := operation.Validate(); !errors.Is(err, acquisition.ErrInvalidCopyOperation) {
		t.Fatalf("domain validation error = %v", err)
	}
	var count int
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT count(*) FROM acquisition_copy_operations").Scan(&count); err != nil || count != 0 {
		t.Fatalf("invalid operation reached database: count=%d error=%v", count, err)
	}
	_ = repository
}
