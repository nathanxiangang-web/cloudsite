package acquisition_test

import (
	"context"
	"testing"
	"time"

	"github.com/nathanxiangang-web/panta/internal/acquisition"
	"github.com/nathanxiangang-web/panta/internal/jobs"
	"github.com/nathanxiangang-web/panta/internal/providers/contracts"
)

type evidenceObjectProvider struct {
	*scriptedProvider
	objects []contracts.StorageObject
}

func (provider *evidenceObjectProvider) InspectObject(context.Context, contracts.StorageObjectReference) (contracts.StorageObject, error) {
	return provider.objects[0], nil
}
func (provider *evidenceObjectProvider) ListObjects(context.Context, string) ([]contracts.StorageObject, error) {
	return append([]contracts.StorageObject(nil), provider.objects...), nil
}
func (provider *evidenceObjectProvider) CopyObject(context.Context, contracts.StorageObjectReference, string) error {
	return nil
}

type resultEvidenceStoreFake struct {
	values   map[acquisition.ManifestID]acquisition.ResultObjectEvidence
	saveCalls int
}

func (store *resultEvidenceStoreFake) SaveResultObjectEvidence(_ context.Context, evidence acquisition.ResultObjectEvidence) error {
	if store.values == nil {
		store.values = map[acquisition.ManifestID]acquisition.ResultObjectEvidence{}
	}
	store.values[evidence.ManifestID] = evidence
	store.saveCalls++
	return nil
}
func (store *resultEvidenceStoreFake) GetResultObjectEvidence(_ context.Context, id acquisition.ManifestID) (acquisition.ResultObjectEvidence, error) {
	if value, ok := store.values[id]; ok {
		return value, nil
	}
	return acquisition.ResultObjectEvidence{}, acquisition.ErrCopyOperationNotFound
}

func TestExecutionCapturesDonorEvidenceOnce(t *testing.T) {
	manifests := &manifestReaderDouble{values: map[acquisition.ManifestID]acquisition.Manifest{executionManifestID: executionManifest()}}
	jobs := &jobReaderDouble{values: map[jobs.JobID]jobs.Job{executionJobID: executionJob()}}
	tasks := newTaskStoreDouble()
	provider := &evidenceObjectProvider{scriptedProvider: newScriptedProvider(executionProviderID)}
	sessions := &sessionDouble{bindings: map[sessionKey]contracts.DownloaderBinding{
		newSessionKey(executionProviderID, string(executionConnID)): {Descriptor: provider.Descriptor(), Downloader: provider},
	}}
	resolver, err := acquisition.NewExecutionInputResolver(manifests, executionTopology("library"))
	if err != nil {
		t.Fatal(err)
	}
	store := &resultEvidenceStoreFake{}
	service, err := acquisition.NewExecutionStepService(resolver, manifests, jobs, sessions, tasks,
		acquisition.WithExecutionClock(func() time.Time { return executionNow }), acquisition.WithResultEvidenceStore(store))
	if err != nil {
		t.Fatal(err)
	}
	provider.objects = []contracts.StorageObject{{
		Reference: contracts.StorageObjectReference{Scope: "library", Path: "file-a"}, Name: "movie.mkv",
		SizeBytes: 42, HashAlgorithm: contracts.ContentHashSHA1, ContentHash: privateCopyHash,
	}}
	provider.adoptTask("provider-task-0001", contracts.TaskStateSucceeded)
	provider.statuses["provider-task-0001"] = contracts.TaskStatus{
		Reference: contracts.TaskReference{Value: "provider-task-0001"}, State: contracts.TaskStateSucceeded,
		Result: &contracts.DownloadResult{Name: "movie.mkv"},
	}
	result, err := service.Execute(context.Background(), executionRequest())
	if err != nil || result.Outcome != acquisition.OutcomeProviderSucceeded {
		t.Fatalf("result = %#v, error = %v", result, err)
	}
	evidence := store.values[executionManifestID]
	if evidence.ProviderObjectRef != "file-a" || evidence.ProviderParentScope != "library" || evidence.ContentHash != privateCopyHash || store.saveCalls != 1 {
		t.Fatalf("evidence = %#v, saveCalls = %d", evidence, store.saveCalls)
	}
	if _, err := service.Execute(context.Background(), executionRequest()); err != nil {
		t.Fatal(err)
	}
	if store.saveCalls != 1 {
		t.Fatalf("replayed evidence save count = %d", store.saveCalls)
	}
}