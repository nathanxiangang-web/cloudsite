package acquisitionruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nathanxiangang-web/panta/internal/acquisition"
	"github.com/nathanxiangang-web/panta/internal/app"

	"github.com/nathanxiangang-web/panta/internal/jobs"
	"github.com/nathanxiangang-web/panta/internal/product"
	"github.com/nathanxiangang-web/panta/internal/providers/contracts"
	"github.com/nathanxiangang-web/panta/internal/runtime"
	"github.com/nathanxiangang-web/panta/internal/storage"
	"github.com/nathanxiangang-web/panta/internal/store/postgres"
)

const privateCopyHash = "0123456789abcdef0123456789abcdef01234567"

type runtimeCopyProvider struct {
	runtimeDownloader
	donor       contracts.StorageObject
	copied      contracts.StorageObject
	target      []contracts.StorageObject
	copyCalls   int
	copyErr     error
	copyApplied bool
	stop        context.CancelFunc
}

func (provider *runtimeCopyProvider) InspectObject(_ context.Context, reference contracts.StorageObjectReference) (contracts.StorageObject, error) {
	if reference == provider.donor.Reference {
		return provider.donor, nil
	}
	if reference == provider.copied.Reference {
		return provider.copied, nil
	}
	return contracts.StorageObject{}, errors.New("object not found")
}

func (provider *runtimeCopyProvider) ListObjects(_ context.Context, scope string) ([]contracts.StorageObject, error) {
	switch scope {
	case provider.copied.Reference.Scope:
		return append([]contracts.StorageObject(nil), provider.target...), nil
	case provider.donor.Reference.Scope:
		if provider.donor.Reference.Path == "" {
			return []contracts.StorageObject{}, nil
		}
		return []contracts.StorageObject{provider.donor}, nil
	default:
		return nil, errors.New("wrong scope")
	}
}

func (provider *runtimeCopyProvider) CopyObject(_ context.Context, _ contracts.StorageObjectReference, _ string) error {
	provider.copyCalls++
	if provider.copyApplied {
		provider.target = []contracts.StorageObject{provider.copied}
	}
	if provider.stop != nil {
		provider.stop()
	}
	return provider.copyErr
}

func privateCopyObject(scope, id string) contracts.StorageObject {
	return contracts.StorageObject{Reference: contracts.StorageObjectReference{Scope: scope, Path: id},
		Name: "movie.mkv", SizeBytes: 42, HashAlgorithm: contracts.ContentHashSHA1, ContentHash: privateCopyHash}
}

func seedPrivateCopyRuntime(t *testing.T) (fixtureState, acquisition.ManifestID, jobs.JobID, *runtimeCopyProvider) {
	t.Helper()
	fixture, donorID, donorJobID := runtimeFixture(t)
	spaces, err := postgres.NewCloudUserSpaceRepository(fixture.pool)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for _, space := range []struct {
		id, owner, nativeID, prefix string
	}{
		{"54000000-0000-4000-8000-000000000101", "54000000-0000-4000-8000-000000000102", "donor-dir", "/downloads/_users/a"},
		{"54000000-0000-4000-8000-000000000103", "54000000-0000-4000-8000-000000000104", "requester-dir", "/downloads/_users/b"},
	} {
		nativeID := space.nativeID
		if err := spaces.CreateSpace(fixture.ctx, product.CloudUserSpace{
			ID: product.UserSpaceID(space.id), OwnerUserID: space.owner, BindingID: fixture.bindingID,
			ProviderConnectionID: storage.ConnectionID(fixture.bindingID), IndexCoreRootID: "canonical-root",
			NativeParentID: "runtime-save-dir", NativeDirID: &nativeID, CanonicalPrefix: space.prefix,
			DirectoryName: "user", Status: product.UserSpaceActive, OwnerState: product.UserSpaceOwnerActive,
			ProvisioningKey: "test", Version: 0, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}
	copyID := "54000000-0000-4000-8000-000000000105"
	if _, err := fixture.pool.Exec(fixture.ctx, `INSERT INTO copies(
copy_id,indexcore_root_id,indexcore_resource_id,storage_binding_id,availability,created_at,updated_at
) VALUES($1,'canonical-root','donor-resource',$2,'PRESENT',$3,$3)`, copyID, string(fixture.bindingID), now); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.pool.Exec(fixture.ctx, `UPDATE acquisition_manifests SET state='READY', result_name='movie.mkv', result_copy_id=$2,
user_space_id='54000000-0000-4000-8000-000000000101', target_provider_connection_id=$3,
target_provider_scope='donor-dir', target_path='/downloads/_users/a' WHERE manifest_id=$1`, string(donorID), copyID, string(fixture.bindingID)); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.pool.Exec(fixture.ctx, `UPDATE jobs SET state='SUCCEEDED', finished_at=$2, updated_at=$2 WHERE job_id=$1`, string(donorJobID), now); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.pool.Exec(fixture.ctx, `INSERT INTO acquisition_provider_tasks(
manifest_id,job_id,provider_id,provider_task_ref,state,created_at,updated_at
) VALUES($1,$2,'115','donor-task','REFERENCE_KNOWN',$3,$3)`, string(donorID), string(donorJobID), now); err != nil {
		t.Fatal(err)
	}
	copies, err := postgres.NewAcquisitionCopyRepository(fixture.pool)
	if err != nil {
		t.Fatal(err)
	}
	if err := copies.SaveResultObjectEvidence(fixture.ctx, acquisition.ResultObjectEvidence{
		ManifestID: donorID, ProviderConnectionID: storage.ConnectionID(fixture.bindingID),
		ProviderObjectRef: "donor-file", ProviderParentScope: "donor-dir", ResultName: "movie.mkv",
		SizeBytes: 42, HashAlgorithm: "SHA1", ContentHash: privateCopyHash, ObservedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	requesterID := acquisition.ManifestID("54000000-0000-4000-8000-000000000106")
	requesterJobID := jobs.JobID("54000000-0000-4000-8000-000000000107")
	jobStore, err := postgres.NewJobRepository(fixture.pool)
	if err != nil {
		t.Fatal(err)
	}
	key := acquisition.AcquisitionJobIdempotencyKey(requesterID)
	if _, err := jobStore.Create(fixture.ctx, jobs.CreateRequest{ID: requesterJobID, Type: acquisition.JobTypeAcquisition,
		Payload:        []byte(fmt.Sprintf(`{"manifest_id":"%s","schema_version":1}`, requesterID)),
		IdempotencyKey: &key, MaxAttempts: 3}); err != nil {
		t.Fatal(err)
	}
	manifests, err := postgres.NewAcquisitionManifestRepository(fixture.pool)
	if err != nil {
		t.Fatal(err)
	}
	owner := acquisition.UserID("54000000-0000-4000-8000-000000000104")
	spaceID := "54000000-0000-4000-8000-000000000103"
	connectionID := storage.ConnectionID(fixture.bindingID)
	scope := "requester-dir"
	if err := manifests.CreateManifest(fixture.ctx, acquisition.Manifest{
		ID: requesterID, UserID: &owner, SourceType: "magnet", SourceRef: "magnet:?xt=urn:btih:synthetic",
		ExpectedName: stringPointer("movie.mkv"), TargetStorageBindingID: fixture.bindingID,
		TargetPath: "/downloads/_users/b", UserSpaceID: &spaceID, TargetProviderConnectionID: &connectionID,
		TargetProviderScope: &scope, JobID: &requesterJobID, State: acquisition.StateActive, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	provider := &runtimeCopyProvider{donor: privateCopyObject("donor-dir", "donor-file"), copied: privateCopyObject("requester-dir", "requester-file")}
	return fixture, requesterID, requesterJobID, provider
}

func stringPointer(value string) *string { return &value }

func runPrivateCopyRuntime(t *testing.T, fixture fixtureState, manifestID acquisition.ManifestID, jobID jobs.JobID, provider *runtimeCopyProvider, stopOnCanonical, discoverSpace bool) (int, int, int) {
	t.Helper()
	var parentVisible, directoryVisible, resultVisible atomic.Bool
	parentVisible.Store(!discoverSpace)
	directoryVisible.Store(!discoverSpace)
	var hintCalls, resolveCalls, journalCalls atomic.Int32
	read := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/roots/canonical-root/resolve":
			switch r.URL.Query().Get("path") {
			case "/downloads":
				fmt.Fprint(w, `{"matches":[{"resource_id":"downloads-root","root_id":"canonical-root","canonical_path":"/downloads","is_dir":true,"resource_presence":"PRESENT","introduced_at_generation":1,"last_confirmed_generation":1}],"ambiguous":false}`)
			case "/downloads/_users":
				if !parentVisible.Load() {
					fmt.Fprint(w, `{"matches":[],"ambiguous":false}`)
					return
				}
				fmt.Fprint(w, `{"matches":[{"resource_id":"spaces-root","root_id":"canonical-root","canonical_path":"/downloads/_users","is_dir":true,"resource_presence":"PRESENT","introduced_at_generation":1,"last_confirmed_generation":1}],"ambiguous":false}`)
			case "/downloads/_users/b":
				if !directoryVisible.Load() {
					fmt.Fprint(w, `{"matches":[],"ambiguous":false}`)
					return
				}
				fmt.Fprint(w, `{"matches":[{"resource_id":"requester-dir","root_id":"canonical-root","canonical_path":"/downloads/_users/b","is_dir":true,"resource_presence":"PRESENT","introduced_at_generation":1,"last_confirmed_generation":1}],"ambiguous":false}`)
			case "/downloads/_users/b/movie.mkv":
				resolveCalls.Add(1)
				if !resultVisible.Load() {
					fmt.Fprint(w, `{"matches":[],"ambiguous":false}`)
					return
				}
				fmt.Fprint(w, `{"matches":[{"resource_id":"requester-resource","root_id":"canonical-root","canonical_path":"/downloads/_users/b/movie.mkv","parent_resource_id":"requester-dir","name":"movie.mkv","is_dir":false,"size":42,"resource_presence":"PRESENT","introduced_at_generation":1,"last_confirmed_generation":1}],"ambiguous":false}`)
			default:
				t.Error("unexpected private resolve path")
				http.NotFound(w, r)
			}
		case "/v1/roots/canonical-root/journal":
			journalCalls.Add(1)
			fmt.Fprint(w, `{"items":[{"event_seq":1,"generation_number":1,"intra_generation_seq":1,"event_type":"resource-added","resource_id":"requester-resource","payload":"{}","committed_at":"2026-09-28T01:02:03Z"}]}`)
		default:
			t.Errorf("unexpected read path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer read.Close()
	hint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Scope string `json:"scope_key"`
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil {
			t.Error("incomplete Hint request")
			http.Error(w, "invalid hint", http.StatusBadRequest)
			return
		}
		switch request.Scope {
		case "/downloads":
			if !discoverSpace || parentVisible.Swap(true) {
				t.Error("unexpected base Hint")
			}
		case "/downloads/_users":
			if !discoverSpace || !parentVisible.Load() || directoryVisible.Swap(true) {
				t.Error("unexpected parent Hint")
			}
		case "/downloads/_users/b":
			if !directoryVisible.Load() {
				t.Error("target Hint preceded parent discovery")
			}
			resultVisible.Store(true)
		default:
			t.Error("unexpected Hint scope")
			http.Error(w, "invalid scope", http.StatusBadRequest)
			return
		}
		seq := hintCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		fmt.Fprintf(w, `{"status":"accepted","root_id":"canonical-root","scope_key":%q,"work_state":"PENDING","signal_seq":%d}`, request.Scope, seq)
	}))
	defer hint.Close()
	cfg := runtimeConfig(read.URL)
	deps := runtimeDeps(fixture, hint.URL, provider)
	clock := &runtimeClock{now: time.Now().UTC()}
	waiter := runtimeWaiter{clock: clock}
	runCtx, stop := context.WithTimeout(fixture.ctx, 8*time.Second)
	defer stop()
	if provider.stop != nil {
		provider.stop = stop
	}
	deps.WorkerOptions = []app.WorkerOption{app.WithWorkerClock(clock.Now), app.WithWorkerWaiter(waiter),
		app.WithWorkerEventSink(func(event app.WorkerEvent) {
			if stopOnCanonical && event.Kind == app.WorkerStage && event.Stage == acquisition.StageCanonical {
				stop()
			}
			if event.Kind == app.WorkerFatalRecoveryDebt {
				t.Errorf("fatal recovery debt: %s", event.ErrorKind)
				stop()
			}
		})}
	application, err := runtime.NewRuntime(fixture.ctx, cfg, deps)
	if err != nil {
		t.Fatal(err)
	}
	defer application.Close()
	if err := application.Run(runCtx); err != nil {
		t.Fatal(err)
	}
	return int(hintCalls.Load()), int(resolveCalls.Load()), int(journalCalls.Load())
}

func TestPostgresPrivateCopyRuntimeReady(t *testing.T) {
	fixture, manifestID, jobID, provider := seedPrivateCopyRuntime(t)
	provider.copyApplied = true
	hintCalls, resolveCalls, journalCalls := runPrivateCopyRuntime(t, fixture, manifestID, jobID, provider, true, true)
	manifest := readManifest(t, fixture.ctx, fixture.pool, manifestID)
	job := readJob(t, fixture.ctx, fixture.pool, jobID)
	if manifest.State != acquisition.StateReady || manifest.ResultCopyID == nil || job.State != jobs.StateSucceeded || job.FailureCount != 0 ||
		provider.copyCalls != 1 || provider.starts != 0 || hintCalls != 3 || resolveCalls != 1 || journalCalls != 1 {
		t.Fatalf("state=%s copy=%v job=%s copyCalls=%d starts=%d hint/Q5/Q8=%d/%d/%d", manifest.State, manifest.ResultCopyID, job.State, provider.copyCalls, provider.starts, hintCalls, resolveCalls, journalCalls)
	}
	var status, resultRef string
	if err := fixture.pool.QueryRow(fixture.ctx, `SELECT status, provider_result_ref FROM acquisition_copy_operations WHERE manifest_id=$1`, string(manifestID)).Scan(&status, &resultRef); err != nil || status != string(acquisition.CopyProviderConfirmed) || resultRef != "requester-file" {
		t.Fatalf("copy operation = %s/%s, error = %v", status, resultRef, err)
	}
	var donorCopy string
	if err := fixture.pool.QueryRow(fixture.ctx, `SELECT result_copy_id::text FROM acquisition_manifests WHERE manifest_id=$1`, string(manifestID)).Scan(&donorCopy); err != nil {
		t.Fatal(err)
	}
	if donorCopy == "54000000-0000-4000-8000-000000000105" {
		t.Fatal("requester reused donor Copy identity")
	}
}

func TestPostgresPrivateCopyUnknownRecoversWithoutSecondCopy(t *testing.T) {
	fixture, manifestID, jobID, provider := seedPrivateCopyRuntime(t)
	provider.copyApplied = true
	provider.copyErr = errors.New("response lost")
	provider.stop = func() {}
	_, _, _ = runPrivateCopyRuntime(t, fixture, manifestID, jobID, provider, false, false)
	var status string
	if err := fixture.pool.QueryRow(fixture.ctx, `SELECT status FROM acquisition_copy_operations WHERE manifest_id=$1`, string(manifestID)).Scan(&status); err != nil || status != string(acquisition.CopyProviderUnknown) {
		t.Fatalf("copy operation status = %s, error = %v", status, err)
	}
	if provider.copyCalls != 1 || provider.starts != 0 {
		t.Fatalf("unexpected provider writes: copy=%d start=%d", provider.copyCalls, provider.starts)
	}
	currentJob := readJob(t, fixture.ctx, fixture.pool, jobID)
	if currentJob.State == jobs.StateRunning {
		if _, err := fixture.pool.Exec(fixture.ctx, `UPDATE jobs SET lease_expires_at=clock_timestamp()-interval '1 second' WHERE job_id=$1`, string(jobID)); err != nil {
			t.Fatal(err)
		}
	}
	jobStore, err := postgres.NewJobRepository(fixture.pool)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jobStore.MarkExpiredAcquisitionRecoveryRequired(fixture.ctx, jobs.RecoveryRequest{OnlyJobID: jobID, Now: time.Now().UTC(), Limit: 1}); err != nil {
		t.Fatal(err)
	}
	manifest := readManifest(t, fixture.ctx, fixture.pool, manifestID)
	job := readJob(t, fixture.ctx, fixture.pool, jobID)
	recovery, err := postgres.NewKnownRecoveryRepository(fixture.pool)
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := recovery.ResumeKnownAcquisition(fixture.ctx, acquisition.KnownRecoveryRequest{
		Scope: acquisition.KnownPollScope{ManifestID: manifestID, JobID: jobID, BindingID: fixture.bindingID,
			ConnectionID: storage.ConnectionID(fixture.bindingID), ProviderID: "115", RootID: "canonical-root",
			ReferenceSHA256: acquisition.EvidenceSHA256("copy-recovery"), SourceSHA256: acquisition.EvidenceSHA256(manifest.SourceRef)},
		ExpectedClaim: job.ClaimAttempts, ManifestUpdatedAt: manifest.UpdatedAt, JobUpdatedAt: job.UpdatedAt, RetryDelay: time.Second,
	})
	if err != nil || !resumed.Changed {
		t.Fatalf("resume result = %#v, error = %v", resumed, err)
	}
	provider.donor = contracts.StorageObject{}
	provider.copyErr = nil
	provider.stop = nil
	hintCalls, resolveCalls, journalCalls := runPrivateCopyRuntime(t, fixture, manifestID, jobID, provider, true, false)
	manifest = readManifest(t, fixture.ctx, fixture.pool, manifestID)
	job = readJob(t, fixture.ctx, fixture.pool, jobID)
	if manifest.State != acquisition.StateReady || job.State != jobs.StateSucceeded || provider.copyCalls != 1 || hintCalls != 1 || resolveCalls != 1 || journalCalls != 1 {
		t.Fatalf("recovery state=%s job=%s copyCalls=%d hint/Q5/Q8=%d/%d/%d", manifest.State, job.State, provider.copyCalls, hintCalls, resolveCalls, journalCalls)
	}
}
