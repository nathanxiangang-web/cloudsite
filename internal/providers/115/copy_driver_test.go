package p115

import (
	"context"
	"errors"
	"testing"

	"github.com/SheltonZhu/115driver/pkg/driver"
)

type fakeCopyClient struct {
	files          map[string]driver.File
	pages          map[int64][]driver.File
	category       string
	count          int
	useCount       bool
	reportedOffset *int
	listOffsets    []int64
	copyCalls      int
	copyTarget     string
	copySource     string
	copyErr        error
	mkdirResult    string
	mkdirErr       error
	mkdirCalls     int
}

func (client *fakeCopyClient) GetFile(fileID string) (*driver.File, error) {
	file, ok := client.files[fileID]
	if !ok {
		return nil, errors.New("missing")
	}
	return &file, nil
}

func (client *fakeCopyClient) ListCopyPage(parentID string, offset, limit int64) (CopyPage, error) {
	client.listOffsets = append(client.listOffsets, offset)
	category := client.category
	if category == "" {
		category = parentID
	}
	count := client.count
	if !client.useCount {
		count = 0
		for _, page := range client.pages {
			count += len(page)
		}
	}
	reportedOffset := int(offset)
	if client.reportedOffset != nil {
		reportedOffset = *client.reportedOffset
	}
	return CopyPage{Files: append([]driver.File(nil), client.pages[offset]...), CategoryID: category,
		Count: count, Offset: reportedOffset, Limit: int(limit)}, nil
}

func (client *fakeCopyClient) Mkdir(string, string) (string, error) {
	client.mkdirCalls++
	return client.mkdirResult, client.mkdirErr
}

func (client *fakeCopyClient) Copy(target string, sources ...string) error {
	client.copyCalls++
	client.copyTarget = target
	if len(sources) > 0 {
		client.copySource = sources[0]
	}
	return client.copyErr
}

func copyDriverFile(id, parent string) driver.File {
	return driver.File{FileID: id, ParentID: parent, Name: id + ".bin", Size: 1, Sha1: "0123456789abcdef0123456789abcdef01234567"}
}

func TestCookiedBackendListFilesUsesActualProgress(t *testing.T) {
	client := &fakeCopyClient{
		files: map[string]driver.File{"parent": {FileID: "parent", IsDirectory: true}},
		pages: map[int64][]driver.File{
			0: {copyDriverFile("a", "parent"), copyDriverFile("b", "parent")},
			2: {copyDriverFile("c", "parent")},
			3: {},
		},
	}
	backend := &CookiedBackend{copyClient: client}
	files, err := backend.ListFiles(context.Background(), "parent")
	if err != nil || len(files) != 3 {
		t.Fatalf("files = %#v, error = %v", files, err)
	}
	want := []int64{0, 2}
	for index := range want {
		if client.listOffsets[index] != want[index] {
			t.Fatalf("offsets = %v, want %v", client.listOffsets, want)
		}
	}
}

func TestCookiedBackendListFilesRejectsWrongParentAndRepeatedPage(t *testing.T) {
	client := &fakeCopyClient{
		files: map[string]driver.File{"parent": {FileID: "parent", IsDirectory: true}},
		pages: map[int64][]driver.File{0: {copyDriverFile("a", "parent")}, 1: {copyDriverFile("a", "parent")}},
	}
	backend := &CookiedBackend{copyClient: client}
	if _, err := backend.ListFiles(context.Background(), "parent"); !errors.Is(err, ErrObjectList) {
		t.Fatalf("error = %v, want ErrObjectList", err)
	}
	client.files["parent"] = driver.File{FileID: "other", IsDirectory: true}
	if _, err := backend.ListFiles(context.Background(), "parent"); !errors.Is(err, ErrObjectIdentity) {
		t.Fatalf("error = %v, want ErrObjectIdentity", err)
	}
}

func TestCookiedBackendListFilesRejectsResponseCIDMismatch(t *testing.T) {
	client := &fakeCopyClient{
		files: map[string]driver.File{"parent": {FileID: "parent", IsDirectory: true}},
		pages: map[int64][]driver.File{0: {}}, category: "other", useCount: true, count: 0,
	}
	backend := &CookiedBackend{copyClient: client}
	if _, err := backend.ListFiles(context.Background(), "parent"); !errors.Is(err, ErrObjectIdentity) {
		t.Fatalf("error = %v, want ErrObjectIdentity", err)
	}
}

func TestCookiedBackendListFilesRejectsEarlyEmptyPage(t *testing.T) {
	client := &fakeCopyClient{
		files: map[string]driver.File{"parent": {FileID: "parent", IsDirectory: true}},
		pages: map[int64][]driver.File{0: {}}, useCount: true, count: 1,
	}
	backend := &CookiedBackend{copyClient: client}
	if _, err := backend.ListFiles(context.Background(), "parent"); !errors.Is(err, ErrObjectList) {
		t.Fatalf("error = %v, want ErrObjectList", err)
	}
}

func TestCookiedBackendListFilesAcceptsProvenEmptyDirectory(t *testing.T) {
	client := &fakeCopyClient{
		files: map[string]driver.File{"parent": {FileID: "parent", IsDirectory: true}},
		pages: map[int64][]driver.File{0: {}}, useCount: true, count: 0,
	}
	backend := &CookiedBackend{copyClient: client}
	files, err := backend.ListFiles(context.Background(), "parent")
	if err != nil || len(files) != 0 {
		t.Fatalf("files = %#v, error = %v", files, err)
	}
}

func TestCookiedBackendListFilesRejectsWrongOffset(t *testing.T) {
	offset := 99
	client := &fakeCopyClient{
		files: map[string]driver.File{"parent": {FileID: "parent", IsDirectory: true}},
		pages: map[int64][]driver.File{0: {}}, reportedOffset: &offset, useCount: true, count: 0,
	}
	backend := &CookiedBackend{copyClient: client}
	if _, err := backend.ListFiles(context.Background(), "parent"); !errors.Is(err, ErrObjectList) {
		t.Fatalf("error = %v, want ErrObjectList", err)
	}
}

func TestCookiedBackendCopyFileCallsProviderOnce(t *testing.T) {
	client := &fakeCopyClient{copyErr: errors.New("timeout")}
	backend := &CookiedBackend{copyClient: client}
	if err := backend.CopyFile(context.Background(), "target", "source"); err == nil {
		t.Fatal("expected provider error")
	}
	if client.copyCalls != 1 || client.copyTarget != "target" || client.copySource != "source" {
		t.Fatalf("calls = %d, target = %q, source = %q", client.copyCalls, client.copyTarget, client.copySource)
	}
}
