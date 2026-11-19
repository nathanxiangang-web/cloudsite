package postgres

import (
	"errors"
	"testing"
	"time"

	"github.com/nathanxiangang-web/panta/internal/acquisition"
	"github.com/nathanxiangang-web/panta/internal/catalog"
	"github.com/nathanxiangang-web/panta/internal/product"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

func TestPostgresFindCopyDonorRequiresReadyPresentEvidence(t *testing.T) {
	fixture := newAdmissionFixture(t)
	donor, err := fixture.service.Admit(fixture.ctx, fixture.p, fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	request := fixture.input
	request.OperationKey = "donor-requester"
	requester, err := fixture.service.Admit(fixture.ctx, fixture.other, request)
	if err != nil {
		t.Fatal(err)
	}
	spaces, err := NewCloudUserSpaceRepository(fixture.pool)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	directoryID := "donor-directory"
	space := product.CloudUserSpace{
		ID: product.UserSpaceID(adminUUID(7401)), OwnerUserID: string(fixture.p.UserID),
		BindingID: fixture.input.BindingID, ProviderConnectionID: storage.ConnectionID(fixture.input.BindingID),
		IndexCoreRootID: "admission-root", NativeParentID: "parent", NativeDirID: &directoryID,
		CanonicalPrefix: "/云下载/_users/donor", DirectoryName: "donor",
		Status: product.UserSpaceActive, OwnerState: product.UserSpaceOwnerActive,
		ProvisioningKey: "provision", Version: 0, CreatedAt: now, UpdatedAt: now,
	}
	if err := spaces.CreateSpace(fixture.ctx, space); err != nil {
		t.Fatal(err)
	}
	catalogRepository, err := NewCatalogRepository(fixture.pool)
	if err != nil {
		t.Fatal(err)
	}
	copyID := catalog.CopyID(adminUUID(7402))
	if err := catalogRepository.CreateCopy(fixture.ctx, catalog.Copy{
		ID: copyID, IndexCoreRootID: "admission-root", IndexCoreResourceID: "donor-resource",
		StorageBindingID: catalog.StorageBindingID(fixture.input.BindingID), Availability: "PRESENT", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	resultName := "result.bin"
	if _, err := fixture.pool.Exec(fixture.ctx, `UPDATE acquisition_manifests SET state='READY', result_copy_id=$2,
user_space_id=$3, target_provider_connection_id=$4, target_provider_scope=$5, result_name=$6 WHERE manifest_id=$1`,
		string(donor.ManifestID), string(copyID), string(space.ID), string(fixture.input.BindingID), directoryID, resultName); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.pool.Exec(fixture.ctx, `UPDATE jobs SET state='SUCCEEDED' WHERE job_id=$1`, string(donor.JobID)); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.pool.Exec(fixture.ctx, `INSERT INTO acquisition_provider_tasks(
manifest_id, job_id, provider_id, provider_task_ref, state, created_at, updated_at
) VALUES($1,$2,'115','donor-task','REFERENCE_KNOWN',$3,$3)`, string(donor.ManifestID), string(donor.JobID), now); err != nil {
		t.Fatal(err)
	}
	copies, err := NewAcquisitionCopyRepository(fixture.pool)
	if err != nil {
		t.Fatal(err)
	}
	evidence := acquisition.ResultObjectEvidence{
		ManifestID: donor.ManifestID, ProviderConnectionID: storage.ConnectionID(fixture.input.BindingID),
		ProviderObjectRef: "donor-file", ProviderParentScope: directoryID, ResultName: resultName,
		SizeBytes: 42, HashAlgorithm: "SHA1", ContentHash: "0123456789abcdef0123456789abcdef01234567", ObservedAt: now,
	}
	if err := copies.SaveResultObjectEvidence(fixture.ctx, evidence); err != nil {
		t.Fatal(err)
	}
	donors, err := NewAcquisitionCopyDonorRepository(fixture.pool)
	if err != nil {
		t.Fatal(err)
	}
	donorManifest, err := fixture.manifests.GetManifest(fixture.ctx, donor.ManifestID)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := donors.FindCopyDonor(fixture.ctx, donorManifest.SourceType, donorManifest.SourceRef,
		storage.ConnectionID(fixture.input.BindingID), requester.ManifestID)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.Manifest.ID != donor.ManifestID || candidate.Task.ProviderTaskRef != "donor-task" ||
		candidate.Evidence.ProviderObjectRef != "donor-file" || candidate.Job.State != "SUCCEEDED" {
		t.Fatalf("candidate = %#v", candidate)
	}
	if _, err := fixture.pool.Exec(fixture.ctx, `UPDATE copies SET availability='REMOVED' WHERE copy_id=$1`, string(copyID)); err != nil {
		t.Fatal(err)
	}
	if _, err := donors.FindCopyDonor(fixture.ctx, donorManifest.SourceType, donorManifest.SourceRef,
		storage.ConnectionID(fixture.input.BindingID), requester.ManifestID); !errors.Is(err, acquisition.ErrCopyDonorUnavailable) {
		t.Fatalf("removed donor error = %v, want ErrCopyDonorUnavailable", err)
	}
}
