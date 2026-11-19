package p115

import (
	"context"
	"errors"
	"fmt"

	"github.com/nathanxiangang-web/panta/internal/providers/contracts"
)

var (
	ErrInvalidObjectEvidence = errors.New("115 object evidence is invalid")
	ErrObjectRead            = errors.New("115 object read failed")
	ErrObjectList            = errors.New("115 object listing failed")
	ErrObjectCopyUnknown     = errors.New("115 object copy outcome is unknown")
	ErrObjectIdentity        = errors.New("115 object identity is ambiguous")
)

type FileEvidence struct {
	FileID      string
	ParentID    string
	Name        string
	SizeBytes   int64
	SHA1        string
	IsDirectory bool
}

type CopyBackend interface {
	InspectFile(context.Context, string) (FileEvidence, error)
	ListFiles(context.Context, string) ([]FileEvidence, error)
	CopyFile(context.Context, string, string) error
	FindDirectory(context.Context, string, string) (contracts.DirectoryMatch, error)
	CreateDirectory(context.Context, string, string) (string, error)
}

var _ contracts.ObjectCopyProvider = (*Adapter)(nil)

func (adapter *Adapter) InspectObject(ctx context.Context, reference contracts.StorageObjectReference) (contracts.StorageObject, error) {
	if err := ctx.Err(); err != nil {
		return contracts.StorageObject{}, err
	}
	if err := contracts.ValidateObjectReference(reference); err != nil || adapter.copyBackend == nil {
		if err != nil {
			return contracts.StorageObject{}, fmt.Errorf("%w: reference", ErrInvalidObjectEvidence)
		}
		return contracts.StorageObject{}, contracts.ErrUnsupportedCapability
	}
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	file, err := adapter.copyBackend.InspectFile(ctx, reference.Path)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return contracts.StorageObject{}, err
		}
		if errors.Is(err, contracts.ErrUnsupportedCapability) {
			return contracts.StorageObject{}, err
		}
		return contracts.StorageObject{}, ErrObjectRead
	}
	object, err := storageObjectFromEvidence(file)
	if err != nil {
		return contracts.StorageObject{}, err
	}
	if object.Reference != reference {
		return contracts.StorageObject{}, ErrObjectIdentity
	}
	return object, nil
}

func (adapter *Adapter) ListObjects(ctx context.Context, parentScope string) ([]contracts.StorageObject, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := contracts.ValidateObjectReference(contracts.StorageObjectReference{Scope: parentScope, Path: "scope"}); err != nil {
		return nil, fmt.Errorf("%w: parent scope", ErrInvalidObjectEvidence)
	}
	if adapter.copyBackend == nil {
		return nil, contracts.ErrUnsupportedCapability
	}
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	files, err := adapter.copyBackend.ListFiles(ctx, parentScope)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		if errors.Is(err, contracts.ErrUnsupportedCapability) {
			return nil, err
		}
		return nil, ErrObjectList
	}
	objects := make([]contracts.StorageObject, 0, len(files))
	seen := make(map[string]struct{}, len(files))
	for _, file := range files {
		if file.IsDirectory {
			continue
		}
		object, err := storageObjectFromEvidence(file)
		if err != nil {
			return nil, err
		}
		if object.Reference.Scope != parentScope {
			return nil, ErrObjectIdentity
		}
		if _, exists := seen[object.Reference.Path]; exists {
			return nil, ErrObjectIdentity
		}
		seen[object.Reference.Path] = struct{}{}
		objects = append(objects, object)
	}
	return objects, nil
}

func (adapter *Adapter) CopyObject(ctx context.Context, source contracts.StorageObjectReference, destinationScope string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := contracts.ValidateObjectReference(source); err != nil {
		return fmt.Errorf("%w: source", contracts.ErrInvalidStorageObject)
	}
	if err := contracts.ValidateObjectReference(contracts.StorageObjectReference{Scope: destinationScope, Path: "scope"}); err != nil {
		return fmt.Errorf("%w: destination", contracts.ErrInvalidStorageObject)
	}
	if source.Scope == destinationScope {
		return fmt.Errorf("%w: source and destination must differ", contracts.ErrInvalidStorageObject)
	}
	if adapter.copyBackend == nil {
		return contracts.ErrUnsupportedCapability
	}
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	if err := adapter.copyBackend.CopyFile(ctx, destinationScope, source.Path); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("%w: %w", ErrObjectCopyUnknown, err)
		}
		if errors.Is(err, contracts.ErrUnsupportedCapability) {
			return err
		}
		return ErrObjectCopyUnknown
	}
	return nil
}

func storageObjectFromEvidence(file FileEvidence) (contracts.StorageObject, error) {
	object := contracts.StorageObject{
		Reference: contracts.StorageObjectReference{Scope: file.ParentID, Path: file.FileID},
		Name:      file.Name, SizeBytes: file.SizeBytes, Directory: file.IsDirectory,
		HashAlgorithm: contracts.ContentHashSHA1, ContentHash: file.SHA1,
	}
	if err := contracts.ValidateObjectEvidence(object); err != nil {
		return contracts.StorageObject{}, ErrInvalidObjectEvidence
	}
	return object, nil
}
