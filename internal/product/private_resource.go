package product

import (
	"context"
	"errors"
	"strings"

	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/cloudpath"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

type PrivateResourceAuthorizer interface {
	Allowed(context.Context, auth.Principal, storage.BindingID, string, string) (bool, error)
	PrivateScope(context.Context, auth.Principal, storage.BindingID, string) (string, error)
}

type CloudSpaceResourceAuthorizer struct {
	Spaces CloudUserSpaceStore
}

func (authorizer CloudSpaceResourceAuthorizer) Allowed(ctx context.Context, principal auth.Principal, bindingID storage.BindingID, rootID, resourcePath string) (bool, error) {
	if !cloudpath.IsPrivate(resourcePath) {
		return true, nil
	}
	prefix, err := authorizer.PrivateScope(ctx, principal, bindingID, rootID)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return resourcePath == prefix || strings.HasPrefix(resourcePath, prefix+"/"), nil
}

func IsPrivateCloudPath(resourcePath string) bool {
	return cloudpath.IsPrivate(resourcePath)
}
