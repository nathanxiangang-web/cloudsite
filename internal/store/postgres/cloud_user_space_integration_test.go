package postgres

import (
	"errors"
	"testing"
	"time"

	"github.com/nathanxiangang-web/panta/internal/adminops"
	"github.com/nathanxiangang-web/panta/internal/product"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

func TestPostgresCloudUserSpaceRoundTripCASAndNativeUniqueness(t *testing.T) {
	fixture := newAdmissionFixture(t)
	repository, err := NewCloudUserSpaceRepository(fixture.pool)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	space := product.CloudUserSpace{
		ID: product.UserSpaceID(adminUUID(7001)), OwnerUserID: string(fixture.p.UserID),
		BindingID: fixture.input.BindingID, ProviderConnectionID: storage.ConnectionID(fixture.input.BindingID),
		IndexCoreRootID: "admission-root", NativeParentID: "native-parent",
		CanonicalPrefix: "/云下载/_users/user__uuid", DirectoryName: "user__uuid",
		Status: product.UserSpaceProvisioning, OwnerState: product.UserSpaceOwnerActive,
		ProvisioningKey: "provision-key", Version: 0, CreatedAt: now, UpdatedAt: now,
	}
	if err := repository.CreateSpace(fixture.ctx, space); err != nil {
		t.Fatal(err)
	}
	if err := repository.CreateSpace(fixture.ctx, space); !errors.Is(err, product.ErrUserSpaceConflict) {
		t.Fatalf("duplicate error = %v, want ErrUserSpaceConflict", err)
	}
	got, err := repository.GetSpaceByOwner(fixture.ctx, space.OwnerUserID, space.BindingID)
	if err != nil || got.Status != product.UserSpaceProvisioning || got.NativeDirID != nil {
		t.Fatalf("space = %#v, error = %v", got, err)
	}
	directoryID := "native-directory"
	space.NativeDirID = &directoryID
	space.Status = product.UserSpaceActive
	space.Version = 1
	space.UpdatedAt = now.Add(time.Second)
	if err := repository.UpdateSpace(fixture.ctx, space, 0); err != nil {
		t.Fatal(err)
	}
	got, err = repository.GetSpace(fixture.ctx, space.ID)
	if err != nil || got.Status != product.UserSpaceActive || got.Version != 1 || got.ProvisioningKey != space.ProvisioningKey {
		t.Fatalf("updated space = %#v, error = %v", got, err)
	}
	space.OwnerState = product.UserSpaceOwnerDeleted
	space.Status = product.UserSpaceUnavailable
	space.Version = 2
	if err := repository.UpdateSpace(fixture.ctx, space, 1); !errors.Is(err, product.ErrUserSpaceConflict) {
		t.Fatalf("owner resurrection error = %v, want ErrUserSpaceConflict", err)
	}
	otherBinding := storage.BindingID(adminUUID(7002))
	if _, err := fixture.pool.Exec(fixture.ctx, `INSERT INTO storage_bindings(
storage_binding_id, storage_connection_id, openlist_mount_path, indexcore_root_id, status, created_at, updated_at
) VALUES($1,$2,'/other-root','other-root','ACTIVE',$3,$3)`, string(otherBinding), string(fixture.input.BindingID), now); err != nil {
		t.Fatal(err)
	}
	other := space
	other.ID = product.UserSpaceID(adminUUID(7003))
	other.OwnerUserID = string(fixture.other.UserID)
	other.BindingID = otherBinding
	other.OwnerState = product.UserSpaceOwnerActive
	other.Status = product.UserSpaceActive
	other.Version = 0
	other.CanonicalPrefix = "/云下载/_users/other__uuid"
	other.DirectoryName = "other__uuid"
	if err := repository.CreateSpace(fixture.ctx, other); !errors.Is(err, product.ErrUserSpaceConflict) {
		t.Fatalf("same connection native directory was reused: %v", err)
	}
}

func TestPostgresAdminDeleteUserMarksSpaceOwnerDeleted(t *testing.T) {
	ctx, pool, _, actor, _, operations := opFixture(t)
	service, _ := adminops.NewService(operations, nil, []string{adminBinding})
	created := opRun(t, service, actor, adminops.Command{RequestID: opID(7004), Action: adminops.CreateUser, Login: "space-owner", Password: "synthetic-password"})
	spaces, err := NewCloudUserSpaceRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	directoryID := "owner-native-directory"
	space := product.CloudUserSpace{
		ID: product.UserSpaceID(adminUUID(7005)), OwnerUserID: created.TargetID,
		BindingID: adminBinding, ProviderConnectionID: storage.ConnectionID(adminBinding),
		IndexCoreRootID: "admin-root", NativeParentID: "native-parent", NativeDirID: &directoryID,
		CanonicalPrefix: "/云下载/_users/space-owner__uuid", DirectoryName: "space-owner__uuid",
		Status: product.UserSpaceActive, OwnerState: product.UserSpaceOwnerActive,
		ProvisioningKey: "provision-key", Version: 0, CreatedAt: now, UpdatedAt: now,
	}
	if err := spaces.CreateSpace(ctx, space); err != nil {
		t.Fatal(err)
	}
	result := opRun(t, service, actor, adminops.Command{RequestID: opID(7006), Action: adminops.DeleteUser, TargetID: created.TargetID})
	if result.Status != "SUCCEEDED" {
		t.Fatalf("delete result = %#v", result)
	}
	got, err := spaces.GetSpace(ctx, space.ID)
	if err != nil || got.OwnerState != product.UserSpaceOwnerDeleted || got.Status != product.UserSpaceUnavailable || got.NativeDirID == nil || *got.NativeDirID != directoryID {
		t.Fatalf("space after account deletion = %#v, error = %v", got, err)
	}
}
