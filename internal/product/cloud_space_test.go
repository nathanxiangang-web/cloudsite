package product

import (
	"errors"
	"testing"
	"time"
)

func validCloudUserSpace() CloudUserSpace {
	directoryID := "native-directory-id"
	return CloudUserSpace{
		ID: "space-id", OwnerUserID: "owner-id", BindingID: "binding-id",
		ProviderConnectionID: "connection-id", IndexCoreRootID: "root-id",
		NativeParentID: "native-parent-id", NativeDirID: &directoryID,
		CanonicalPrefix: "/云下载/_users/user__uuid", DirectoryName: "user__uuid",
		Status: UserSpaceActive, OwnerState: UserSpaceOwnerActive,
		ProvisioningKey: "provision-key", Version: 1,
		CreatedAt: time.Unix(100, 0), UpdatedAt: time.Unix(100, 0),
	}
}

func TestCloudUserSpaceValidate(t *testing.T) {
	space := validCloudUserSpace()
	if err := space.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*CloudUserSpace){
		"missing active directory": func(value *CloudUserSpace) { value.NativeDirID = nil },
		"root prefix":              func(value *CloudUserSpace) { value.CanonicalPrefix = "/" },
		"trailing slash":           func(value *CloudUserSpace) { value.CanonicalPrefix += "/" },
		"directory separator":      func(value *CloudUserSpace) { value.DirectoryName = "nested/name" },
		"invalid status":           func(value *CloudUserSpace) { value.Status = "READY" },
		"invalid owner state":      func(value *CloudUserSpace) { value.OwnerState = "MISSING" },
		"negative version":         func(value *CloudUserSpace) { value.Version = -1 },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := space
			mutate(&candidate)
			if err := candidate.Validate(); !errors.Is(err, ErrInvalidUserSpace) {
				t.Fatalf("error = %v, want ErrInvalidUserSpace", err)
			}
		})
	}
}

func TestCloudUserSpaceAllowsDeletedOwnerHistory(t *testing.T) {
	space := validCloudUserSpace()
	space.OwnerState = UserSpaceOwnerDeleted
	space.Status = UserSpaceUnavailable
	if err := space.Validate(); err != nil {
		t.Fatal(err)
	}
}
