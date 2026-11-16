package runtime

import (
	"context"

	"github.com/nathanxiangang-web/panta/internal/acquisition"
	"github.com/nathanxiangang-web/panta/internal/providers/contracts"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

func (runtime *Runtime) DirectoryProvider(ctx context.Context, id storage.ConnectionID) (contracts.DirectoryProvider, error) {
	if runtime == nil || runtime.sessions == nil {
		return nil, ErrInvalidAcquisitionRuntime
	}
	registered, ok := runtime.sessionBindings[id]
	if !ok {
		return nil, ErrInvalidAcquisitionRuntime
	}
	credentialRef := string(registered.CredentialRef)
	binding, err := runtime.sessions.ResolveDownloader(ctx, acquisition.DownloaderSessionRequest{
		ProviderID: registered.ProviderID, ConnectionID: id, CredentialRef: &credentialRef,
	})
	if err != nil {
		return nil, err
	}
	provider, ok := binding.Downloader.(contracts.DirectoryProvider)
	if !ok {
		return nil, contracts.ErrUnsupportedCapability
	}
	return provider, nil
}
