package p115

import (
	"context"
	"errors"
	"testing"

	"github.com/SheltonZhu/115driver/pkg/driver"
	"github.com/nathanxiangang-web/panta/internal/providers/contracts"
)

func TestCookiedBackendFindDirectoryRequiresUniqueExactMatch(t *testing.T) {
	directory := driver.File{FileID: "directory", ParentID: "parent", Name: "user__uuid", IsDirectory: true}
	client := &fakeCopyClient{
		files: map[string]driver.File{"parent": {FileID: "parent", IsDirectory: true}},
		pages: map[int64][]driver.File{0: {directory}},
	}
	backend := &CookiedBackend{copyClient: client}
	match, err := backend.FindDirectory(context.Background(), "parent", "user__uuid")
	if err != nil || !match.Found || match.DirectoryID != "directory" {
		t.Fatalf("match = %#v, error = %v", match, err)
	}
	client.pages[0] = append(client.pages[0], directory)
	if _, err := backend.FindDirectory(context.Background(), "parent", "user__uuid"); !errors.Is(err, ErrObjectIdentity) {
		t.Fatalf("error = %v, want ErrObjectIdentity", err)
	}
}

func TestCookiedBackendCreateDirectoryCallsMkdirOnce(t *testing.T) {
	client := &fakeCopyClient{mkdirResult: "new-directory"}
	backend := &CookiedBackend{copyClient: client}
	id, err := backend.CreateDirectory(context.Background(), "parent", "user__uuid")
	if err != nil || id != "new-directory" || client.mkdirCalls != 1 {
		t.Fatalf("id = %q, calls = %d, error = %v", id, client.mkdirCalls, err)
	}
}

func TestAdapterCreateDirectoryRejectsUnknownAndInvalidInput(t *testing.T) {
	backend := &fakeCopyBackend{createErr: errors.New("timeout")}
	adapter := newCopyAdapter(t, backend)
	if _, err := adapter.CreateDirectory(context.Background(), "parent", "user__uuid"); !errors.Is(err, ErrDirectoryCreateUnknown) {
		t.Fatalf("error = %v, want ErrDirectoryCreateUnknown", err)
	}
	if _, err := adapter.CreateDirectory(context.Background(), "parent", "nested/name"); !errors.Is(err, contracts.ErrInvalidStorageObject) {
		t.Fatalf("error = %v, want ErrInvalidStorageObject", err)
	}
}
