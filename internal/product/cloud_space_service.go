package product

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/nathanxiangang-web/panta/internal/acquisition"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

var (
	ErrCloudDirectoryUnknown   = errors.New("cloud directory outcome is unknown")
	ErrCloudDirectoryAmbiguous = errors.New("cloud directory is ambiguous")
	ErrCloudSpaceUnavailable   = errors.New("cloud user space is unavailable")
)

type CloudSpaceTarget struct {
	BindingID            storage.BindingID
	ProviderConnectionID storage.ConnectionID
	IndexCoreRootID      string
	NativeParentID       string
	CanonicalBase        string
}

func (target CloudSpaceTarget) Valid() bool {
	if target.BindingID == "" || target.ProviderConnectionID == "" || strings.TrimSpace(target.IndexCoreRootID) == "" ||
		storage.ValidateProviderScope(&target.NativeParentID) != nil {
		return false
	}
	base, err := acquisition.NormalizeTargetPath(target.CanonicalBase)
	return err == nil && base == target.CanonicalBase && base != "/"
}

type CloudDirectoryMatch struct {
	DirectoryID string
	ParentID    string
	Name        string
	Found       bool
}

type CloudDirectoryProvider interface {
	FindDirectory(context.Context, string, string) (CloudDirectoryMatch, error)
	CreateDirectory(context.Context, string, string) (string, error)
}

type CloudSpaceDTO struct {
	SpaceID         UserSpaceID     `json:"space_id,omitempty"`
	Status          UserSpaceStatus `json:"status"`
	CanonicalPrefix string          `json:"canonical_prefix"`
	DirectoryName   string          `json:"directory_name"`
	Version         int64           `json:"version"`
}

type CloudSpaceService struct {
	spaces     CloudUserSpaceStore
	provider   CloudDirectoryProvider
	now        func() time.Time
	newSpaceID func() (UserSpaceID, error)
}

func NewCloudSpaceService(spaces CloudUserSpaceStore, provider CloudDirectoryProvider) (*CloudSpaceService, error) {
	if spaces == nil || provider == nil {
		return nil, ErrInvalidUserSpace
	}
	return &CloudSpaceService{spaces: spaces, provider: provider, now: time.Now, newSpaceID: func() (UserSpaceID, error) {
		id, err := auth.NewUserID()
		return UserSpaceID(id), err
	}}, nil
}

func (service *CloudSpaceService) GetSpace(ctx context.Context, principal auth.Principal, target CloudSpaceTarget) (CloudSpaceDTO, error) {
	if !principal.Valid() || !target.Valid() {
		return CloudSpaceDTO{}, ErrInvalidUserSpace
	}
	space, err := service.spaces.GetSpaceByOwner(ctx, string(principal.UserID), target.BindingID)
	if errors.Is(err, ErrUserSpaceNotFound) {
		_, prefix, name := cloudSpaceNames(principal, target)
		return CloudSpaceDTO{Status: UserSpaceNotProvisioned, CanonicalPrefix: prefix, DirectoryName: name}, nil
	}
	if err != nil {
		return CloudSpaceDTO{}, err
	}
	return cloudSpaceDTO(space), nil
}

func (service *CloudSpaceService) ActiveSpace(ctx context.Context, principal auth.Principal, target CloudSpaceTarget) (CloudUserSpace, error) {
	if !principal.Valid() || !target.Valid() {
		return CloudUserSpace{}, ErrInvalidUserSpace
	}
	space, err := service.spaces.GetSpaceByOwner(ctx, string(principal.UserID), target.BindingID)
	if err != nil {
		return CloudUserSpace{}, err
	}
	if space.Status != UserSpaceActive || space.OwnerState != UserSpaceOwnerActive || space.NativeDirID == nil {
		return CloudUserSpace{}, ErrCloudSpaceUnavailable
	}
	return space, nil
}

func (service *CloudSpaceService) Provision(ctx context.Context, principal auth.Principal, target CloudSpaceTarget, operationKey string) (CloudSpaceDTO, error) {
	if !principal.Valid() || !target.Valid() || !operationKeyPattern.MatchString(operationKey) {
		return CloudSpaceDTO{}, ErrInvalidUserSpace
	}
	space, err := service.spaces.GetSpaceByOwner(ctx, string(principal.UserID), target.BindingID)
	if errors.Is(err, ErrUserSpaceNotFound) {
		_, prefix, name := cloudSpaceNames(principal, target)
		now := service.now().UTC()
		id, idErr := service.newSpaceID()
		if idErr != nil {
			return CloudSpaceDTO{}, idErr
		}
		space = CloudUserSpace{
			ID: id, OwnerUserID: string(principal.UserID), BindingID: target.BindingID,
			ProviderConnectionID: target.ProviderConnectionID, IndexCoreRootID: target.IndexCoreRootID,
			NativeParentID: target.NativeParentID, CanonicalPrefix: prefix, DirectoryName: name,
			Status: UserSpaceProvisioning, OwnerState: UserSpaceOwnerActive, ProvisioningKey: operationKey,
			CreatedAt: now, UpdatedAt: now,
		}
		if err := service.spaces.CreateSpace(ctx, space); err != nil {
			return CloudSpaceDTO{}, err
		}
	} else if err != nil {
		return CloudSpaceDTO{}, err
	} else {
		if space.ProvisioningKey != operationKey && space.Status != UserSpaceActive {
			return CloudSpaceDTO{}, ErrUserSpaceConflict
		}
		if space.Status == UserSpaceActive && space.OwnerState == UserSpaceOwnerActive {
			return cloudSpaceDTO(space), nil
		}
		if space.Status == UserSpaceBlocked || space.Status == UserSpaceUnavailable || space.OwnerState != UserSpaceOwnerActive {
			return CloudSpaceDTO{}, ErrCloudSpaceUnavailable
		}
		if space.Status == UserSpaceProvisioning || space.Status == UserSpaceReconcileRequired {
			return service.reconcile(ctx, space)
		}
	}
	match, err := service.provider.FindDirectory(ctx, space.NativeParentID, space.DirectoryName)
	if err == nil && match.Found {
		return service.reconcile(ctx, space)
	}
	if err != nil && !errors.Is(err, ErrCloudDirectoryUnknown) {
		return CloudSpaceDTO{}, err
	}
	directoryID, createErr := service.provider.CreateDirectory(ctx, space.NativeParentID, space.DirectoryName)
	if createErr != nil || directoryID == "" {
		space.Status = UserSpaceReconcileRequired
		space.Version++
		space.UpdatedAt = service.now().UTC()
		if err := service.spaces.UpdateSpace(ctx, space, space.Version-1); err != nil {
			return CloudSpaceDTO{}, err
		}
		return service.reconcile(ctx, space)
	}
	return service.activateSpace(ctx, space, directoryID)
}

func (service *CloudSpaceService) reconcile(ctx context.Context, space CloudUserSpace) (CloudSpaceDTO, error) {
	match, err := service.provider.FindDirectory(ctx, space.NativeParentID, space.DirectoryName)
	if err != nil {
		return cloudSpaceDTO(space), err
	}
	if !match.Found {
		return cloudSpaceDTO(space), nil
	}
	if match.ParentID != space.NativeParentID || match.Name != space.DirectoryName || match.DirectoryID == "" {
		space.Status = UserSpaceReconcileRequired
		space.Version++
		space.UpdatedAt = service.now().UTC()
		if err := service.spaces.UpdateSpace(ctx, space, space.Version-1); err != nil {
			return CloudSpaceDTO{}, err
		}
		return cloudSpaceDTO(space), ErrCloudDirectoryAmbiguous
	}
	return service.activateSpace(ctx, space, match.DirectoryID)
}

func (service *CloudSpaceService) activateSpace(ctx context.Context, space CloudUserSpace, directoryID string) (CloudSpaceDTO, error) {
	now := service.now().UTC()
	space.NativeDirID = &directoryID
	space.Status = UserSpaceActive
	space.OwnerState = UserSpaceOwnerActive
	space.Version++
	space.LastVerifiedAt = &now
	space.UpdatedAt = now
	if err := service.spaces.UpdateSpace(ctx, space, space.Version-1); err != nil {
		return CloudSpaceDTO{}, err
	}
	return cloudSpaceDTO(space), nil
}

func cloudSpaceDTO(space CloudUserSpace) CloudSpaceDTO {
	return CloudSpaceDTO{SpaceID: space.ID, Status: space.Status, CanonicalPrefix: space.CanonicalPrefix,
		DirectoryName: space.DirectoryName, Version: space.Version}
}

func cloudSpaceNames(principal auth.Principal, target CloudSpaceTarget) (string, string, string) {
	login, _ := auth.NormalizeLogin(principal.Login)
	var safe strings.Builder
	for _, value := range login {
		if value >= 'a' && value <= 'z' || value >= '0' && value <= '9' || value == '.' || value == '_' || value == '-' {
			safe.WriteRune(value)
		} else {
			safe.WriteRune('_')
		}
	}
	name := safe.String() + "__" + string(principal.UserID)
	return target.NativeParentID, target.CanonicalBase + "/_users/" + name, name
}
