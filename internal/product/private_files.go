package product

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"

	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/cloudpath"
	"github.com/nathanxiangang-web/panta/internal/integrations/indexcore"
	"github.com/nathanxiangang-web/panta/internal/resourceview"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

type PrivateFileReader interface {
	PrivateFiles(context.Context, auth.Principal, PrivateFilesRequest) (ResourcePage, error)
}

type PrivateFilesRequest struct {
	BindingID storage.BindingID
	Folder    string
	Cursor    string
	Limit     int
}

type privateFileCursor struct {
	Owner  string `json:"owner"`
	Prefix string `json:"prefix"`
	Parent string `json:"parent"`
	Inner  string `json:"inner"`
}

func (s *ReadService) PrivateFiles(ctx context.Context, principal auth.Principal, request PrivateFilesRequest) (ResourcePage, error) {
	if s == nil || missing(s.privateResources) {
		return ResourcePage{}, ErrUnavailable
	}
	if request.Limit < 1 || request.Limit > 50 || !ValidOpaque(request.Cursor) || !ValidOpaque(request.Folder) {
		return ResourcePage{}, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, ReadTimeout)
	defer cancel()
	binding, err := s.binding(ctx, principal, request.BindingID)
	if err != nil {
		return ResourcePage{}, err
	}
	prefix, err := s.privateResources.PrivateScope(ctx, principal, binding.ID, binding.IndexCoreRootID)
	if err != nil {
		if errors.Is(err, auth.ErrUnauthorized) {
			return ResourcePage{}, err
		}
		return ResourcePage{}, safeReadError(err)
	}
	folder := prefix
	if request.Folder != "" {
		clean, err := storage.NormalizeMountPath(request.Folder)
		if err != nil || clean != request.Folder {
			return ResourcePage{}, ErrInvalid
		}
		if clean != prefix && !strings.HasPrefix(clean, prefix+"/") {
			return ResourcePage{}, ErrNotFound
		}
		folder = clean
	}
	detail, err := s.physical.Resolve(ctx, resourceview.PhysicalRequest{RootID: binding.IndexCoreRootID, Path: folder})
	if err != nil {
		return ResourcePage{}, safeReadError(err)
	}
	if detail.Ambiguous || len(detail.Matches) > 1 {
		return ResourcePage{}, ErrUnavailable
	}
	if len(detail.Matches) == 0 {
		// A provisioned, empty space may not have received its first IndexCore update.
		if folder == prefix && request.Cursor == "" {
			return ResourcePage{Items: []Resource{}}, nil
		}
		return ResourcePage{}, ErrNotFound
	}
	parent := detail.Matches[0].Physical
	if parent.ResourceID == "" || !ValidOpaque(parent.ResourceID) || parent.RootID != binding.IndexCoreRootID || parent.Presence != indexcore.ResourcePresent ||
		parent.CanonicalPath == nil || *parent.CanonicalPath != folder || parent.Directory == nil {
		return ResourcePage{}, ErrUnavailable
	}
	if !*parent.Directory {
		return ResourcePage{}, ErrNotFound
	}
	inner, err := decodePrivateFileCursor(request.Cursor, principal.UserID, folder, parent.ResourceID)
	if err != nil {
		return ResourcePage{}, err
	}
	page, err := s.index.Browse(ctx, indexcore.BrowseRequest{RootID: binding.IndexCoreRootID, ParentResourceID: &parent.ResourceID, Cursor: inner, Limit: request.Limit})
	if err != nil {
		return ResourcePage{}, safeReadError(err)
	}
	if len(page.Items) > request.Limit || len(page.Items) == 0 && page.NextCursor != "" || !ValidOpaque(page.NextCursor) || page.NextCursor != "" && page.NextCursor == inner {
		return ResourcePage{}, ErrUnavailable
	}
	for _, item := range page.Items {
		if item.ParentResourceID == nil || *item.ParentResourceID != parent.ResourceID || item.CanonicalPath == nil {
			return ResourcePage{}, ErrUnavailable
		}
		relative := strings.TrimPrefix(*item.CanonicalPath, folder+"/")
		if relative == *item.CanonicalPath || relative == "" || strings.Contains(relative, "/") {
			return ResourcePage{}, ErrUnavailable
		}
	}
	items, err := s.resources(ctx, principal, binding, page.Items)
	if err != nil {
		return ResourcePage{}, err
	}
	if len(items) != len(page.Items) {
		return ResourcePage{}, ErrNotFound
	}
	result := ResourcePage{Items: items}
	if page.NextCursor != "" {
		result.NextCursor, err = encodePrivateFileCursor(privateFileCursor{Owner: string(principal.UserID), Prefix: folder, Parent: parent.ResourceID, Inner: page.NextCursor})
		if err != nil {
			return ResourcePage{}, err
		}
	}
	return result, nil
}

func decodePrivateFileCursor(raw string, owner auth.UserID, prefix, parent string) (string, error) {
	if raw == "" {
		return "", nil
	}
	if len(raw) > 2048 {
		return "", ErrInvalid
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return "", ErrInvalid
	}
	var cursor privateFileCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil {
		return "", ErrInvalid
	}
	if cursor.Owner != string(owner) || cursor.Prefix != prefix || cursor.Parent != parent || cursor.Inner == "" || !ValidOpaque(cursor.Inner) {
		return "", ErrInvalid
	}
	canonical, err := encodePrivateFileCursor(cursor)
	if err != nil || canonical != raw {
		return "", ErrInvalid
	}
	return cursor.Inner, nil
}

func encodePrivateFileCursor(cursor privateFileCursor) (string, error) {
	if cursor.Owner == "" || cursor.Prefix == "" || cursor.Parent == "" || cursor.Inner == "" ||
		!ValidOpaque(cursor.Owner) || !ValidOpaque(cursor.Prefix) || !ValidOpaque(cursor.Parent) || !ValidOpaque(cursor.Inner) {
		return "", ErrInvalid
	}
	encoded, err := json.Marshal(cursor)
	if err != nil {
		return "", ErrUnavailable
	}
	result := base64.RawURLEncoding.EncodeToString(encoded)
	if len(result) > 2048 {
		return "", ErrUnavailable
	}
	return result, nil
}

func (authorizer CloudSpaceResourceAuthorizer) PrivateScope(ctx context.Context, principal auth.Principal, bindingID storage.BindingID, rootID string) (string, error) {
	if !principal.Valid() {
		return "", auth.ErrUnauthorized
	}
	if missing(authorizer.Spaces) {
		return "", ErrUnavailable
	}
	space, err := authorizer.Spaces.GetSpaceByOwner(ctx, string(principal.UserID), bindingID)
	if errors.Is(err, ErrUserSpaceNotFound) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if space.ID == "" || space.OwnerUserID != string(principal.UserID) || space.BindingID != bindingID ||
		space.Status != UserSpaceActive || space.OwnerState != UserSpaceOwnerActive || space.IndexCoreRootID != rootID {
		return "", ErrNotFound
	}
	prefix, err := storage.NormalizeMountPath(space.CanonicalPrefix)
	if err != nil || prefix != space.CanonicalPrefix || !cloudpath.IsPrivate(prefix) ||
		!validUserSpaceDirectoryName(space.DirectoryName) || !strings.HasSuffix(prefix, "/_users/"+space.DirectoryName) {
		return "", ErrUnavailable
	}
	return prefix, nil
}
