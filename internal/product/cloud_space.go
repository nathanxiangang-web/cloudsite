package product

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nathanxiangang-web/panta/internal/storage"
)

type UserSpaceID string
type UserSpaceStatus string
type UserSpaceOwnerState string

const (
	UserSpaceNotProvisioned    UserSpaceStatus     = "NOT_PROVISIONED"
	UserSpaceProvisioning      UserSpaceStatus     = "PROVISIONING"
	UserSpaceActive            UserSpaceStatus     = "ACTIVE"
	UserSpaceReconcileRequired UserSpaceStatus     = "RECONCILE_REQUIRED"
	UserSpaceBlocked           UserSpaceStatus     = "BLOCKED"
	UserSpaceUnavailable       UserSpaceStatus     = "UNAVAILABLE"
	UserSpaceOwnerActive       UserSpaceOwnerState = "ACTIVE"
	UserSpaceOwnerDeleted      UserSpaceOwnerState = "OWNER_DELETED"
)

var (
	ErrInvalidUserSpace  = errors.New("invalid cloud user space")
	ErrUserSpaceNotFound = errors.New("cloud user space not found")
	ErrUserSpaceConflict = errors.New("cloud user space conflict")
)

type CloudUserSpaceStore interface {
	CreateSpace(context.Context, CloudUserSpace) error
	GetSpace(context.Context, UserSpaceID) (CloudUserSpace, error)
	GetSpaceByOwner(context.Context, string, storage.BindingID) (CloudUserSpace, error)
	UpdateSpace(context.Context, CloudUserSpace, int64) error
}

type CloudUserSpace struct {
	ID                   UserSpaceID
	OwnerUserID          string
	BindingID            storage.BindingID
	ProviderConnectionID storage.ConnectionID
	IndexCoreRootID      string
	NativeParentID       string
	NativeDirID          *string
	CanonicalPrefix      string
	DirectoryName        string
	Status               UserSpaceStatus
	OwnerState           UserSpaceOwnerState
	ProvisioningKey      string
	Version              int64
	LastVerifiedAt       *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

func (status UserSpaceStatus) Valid() bool {
	switch status {
	case UserSpaceNotProvisioned, UserSpaceProvisioning, UserSpaceActive,
		UserSpaceReconcileRequired, UserSpaceBlocked, UserSpaceUnavailable:
		return true
	default:
		return false
	}
}

func (state UserSpaceOwnerState) Valid() bool {
	return state == UserSpaceOwnerActive || state == UserSpaceOwnerDeleted
}

func (space CloudUserSpace) Validate() error {
	if space.ID == "" || !validUserSpaceText(space.OwnerUserID, 128) || space.BindingID == "" ||
		space.ProviderConnectionID == "" || !validUserSpaceText(space.IndexCoreRootID, 256) ||
		!validUserSpaceText(space.NativeParentID, storage.MaxProviderScopeLength) ||
		!validUserSpaceText(space.DirectoryName, 512) || !validUserSpaceDirectoryName(space.DirectoryName) ||
		!space.Status.Valid() || !space.OwnerState.Valid() ||
		!validUserSpaceText(space.ProvisioningKey, 128) || space.Version < 0 ||
		space.CreatedAt.IsZero() || space.UpdatedAt.IsZero() || space.UpdatedAt.Before(space.CreatedAt) {
		return ErrInvalidUserSpace
	}
	if err := storage.ValidateProviderScope(&space.NativeParentID); err != nil {
		return ErrInvalidUserSpace
	}
	if space.NativeDirID != nil && storage.ValidateProviderScope(space.NativeDirID) != nil {
		return ErrInvalidUserSpace
	}
	if space.Status == UserSpaceActive && space.NativeDirID == nil {
		return ErrInvalidUserSpace
	}
	prefix, err := storage.NormalizeMountPath(space.CanonicalPrefix)
	if err != nil || prefix != space.CanonicalPrefix || prefix == "/" || utf8.RuneCountInString(prefix) > 2048 {
		return ErrInvalidUserSpace
	}
	return nil
}

func validUserSpaceText(value string, maximum int) bool {
	return value != "" && strings.TrimSpace(value) == value && utf8.ValidString(value) &&
		!strings.ContainsRune(value, '\x00') && utf8.RuneCountInString(value) <= maximum
}
func validUserSpaceDirectoryName(value string) bool {
	return strings.TrimSpace(value) != "" && value != "." && value != ".." && !strings.ContainsAny(value, `/\\`)
}
