package postgres

import (
	"bytes"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/nathanxiangang-web/panta/internal/acquisition"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

func TestPostgresCopyAndStartShareOneSideEffectFence(t *testing.T) {
	fixture := newAdmissionFixture(t)
	donor, err := fixture.service.Admit(fixture.ctx, fixture.p, fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	request := fixture.input
	request.OperationKey = "requester-copy"
	requester, err := fixture.service.Admit(fixture.ctx, fixture.other, request)
	if err != nil {
		t.Fatal(err)
	}
	copies, err := NewAcquisitionCopyRepository(fixture.pool)
	if err != nil {
		t.Fatal(err)
	}
	evidence := acquisition.ResultObjectEvidence{
		ManifestID: donor.ManifestID, ProviderConnectionID: storage.ConnectionID(fixture.input.BindingID),
		ProviderObjectRef: "donor-file", ProviderParentScope: "donor-directory", ResultName: "result.bin",
		SizeBytes: 42, HashAlgorithm: "SHA1", ContentHash: "0123456789abcdef0123456789abcdef01234567", ObservedAt: time.Now(),
	}
	if err := copies.SaveResultObjectEvidence(fixture.ctx, evidence); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	operation := acquisition.CopyOperation{
		ID: acquisition.CopyOperationID(adminUUID(7201)), ManifestID: requester.ManifestID, JobID: requester.JobID,
		DonorManifestID: donor.ManifestID, ProviderConnectionID: storage.ConnectionID(fixture.input.BindingID),
		RequestDigest: bytes.Repeat([]byte{1}, 32), DestinationBefore: []string{}, Status: acquisition.CopyReserved,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := copies.CreateCopyOperation(fixture.ctx, operation); err != nil {
		t.Fatal(err)
	}
	starts, err := NewAcquisitionProviderTaskRepository(fixture.pool)
	if err != nil {
		t.Fatal(err)
	}
	_, err = starts.ClaimProviderTask(fixture.ctx, acquisition.ProviderTaskClaimRequest{
		ManifestID: requester.ManifestID, JobID: requester.JobID, ProviderID: "115", Now: now,
	})
	if !errors.Is(err, acquisition.ErrProviderTaskConflict) {
		t.Fatalf("start after copy error = %v, want ErrProviderTaskConflict", err)
	}
	request.OperationKey = "requester-start"
	started, err := fixture.service.Admit(fixture.ctx, fixture.p, request)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := starts.ClaimProviderTask(fixture.ctx, acquisition.ProviderTaskClaimRequest{
		ManifestID: started.ManifestID, JobID: started.JobID, ProviderID: "115", Now: now,
	})
	if err != nil || !claim.ClaimedStart {
		t.Fatalf("initial start = %#v, error = %v", claim, err)
	}
	operation.ID = acquisition.CopyOperationID(adminUUID(7202))
	operation.ManifestID = started.ManifestID
	operation.JobID = started.JobID
	if err := copies.CreateCopyOperation(fixture.ctx, operation); !errors.Is(err, acquisition.ErrCopyOperationConflict) {
		t.Fatalf("copy after start error = %v, want ErrCopyOperationConflict", err)
	}
}

func TestPostgresCopyAndStartConcurrentHaveOneWinner(t *testing.T) {
	fixture := newAdmissionFixture(t)
	donor, err := fixture.service.Admit(fixture.ctx, fixture.p, fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	request := fixture.input
	request.OperationKey = "concurrent-requester"
	requester, err := fixture.service.Admit(fixture.ctx, fixture.other, request)
	if err != nil {
		t.Fatal(err)
	}
	copies, _ := NewAcquisitionCopyRepository(fixture.pool)
	starts, _ := NewAcquisitionProviderTaskRepository(fixture.pool)
	evidence := acquisition.ResultObjectEvidence{
		ManifestID: donor.ManifestID, ProviderConnectionID: storage.ConnectionID(fixture.input.BindingID),
		ProviderObjectRef: "donor-file", ProviderParentScope: "donor-directory", ResultName: "result.bin",
		SizeBytes: 42, HashAlgorithm: "SHA1", ContentHash: "0123456789abcdef0123456789abcdef01234567", ObservedAt: time.Now(),
	}
	if err := copies.SaveResultObjectEvidence(fixture.ctx, evidence); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	operation := acquisition.CopyOperation{
		ID: acquisition.CopyOperationID(adminUUID(7203)), ManifestID: requester.ManifestID, JobID: requester.JobID,
		DonorManifestID: donor.ManifestID, ProviderConnectionID: storage.ConnectionID(fixture.input.BindingID),
		RequestDigest: bytes.Repeat([]byte{2}, 32), DestinationBefore: []string{}, Status: acquisition.CopyReserved,
		CreatedAt: now, UpdatedAt: now,
	}
	var wait sync.WaitGroup
	var copyErr, startErr error
	wait.Add(2)
	go func() {
		defer wait.Done()
		copyErr = copies.CreateCopyOperation(fixture.ctx, operation)
	}()
	go func() {
		defer wait.Done()
		_, startErr = starts.ClaimProviderTask(fixture.ctx, acquisition.ProviderTaskClaimRequest{
			ManifestID: requester.ManifestID, JobID: requester.JobID, ProviderID: "115", Now: now,
		})
	}()
	wait.Wait()
	successes := 0
	if copyErr == nil {
		successes++
	}
	if startErr == nil {
		successes++
	}
	if successes != 1 {
		t.Fatalf("copy error = %v, start error = %v, successes = %d", copyErr, startErr, successes)
	}
	var copyCount, startCount int
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT count(*) FROM acquisition_copy_operations").Scan(&copyCount); err != nil {
		t.Fatal(err)
	}
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT count(*) FROM acquisition_provider_tasks").Scan(&startCount); err != nil {
		t.Fatal(err)
	}
	if copyCount+startCount != 1 {
		t.Fatalf("copy rows = %d, start rows = %d", copyCount, startCount)
	}
}
