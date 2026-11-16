package product

import (
	"context"
	"errors"
	"strings"

	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

type PrivateResourceAuthorizer interface {
	Allowed(context.Context, auth.Principal, storage.BindingID, string) (bool, error)
}

type CloudSpaceResourceAuthorizer struct {
	Spaces CloudUserSpaceStore
}

func (authorizer CloudSpaceResourceAuthorizer) Allowed(ctx context.Context, principal auth.Principal, bindingID storage.BindingID, resourcePath string) (bool, error) {
	if !strings.Contains(resourcePath, "/_users/") {
		return true, nil
	}
	space, err := authorizer.Spaces.GetSpaceByOwner(ctx, string(principal.UserID), bindingID)
	if errors.Is(err, ErrUserSpaceNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if space.Status != UserSpaceActive || space.OwnerState != UserSpaceOwnerActive {
		return false, nil
	}
	return resourcePath == space.CanonicalPrefix || strings.HasPrefix(resourcePath, space.CanonicalPrefix+"/"), nil
}

func IsPrivateCloudPath(resourcePath string) bool {
	return strings.Contains(resourcePath, "/_users/")
}
