package p115

import (
	"context"
	"errors"
	"testing"

	"github.com/nathanxiangang-web/panta/internal/providers/contracts"
)

type fakeCopyBackend struct {
	files        map[string]FileEvidence
	list         []FileEvidence
	inspectErr   error
	listErr      error
	copyErr      error
	inspectCalls int
	listCalls    int
	copyCalls    int
	copySource   string
	copyTarget   string
	match        contracts.DirectoryMatch
	createResult string
	createErr    error
}

func (backend *fakeCopyBackend) InspectFile(context.Context, string) (FileEvidence, error) {
	backend.inspectCalls++
	if backend.inspectErr != nil {
		return FileEvidence{}, backend.inspectErr
	}
	for _, file := range backend.files {
		return file, nil
	}
	return FileEvidence{}, errors.New("missing")
}

func (backend *fakeCopyBackend) ListFiles(context.Context, string) ([]FileEvidence, error) {
	backend.listCalls++
	if backend.listErr != nil {
		return nil, backend.listErr
	}
	return append([]FileEvidence(nil), backend.list...), nil
}

func (backend *fakeCopyBackend) CopyFile(_ context.Context, target, source string) error {
	backend.copyCalls++
	backend.copyTarget = target
	backend.copySource = source
	return backend.copyErr
}

func (backend *fakeCopyBackend) FindDirectory(context.Context, string, string) (contracts.DirectoryMatch, error) {
	return backend.match, backend.createErr
}

func (backend *fakeCopyBackend) CreateDirectory(context.Context, string, string) (string, error) {
	return backend.createResult, backend.createErr
}

func validCopyEvidence() FileEvidence {
	return FileEvidence{FileID: "file-1", ParentID: "parent-1", Name: "result.bin", SizeBytes: 42, SHA1: "0123456789abcdef0123456789abcdef01234567"}
}

func newCopyAdapter(t *testing.T, backend CopyBackend) *Adapter {
	t.Helper()
	return newAdapter(t, newFakeBackend(), func(options *Options) { options.CopyBackend = backend })
}

func TestInspectObjectRequiresExactIdentityAndStrongEvidence(t *testing.T) {
	evidence := validCopyEvidence()
	backend := &fakeCopyBackend{files: map[string]FileEvidence{evidence.FileID: evidence}}
	adapter := newCopyAdapter(t, backend)
	reference := contracts.StorageObjectReference{Scope: evidence.ParentID, Path: evidence.FileID}
	object, err := adapter.InspectObject(context.Background(), reference)
	if err != nil {
		t.Fatal(err)
	}
	if object.Reference != reference || object.HashAlgorithm != contracts.ContentHashSHA1 || object.ContentHash != evidence.SHA1 {
		t.Fatalf("unexpected object: %#v", object)
	}
	evidence.SHA1 = "not-a-sha1"
	backend.files[evidence.FileID] = evidence
	if _, err := adapter.InspectObject(context.Background(), reference); !errors.Is(err, ErrInvalidObjectEvidence) {
		t.Fatalf("error = %v, want ErrInvalidObjectEvidence", err)
	}
}

func TestInspectObjectRejectsChangedParent(t *testing.T) {
	evidence := validCopyEvidence()
	backend := &fakeCopyBackend{files: map[string]FileEvidence{evidence.FileID: evidence}}
	adapter := newCopyAdapter(t, backend)
	_, err := adapter.InspectObject(context.Background(), contracts.StorageObjectReference{Scope: "another-parent", Path: evidence.FileID})
	if !errors.Is(err, ErrObjectIdentity) {
		t.Fatalf("error = %v, want ErrObjectIdentity", err)
	}
}

func TestListObjectsSkipsDirectoriesAndRejectsDuplicates(t *testing.T) {
	evidence := validCopyEvidence()
	backend := &fakeCopyBackend{list: []FileEvidence{
		{FileID: "directory", ParentID: evidence.ParentID, IsDirectory: true}, evidence,
	}}
	adapter := newCopyAdapter(t, backend)
	objects, err := adapter.ListObjects(context.Background(), evidence.ParentID)
	if err != nil || len(objects) != 1 {
		t.Fatalf("objects = %#v, error = %v", objects, err)
	}
	backend.list = append(backend.list, evidence)
	if _, err := adapter.ListObjects(context.Background(), evidence.ParentID); !errors.Is(err, ErrObjectIdentity) {
		t.Fatalf("error = %v, want ErrObjectIdentity", err)
	}
}

func TestCopyObjectNeverRetriesBackendWrite(t *testing.T) {
	backend := &fakeCopyBackend{copyErr: errors.New("connection reset")}
	adapter := newCopyAdapter(t, backend)
	source := contracts.StorageObjectReference{Scope: "parent-1", Path: "file-1"}
	err := adapter.CopyObject(context.Background(), source, "parent-2")
	if !errors.Is(err, ErrObjectCopyUnknown) || backend.copyCalls != 1 {
		t.Fatalf("error = %v, calls = %d", err, backend.copyCalls)
	}
	if backend.copySource != source.Path || backend.copyTarget != "parent-2" {
		t.Fatalf("unexpected copy arguments: %q %q", backend.copySource, backend.copyTarget)
	}
}

func TestCopyObjectValidationAndUnsupportedCapabilityPerformNoWrite(t *testing.T) {
	backend := &fakeCopyBackend{}
	adapter := newCopyAdapter(t, backend)
	source := contracts.StorageObjectReference{Scope: "parent-1", Path: "file-1"}
	if err := adapter.CopyObject(context.Background(), source, source.Scope); !errors.Is(err, contracts.ErrInvalidStorageObject) {
		t.Fatalf("error = %v, want ErrInvalidStorageObject", err)
	}
	if backend.copyCalls != 0 {
		t.Fatal("invalid request performed a write")
	}
	plain := newAdapter(t, newFakeBackend())
	if err := plain.CopyObject(context.Background(), source, "parent-2"); !errors.Is(err, contracts.ErrUnsupportedCapability) {
		t.Fatalf("error = %v, want ErrUnsupportedCapability", err)
	}
}
