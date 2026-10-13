package acquisition

import "context"

type StageDispatchPort interface {
	Dispatch(context.Context, StageDispatchRequest) (StageDispatchResult, error)
}

// KnownScopedDispatcher checks the immutable operator namespace before every
// stage, including Hint and Canonical. It cannot silently adopt a replaced Root.
type KnownScopedDispatcher struct {
	Scope     KnownPollScope
	Manifests ManifestReader
	Tasks     ProviderTaskStore
	Topology  StorageTopologyReader
	Next      StageDispatchPort
}

func (guard KnownScopedDispatcher) Dispatch(ctx context.Context, request StageDispatchRequest) (StageDispatchResult, error) {
	if !guard.Scope.Valid() || request.JobID != guard.Scope.JobID {
		return StageDispatchResult{}, ErrKnownRecoveryConflict
	}
	manifest, err := guard.Manifests.GetManifest(ctx, guard.Scope.ManifestID)
	if err != nil {
		return StageDispatchResult{}, ErrKnownRecoveryConflict
	}
	task, err := guard.Tasks.GetProviderTask(ctx, guard.Scope.ManifestID)
	if err != nil || !guard.Scope.Matches(manifest, task) {
		return StageDispatchResult{}, ErrKnownRecoveryConflict
	}
	binding, err := guard.Topology.GetBinding(ctx, guard.Scope.BindingID)
	if err != nil || binding.ConnectionID != guard.Scope.ConnectionID || binding.IndexCoreRootID != guard.Scope.RootID {
		return StageDispatchResult{}, ErrKnownRecoveryConflict
	}
	connection, err := guard.Topology.GetConnection(ctx, guard.Scope.ConnectionID)
	if err != nil || connection.ProviderType != string(guard.Scope.ProviderID) {
		return StageDispatchResult{}, ErrKnownRecoveryConflict
	}
	return guard.Next.Dispatch(ctx, request)
}
