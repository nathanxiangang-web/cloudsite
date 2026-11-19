package acquisition

import (
	"errors"
	"testing"
	"time"

	"github.com/nathanxiangang-web/panta/internal/catalog"
	"github.com/nathanxiangang-web/panta/internal/jobs"
	"github.com/nathanxiangang-web/panta/internal/providers/contracts"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

type copyPlanFixture struct {
	requester Manifest
	donor     Manifest
	job       jobs.Job
	task      ProviderTask
	evidence  ResultObjectEvidence
}

func newCopyPlanFixture() copyPlanFixture {
	requesterSpace := "space-b"
	donorSpace := "space-a"
	requesterJob := jobs.JobID("job-b")
	donorJob := jobs.JobID("job-a")
	connection := storage.ConnectionID("connection-a")
	requesterScope := "directory-b"
	donorScope := "directory-a"
	donorCopy := catalog.CopyID("copy-a")
	return copyPlanFixture{
		requester: Manifest{ID: "manifest-b", UserSpaceID: &requesterSpace, JobID: &requesterJob,
			TargetProviderConnectionID: &connection, TargetProviderScope: &requesterScope,
			TargetPath: "/云下载/_users/b", State: StateActive},
		donor: Manifest{ID: "manifest-a", UserSpaceID: &donorSpace, JobID: &donorJob,
			TargetProviderConnectionID: &connection, TargetProviderScope: &donorScope,
			ResultCopyID: &donorCopy, State: StateReady},
		job: jobs.Job{ID: donorJob, Type: "ACQUISITION", State: jobs.StateSucceeded},
		task: ProviderTask{ManifestID: "manifest-a", JobID: donorJob, ProviderID: "115",
			ProviderTaskRef: "task-a", State: ProviderTaskReferenceKnown,
			CreatedAt: time.Unix(1, 0), UpdatedAt: time.Unix(1, 0)},
		evidence: ResultObjectEvidence{ManifestID: "manifest-a", ProviderConnectionID: connection,
			ProviderObjectRef: "file-a", ProviderParentScope: "directory-a", ResultName: "result.bin",
			SizeBytes: 42, HashAlgorithm: "SHA1", ContentHash: "0123456789abcdef0123456789abcdef01234567",
			ObservedAt: time.Unix(1, 0)},
	}
}

func (fixture copyPlanFixture) plan(t *testing.T) CopyPlan {
	t.Helper()
	plan, err := NewCopyPlan(fixture.requester, fixture.donor, fixture.job, fixture.task,
		fixture.evidence, "copy-b", nil, time.Unix(100, 0))
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestNewCopyPlanBindsFrozenTargetAndDonorEvidence(t *testing.T) {
	fixture := newCopyPlanFixture()
	plan := fixture.plan(t)
	if plan.Operation.ManifestID != fixture.requester.ID || plan.Operation.DonorManifestID != fixture.donor.ID ||
		len(plan.Operation.RequestDigest) != 32 || plan.Source.Path != fixture.evidence.ProviderObjectRef {
		t.Fatalf("unexpected plan: %#v", plan)
	}
	second, err := NewCopyPlan(fixture.requester, fixture.donor, fixture.job, fixture.task,
		fixture.evidence, "another-copy-id", nil, time.Unix(100, 0))
	if err != nil {
		t.Fatal(err)
	}
	for index := range plan.Operation.RequestDigest {
		if plan.Operation.RequestDigest[index] != second.Operation.RequestDigest[index] {
			t.Fatal("same intent produced a different request digest")
		}
	}
}

func TestNewCopyPlanRejectsUnsafeDonors(t *testing.T) {
	for name, mutate := range map[string]func(*copyPlanFixture){
		"donor not ready":   func(value *copyPlanFixture) { value.donor.State = StateActive },
		"job not succeeded": func(value *copyPlanFixture) { value.job.State = jobs.StateRunning },
		"missing task reference": func(value *copyPlanFixture) {
			value.task.State = ProviderTaskStartReserved
			value.task.ProviderTaskRef = ""
		},
		"wrong connection":      func(value *copyPlanFixture) { value.evidence.ProviderConnectionID = "other" },
		"same user space":       func(value *copyPlanFixture) { value.donor.UserSpaceID = value.requester.UserSpaceID },
		"missing evidence hash": func(value *copyPlanFixture) { value.evidence.ContentHash = "" },
		"same native directory": func(value *copyPlanFixture) {
			value.evidence.ProviderParentScope = *value.requester.TargetProviderScope
		},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newCopyPlanFixture()
			mutate(&fixture)
			if _, err := NewCopyPlan(fixture.requester, fixture.donor, fixture.job, fixture.task,
				fixture.evidence, "copy-b", nil, time.Unix(100, 0)); !errors.Is(err, ErrCopyDonorUnavailable) {
				t.Fatalf("error = %v, want ErrCopyDonorUnavailable", err)
			}
		})
	}
}

func TestCheckCopyDestinationUnusedRejectsExistingName(t *testing.T) {
	fixture := newCopyPlanFixture()
	plan := fixture.plan(t)
	current := []contracts.StorageObject{{Reference: contracts.StorageObjectReference{Scope: "directory-b", Path: "existing"}, Name: "result.bin"}}
	if err := CheckCopyDestinationUnused(plan, fixture.requester, current); !errors.Is(err, ErrCopyDestinationConflict) {
		t.Fatalf("error = %v, want ErrCopyDestinationConflict", err)
	}
}

func TestReconcileCopyPlanRequiresExactlyOneNewMatchingObject(t *testing.T) {
	fixture := newCopyPlanFixture()
	plan := fixture.plan(t)
	matching := contracts.StorageObject{
		Reference: contracts.StorageObjectReference{Scope: "directory-b", Path: "file-b"},
		Name:      "result.bin", SizeBytes: 42, HashAlgorithm: "SHA1", ContentHash: fixture.evidence.ContentHash,
	}
	result, err := ReconcileCopyPlan(plan, fixture.requester, []contracts.StorageObject{matching})
	if err != nil || result.Status != CopyReconciliationConfirmed || result.ProviderResultRef != "file-b" {
		t.Fatalf("result = %#v, error = %v", result, err)
	}
	duplicate := matching
	duplicate.Reference.Path = "another-file"
	if _, err := ReconcileCopyPlan(plan, fixture.requester, []contracts.StorageObject{matching, duplicate}); !errors.Is(err, ErrCopyResultAmbiguous) {
		t.Fatalf("error = %v, want ErrCopyResultAmbiguous", err)
	}
}

func TestReconcileCopyPlanIgnoresBaselineAndWaitsForVisibility(t *testing.T) {
	fixture := newCopyPlanFixture()
	plan, err := NewCopyPlan(fixture.requester, fixture.donor, fixture.job, fixture.task,
		fixture.evidence, "copy-b", []string{"old-file"}, time.Unix(100, 0))
	if err != nil {
		t.Fatal(err)
	}
	baseline := contracts.StorageObject{
		Reference: contracts.StorageObjectReference{Scope: "directory-b", Path: "old-file"},
		Name:      "old.bin", SizeBytes: 1, HashAlgorithm: "SHA1", ContentHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	result, err := ReconcileCopyPlan(plan, fixture.requester, []contracts.StorageObject{baseline})
	if err != nil || result.Status != CopyReconciliationPending {
		t.Fatalf("result = %#v, error = %v", result, err)
	}
}
