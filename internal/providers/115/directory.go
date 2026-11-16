package p115

import (
	"context"
	"errors"

	"github.com/nathanxiangang-web/panta/internal/providers/contracts"
)

var ErrDirectoryCreateUnknown = errors.New("115 directory creation outcome is unknown")

func (adapter *Adapter) FindDirectory(ctx context.Context, parentID, name string) (contracts.DirectoryMatch, error) {
	if err := ctx.Err(); err != nil {
		return contracts.DirectoryMatch{}, err
	}
	if err := contracts.ValidateObjectReference(contracts.StorageObjectReference{Scope: parentID, Path: "scope"}); err != nil ||
		contracts.ValidateDirectChildName(name) != nil {
		return contracts.DirectoryMatch{}, contracts.ErrInvalidStorageObject
	}
	if adapter.copyBackend == nil {
		return contracts.DirectoryMatch{}, contracts.ErrUnsupportedCapability
	}
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	files, err := adapter.copyBackend.ListFiles(ctx, parentID)
	if err != nil {
		return contracts.DirectoryMatch{}, err
	}
	var match contracts.DirectoryMatch
	for _, file := range files {
		if !file.IsDirectory || file.ParentID != parentID || file.Name != name {
			continue
		}
		if match.Found {
			return contracts.DirectoryMatch{}, ErrObjectIdentity
		}
		match = contracts.DirectoryMatch{DirectoryID: file.FileID, ParentID: file.ParentID, Name: file.Name, Found: true}
	}
	return match, nil
}

func (adapter *Adapter) CreateDirectory(ctx context.Context, parentID, name string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := contracts.ValidateObjectReference(contracts.StorageObjectReference{Scope: parentID, Path: "scope"}); err != nil ||
		contracts.ValidateDirectChildName(name) != nil {
		return "", contracts.ErrInvalidStorageObject
	}
	if adapter.copyBackend == nil {
		return "", contracts.ErrUnsupportedCapability
	}
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	directoryID, err := adapter.copyBackend.CreateDirectory(ctx, parentID, name)
	if err != nil || directoryID == "" {
		return "", ErrDirectoryCreateUnknown
	}
	return directoryID, nil
}

func (backend *CookiedBackend) FindDirectory(ctx context.Context, parentID, name string) (contracts.DirectoryMatch, error) {
	files, err := backend.ListFiles(ctx, parentID)
	if err != nil {
		return contracts.DirectoryMatch{}, err
	}
	var match contracts.DirectoryMatch
	for _, file := range files {
		if !file.IsDirectory || file.ParentID != parentID || file.Name != name {
			continue
		}
		if match.Found {
			return contracts.DirectoryMatch{}, ErrObjectIdentity
		}
		match = contracts.DirectoryMatch{DirectoryID: file.FileID, ParentID: file.ParentID, Name: file.Name, Found: true}
	}
	return match, nil
}

func (backend *CookiedBackend) CreateDirectory(ctx context.Context, parentID, name string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if backend.copyClient == nil {
		return "", contracts.ErrUnsupportedCapability
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	return backend.copyClient.Mkdir(parentID, name)
}
