package product

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nathanxiangang-web/panta/internal/acquisition"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/catalog"
	"github.com/nathanxiangang-web/panta/internal/integrations/indexcore"
	"github.com/nathanxiangang-web/panta/internal/jobs"
	"github.com/nathanxiangang-web/panta/internal/resourceview"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

const bindingID storage.BindingID = "95000000-0000-4000-8000-000000000001"
const owner auth.UserID = "95000000-0000-4000-8000-000000000002"

func principal() auth.Principal {
	return auth.Principal{UserID: owner, Login: "nathan", Role: auth.RoleUser}
}

type readFake struct {
	records        []TaskRecord
	record         TaskRecord
	binding        storage.Binding
	page           indexcore.ResourcePage
	detail         resourceview.PhysicalDetail
	copy           catalog.Copy
	copyErr, error error
	calls          int
	block          bool
	timeout        bool
}

func (f *readFake) invoked(ctx context.Context) error {
	f.calls++
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) <= ReadTimeout {
		f.timeout = true
	}
	if f.block {
		<-ctx.Done()
		return ctx.Err()
	}
	return f.error
}
func (f *readFake) ListTasks(ctx context.Context, _ auth.UserID, _ []storage.BindingID, _ int, _ string) ([]TaskRecord, error) {
	return f.records, f.invoked(ctx)
}
func (f *readFake) GetTask(ctx context.Context, _ auth.UserID, _ []storage.BindingID, _ acquisition.ManifestID) (TaskRecord, error) {
	return f.record, f.invoked(ctx)
}
func (f *readFake) GetBinding(ctx context.Context, _ storage.BindingID) (storage.Binding, error) {
	return f.binding, f.invoked(ctx)
}
func (f *readFake) Browse(ctx context.Context, r indexcore.BrowseRequest) (indexcore.ResourcePage, error) {
	if r.RootID != "root" || r.IncludeRemoved || r.IncludeDeletedRoot || r.IncludeDeprecatedRoot {
		return indexcore.ResourcePage{}, ErrInvalid
	}
	return f.page, f.invoked(ctx)
}
func (f *readFake) Resolve(ctx context.Context, r indexcore.ResolveRequest) (indexcore.ResolveResult, error) {
	return indexcore.ResolveResult{}, ErrUnavailable
}

type physicalFake struct{ f *readFake }

func (f physicalFake) Resolve(ctx context.Context, r resourceview.PhysicalRequest) (resourceview.PhysicalDetail, error) {
	if r.RootID != "root" || r.IncludeRemoved || r.IncludeDeletedRoot || r.IncludeDeprecatedRoot {
		return resourceview.PhysicalDetail{}, ErrInvalid
	}
	return f.f.detail, f.f.invoked(ctx)
}
func (f *readFake) GetCopyByPhysicalIdentity(context.Context, string, string) (catalog.Copy, error) {
	return f.copy, f.copyErr
}
func newFixture(t *testing.T) (*ReadService, *readFake) {
	t.Helper()
	f := &readFake{binding: storage.Binding{ID: bindingID, IndexCoreRootID: "root", Status: storage.BindingStatusDisabled}, copyErr: catalog.ErrNotFound}
	s, err := NewReadService(f, f, f, physicalFake{f}, f, []storage.BindingID{bindingID})
	if err != nil {
		t.Fatal(err)
	}
	return s, f
}
func pending(id acquisition.ManifestID) TaskRecord {
	u := acquisition.UserID(owner)
	now := time.Now()
	return TaskRecord{Manifest: acquisition.Manifest{ID: id, UserID: &u, TargetStorageBindingID: bindingID, TargetPath: "/", SourceType: "https", SourceRef: "https://SECRET.invalid/source", State: acquisition.StatePending, CreatedAt: now, UpdatedAt: now}}
}
func physical() indexcore.ResourceContext {
	path := "/file.txt"
	name := "file.txt"
	return indexcore.ResourceContext{RootID: "root", ResourceID: "opaque-resource", CanonicalPath: &path, Name: &name, Presence: indexcore.ResourcePresent}
}
func TestTaskPolicyPagingRedactionAndPairState(t *testing.T) {
	s, f := newFixture(t)
	a := pending("95000000-0000-4000-8000-000000000010")
	b := pending("95000000-0000-4000-8000-000000000011")
	f.records = []TaskRecord{a, b}
	f.record = a
	page, err := s.Tasks(context.Background(), principal(), 1, "")
	if err != nil || len(page.Items) != 1 || page.NextCursor != string(a.Manifest.ID) || !f.timeout {
		t.Fatal("bounded page failed", err)
	}
	encoded, _ := json.Marshal(page)
	if strings.Contains(string(encoded), "SECRET") || strings.Contains(string(encoded), "source_ref") {
		t.Fatal("raw source leaked")
	}
	other := principal()
	other.UserID = "95000000-0000-4000-8000-000000000003"
	other.Role = auth.RoleAdmin
	if _, err := s.Task(context.Background(), other, a.Manifest.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-owner admin admitted")
	}
	original := a
	for _, kind := range []string{"null owner", "other binding", "invalid pending job", "reversed", "wrong linked job", "terminal mismatch"} {
		t.Run(kind, func(t *testing.T) {
			a = original
			f.record = a
			f.records = []TaskRecord{a, b}
			switch kind {
			case "null owner":
				f.record.Manifest.UserID = nil
			case "other binding":
				f.record.Manifest.TargetStorageBindingID = "95000000-0000-4000-8000-000000000004"
			case "invalid pending job":
				f.record.Job = &jobs.Job{ID: "wrong"}
			case "reversed":
				f.records = []TaskRecord{b, a}
			case "wrong linked job", "terminal mismatch":
				id := jobs.JobID("95000000-0000-4000-8000-000000000020")
				key := acquisition.AcquisitionJobIdempotencyKey(a.Manifest.ID)
				f.record.Manifest.JobID = &id
				f.record.Manifest.State = acquisition.StateActive
				f.record.Job = &jobs.Job{ID: id, Type: acquisition.JobTypeAcquisition, IdempotencyKey: &key, MaxAttempts: 1, State: jobs.StateSucceeded, Payload: json.RawMessage(`{"schema_version":1,"manifest_id":"95000000-0000-4000-8000-000000000010"}`)}
				if kind == "wrong linked job" {
					f.record.Job.ID = "wrong"
				}
			}
			var err error
			if kind == "reversed" {
				_, err = s.Tasks(context.Background(), principal(), 2, "")
			} else {
				_, err = s.Task(context.Background(), principal(), a.Manifest.ID)
			}
			if err == nil {
				t.Fatal("corrupt/unowned record accepted")
			}
		})
	}
}
func TestResourceCanonicalPolicyAndFailures(t *testing.T) {
	s, f := newFixture(t)
	p := physical()
	f.page = indexcore.ResourcePage{Items: []indexcore.ResourceContext{p}}
	f.detail = resourceview.PhysicalDetail{Matches: []resourceview.PhysicalMatch{{Physical: p, Classification: resourceview.ClassificationPhysicalOnly}}}
	page, err := s.Browse(context.Background(), principal(), BrowseRequest{BindingID: bindingID, Limit: 1})
	if err != nil || len(page.Items) != 1 || page.Items[0].CopyID != nil {
		t.Fatal("physical-only browse failed", err)
	}
	item, err := s.Resolve(context.Background(), principal(), bindingID, "/file.txt")
	if err != nil || item.ResourceID != p.ResourceID {
		t.Fatal("exact resolve failed", err)
	}
	for _, kind := range []string{"zero", "ambiguous", "multiple", "root mismatch", "removed", "path mismatch", "copy binding", "bad lineage"} {
		t.Run(kind, func(t *testing.T) {
			match := resourceview.PhysicalMatch{Physical: p, Classification: resourceview.ClassificationPhysicalOnly}
			f.detail = resourceview.PhysicalDetail{Matches: []resourceview.PhysicalMatch{match}}
			want := ErrUnavailable
			switch kind {
			case "zero":
				f.detail.Matches = nil
				want = ErrNotFound
			case "ambiguous":
				f.detail.Ambiguous = true
				want = ErrAmbiguous
			case "multiple":
				f.detail.Matches = append(f.detail.Matches, match)
				want = ErrAmbiguous
			case "root mismatch":
				f.detail.Matches[0].Physical.RootID = "other"
			case "removed":
				f.detail.Matches[0].Physical.Presence = indexcore.ResourceRemoved
			case "path mismatch":
				path := "/wrong"
				f.detail.Matches[0].Physical.CanonicalPath = &path
			case "copy binding":
				f.detail.Matches[0].Classification = resourceview.ClassificationUnresolvedCopy
				f.detail.Matches[0].Copy = &catalog.Copy{ID: "95000000-0000-4000-8000-000000000030", IndexCoreRootID: "root", IndexCoreResourceID: p.ResourceID, StorageBindingID: "wrong", Availability: catalog.CopyAvailabilityPresent}
			case "bad lineage":
				f.detail.Matches[0].Classification = resourceview.ClassificationClassified
			}
			if _, err := s.Resolve(context.Background(), principal(), bindingID, "/file.txt"); !errors.Is(err, want) {
				t.Fatal(kind, err)
			}
		})
	}
	f.page.Items[0].Presence = indexcore.ResourceRemoved
	if _, err := s.Browse(context.Background(), principal(), BrowseRequest{BindingID: bindingID, Limit: 1}); !errors.Is(err, ErrUnavailable) {
		t.Fatal("removed browse admitted")
	}
	before := f.calls
	if _, err := s.Resolve(context.Background(), principal(), "95000000-0000-4000-8000-000000000009", "/file.txt"); !errors.Is(err, ErrNotFound) || f.calls != before {
		t.Fatal("unlisted binding called upstream")
	}
}
func TestReadBoundsCancellationAndImmutableConfig(t *testing.T) {
	s, f := newFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f.block = true
	if _, err := s.Tasks(ctx, principal(), 1, ""); !errors.Is(err, context.Canceled) {
		t.Fatal("cancel ignored")
	}
	f.block = false
	for _, limit := range []int{-1, 0, 51} {
		if _, err := s.Tasks(context.Background(), principal(), limit, ""); !errors.Is(err, ErrInvalid) {
			t.Fatal("limit unbounded")
		}
	}
	if _, err := s.Tasks(context.Background(), auth.Principal{}, 1, ""); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatal("invalid principal admitted")
	}
	if _, err := s.Resolve(context.Background(), principal(), bindingID, "/../outside"); !errors.Is(err, ErrInvalid) {
		t.Fatal("dot path allowed")
	}
	f.error = errors.New("Cookie=SECRET/URL")
	if _, err := s.Tasks(context.Background(), principal(), 1, ""); err != ErrUnavailable {
		t.Fatal("raw error exposed", err)
	}
	ids := []storage.BindingID{bindingID}
	s, err := NewReadService(f, f, f, physicalFake{f}, f, ids)
	if err != nil {
		t.Fatal(err)
	}
	ids[0] = "wrong"
	if !s.bindings[bindingID] || s.allowed[0] != bindingID {
		t.Fatal("mutable external policy")
	}
	var typed *readFake
	if _, err := NewReadService(typed, f, f, physicalFake{f}, f, []storage.BindingID{bindingID}); err == nil {
		t.Fatal("typed nil wired")
	}
}
