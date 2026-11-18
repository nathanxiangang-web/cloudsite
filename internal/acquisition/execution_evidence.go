package acquisition

import (
	"context"
	"errors"
	"strings"

	"github.com/nathanxiangang-web/panta/internal/providers/contracts"
)

type ResultEvidenceStore interface {
	SaveResultObjectEvidence(context.Context, ResultObjectEvidence) error
	GetResultObjectEvidence(context.Context, ManifestID) (ResultObjectEvidence, error)
}

func WithResultEvidenceStore(store ResultEvidenceStore) ExecutionOption {
	return func(service *ExecutionStepService) error {
		if store == nil {
			return ErrInvalidExecutionRequest
		}
		service.resultEvidence = store
		return nil
	}
}

func (service *ExecutionStepService) captureResultEvidence(ctx context.Context, input ExecutionInput, resultName string, provider contracts.ObjectCopyProvider) error {
	if service.resultEvidence == nil || resultName == "" || input.ConnectionID == "" || input.Download.Target.Scope == "" {
		return nil
	}
	objects, err := provider.ListObjects(ctx, input.Download.Target.Scope)
	if err != nil {
		return nil
	}
	var matches []contracts.StorageObject
	for _, object := range objects {
		if object.Name == resultName {
			matches = append(matches, object)
		}
	}
	if len(matches) != 1 {
		return nil
	}
	object := matches[0]
	if object.Reference.Scope != input.Download.Target.Scope {
		return nil
	}
	if err := contracts.ValidateObjectEvidence(object); err != nil {
		return nil
	}
	evidence := ResultObjectEvidence{
		ManifestID: input.ManifestID, ProviderConnectionID: input.ConnectionID,
		ProviderObjectRef: object.Reference.Path, ProviderParentScope: object.Reference.Scope,
		ResultName: resultName, SizeBytes: object.SizeBytes, HashAlgorithm: object.HashAlgorithm,
		ContentHash: object.ContentHash, ObservedAt: service.now().UTC(),
	}
	existing, err := service.resultEvidence.GetResultObjectEvidence(ctx, input.ManifestID)
	if err == nil {
		if sameResultObjectEvidence(existing, evidence) {
			return nil
		}
		return ErrCopyOperationConflict
	}
	if !errors.Is(err, ErrCopyOperationNotFound) {
		return err
	}
	return service.resultEvidence.SaveResultObjectEvidence(ctx, evidence)
}

func sameResultObjectEvidence(left, right ResultObjectEvidence) bool {
	return left.ManifestID == right.ManifestID && left.ProviderConnectionID == right.ProviderConnectionID &&
		left.ProviderObjectRef == right.ProviderObjectRef && left.ProviderParentScope == right.ProviderParentScope &&
		left.ResultName == right.ResultName && left.SizeBytes == right.SizeBytes &&
		left.HashAlgorithm == right.HashAlgorithm && strings.EqualFold(left.ContentHash, right.ContentHash)
}