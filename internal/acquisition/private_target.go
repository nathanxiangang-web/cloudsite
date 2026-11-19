package acquisition

import (
	"context"

	"github.com/nathanxiangang-web/panta/internal/storage"
)

const (
	PrivateTargetStatusActive = "ACTIVE"
	PrivateTargetOwnerActive  = "ACTIVE"
)

type PrivateTargetSpace struct {
	ID                   string
	OwnerUserID          string
	BindingID            storage.BindingID
	ProviderConnectionID storage.ConnectionID
	IndexCoreRootID      string
	NativeDirID          string
	CanonicalPrefix      string
	Status               string
	OwnerState           string
}

type PrivateTargetSpaceReader interface {
	GetPrivateTargetSpace(context.Context, string) (PrivateTargetSpace, error)
}

func ValidatePrivateTargetSpace(space PrivateTargetSpace, spaceID string, binding storage.Binding,
	connection storage.Connection, scope, targetPath string, owner *UserID) error {
	if owner == nil || *owner == "" || space.ID == "" || space.ID != spaceID ||
		space.OwnerUserID != string(*owner) || space.BindingID != binding.ID ||
		space.ProviderConnectionID != connection.ID || space.IndexCoreRootID != binding.IndexCoreRootID ||
		space.NativeDirID == "" || space.NativeDirID != scope || space.CanonicalPrefix != targetPath ||
		space.Status != PrivateTargetStatusActive || space.OwnerState != PrivateTargetOwnerActive {
		return ErrExecutionIdentityMismatch
	}
	return nil
}
