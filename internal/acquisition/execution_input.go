package acquisition

import (
	"context"
	"errors"
	"fmt"

	"github.com/nathanxiangang-web/panta/internal/providers/contracts"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

var (
	ErrInvalidExecutionInputRequest = errors.New("invalid acquisition execution input request")
	ErrExecutionManifestNotFound    = errors.New("acquisition execution Manifest not found")
	ErrExecutionManifestState       = errors.New("acquisition execution Manifest is not ACTIVE")
	ErrExecutionManifestLinkage     = errors.New("acquisition execution Manifest has no durable Job link")
	ErrExecutionIdentityMismatch    = errors.New("acquisition execution identity mismatch")
)

type ManifestReader interface {
	GetManifest(context.Context, ManifestID) (Manifest, error)
}

type ExecutionInput struct {
	ManifestID    ManifestID
	ProviderID    contracts.ProviderID
	ConnectionID  storage.ConnectionID
	CredentialRef *string
	Download      contracts.DownloadRequest
}

type ExecutionInputResolver struct {
	manifests     ManifestReader
	storage       StorageTopologyReader
	privateSpaces PrivateTargetSpaceReader
}

type ExecutionInputOption func(*ExecutionInputResolver) error

func WithExecutionPrivateTargetSpaces(reader PrivateTargetSpaceReader) ExecutionInputOption {
	return func(resolver *ExecutionInputResolver) error {
		if reader == nil {
			return ErrInvalidExecutionInputRequest
		}
		resolver.privateSpaces = reader
		return nil
	}
}

func NewExecutionInputResolver(manifests ManifestReader, topology StorageTopologyReader, options ...ExecutionInputOption) (*ExecutionInputResolver, error) {
	if manifests == nil || topology == nil {
		return nil, ErrInvalidExecutionInputRequest
	}
	resolver := &ExecutionInputResolver{manifests: manifests, storage: topology}
	for _, option := range options {
		if option != nil {
			if err := option(resolver); err != nil {
				return nil, err
			}
		}
	}
	return resolver, nil
}

// Resolve builds provider-neutral execution input from durable Panta state. It
// performs no registry lookup, provider call, credential loading, or network IO.
func (resolver *ExecutionInputResolver) Resolve(ctx context.Context, manifestID ManifestID) (ExecutionInput, error) {
	if manifestID == "" {
		return ExecutionInput{}, ErrInvalidExecutionInputRequest
	}
	manifest, err := resolver.manifests.GetManifest(ctx, manifestID)
	if errors.Is(err, ErrNotFound) {
		return ExecutionInput{}, fmt.Errorf("%w: %s", ErrExecutionManifestNotFound, manifestID)
	}
	if err != nil {
		return ExecutionInput{}, fmt.Errorf("load acquisition Manifest: %w", err)
	}
	if manifest.ID != manifestID {
		return ExecutionInput{}, fmt.Errorf("%w: requested Manifest %s, got %s", ErrExecutionIdentityMismatch, manifestID, manifest.ID)
	}
	if manifest.State != StateActive {
		return ExecutionInput{}, fmt.Errorf("%w: %s", ErrExecutionManifestState, manifest.State)
	}
	if manifest.JobID == nil || *manifest.JobID == "" {
		return ExecutionInput{}, ErrExecutionManifestLinkage
	}
	binding, connection, err := loadAcquisitionTopology(ctx, resolver.storage, manifest.TargetStorageBindingID)
	if err != nil {
		return ExecutionInput{}, err
	}
	credentialRef := cloneString(connection.CredentialRef)
	scope := binding.ProviderScope
	connectionID := connection.ID
	if manifest.UserSpaceID != nil {
		if manifest.TargetProviderConnectionID == nil || manifest.TargetProviderScope == nil ||
			*manifest.TargetProviderConnectionID != connection.ID || resolver.privateSpaces == nil {
			return ExecutionInput{}, ErrExecutionIdentityMismatch
		}
		space, err := resolver.privateSpaces.GetPrivateTargetSpace(ctx, *manifest.UserSpaceID)
		if err != nil {
			return ExecutionInput{}, err
		}
		if err := ValidatePrivateTargetSpace(space, *manifest.UserSpaceID, binding, connection,
			*manifest.TargetProviderScope, manifest.TargetPath, manifest.UserID); err != nil {
			return ExecutionInput{}, err
		}
		scope = manifest.TargetProviderScope
		connectionID = *manifest.TargetProviderConnectionID
	}
	if scope == nil {
		return ExecutionInput{}, ErrExecutionIdentityMismatch
	}
	return ExecutionInput{
		ManifestID: manifest.ID, ProviderID: contracts.ProviderID(connection.ProviderType),
		ConnectionID: connectionID, CredentialRef: credentialRef,
		Download: contracts.DownloadRequest{
			Source: contracts.SourceReference{Scheme: manifest.SourceType, Value: manifest.SourceRef},
			Target: contracts.TargetPath{Scope: *scope, Path: manifest.TargetPath},
		},
	}, nil
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
