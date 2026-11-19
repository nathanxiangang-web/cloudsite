package p115

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/SheltonZhu/115driver/pkg/driver"
	"github.com/nathanxiangang-web/panta/internal/providers/contracts"
)

const (
	copyListPageSize = int64(100)
	copyListMaxItems = 10000
)

type CopyPage struct {
	Files      []driver.File
	CategoryID string
	Count      int
	Offset     int
	Limit      int
}

type CopyClient interface {
	GetFile(string) (*driver.File, error)
	ListCopyPage(string, int64, int64) (CopyPage, error)
	Copy(string, ...string) error
	Mkdir(string, string) (string, error)
}

type nativeCopyClient struct {
	client *driver.Pan115Client
}

func (client *nativeCopyClient) GetFile(fileID string) (*driver.File, error) {
	return client.client.GetFile(fileID)
}

func (client *nativeCopyClient) ListCopyPage(parentID string, offset, limit int64) (CopyPage, error) {
	response, err := driver.GetFiles(client.client.NewRequest().ForceContentType("application/json;charset=UTF-8"),
		parentID, driver.WithApiURL(driver.ApiFileList), driver.WithLimit(limit), driver.WithOffset(offset))
	if err != nil {
		return CopyPage{}, err
	}
	page := CopyPage{CategoryID: string(response.CategoryID), Count: response.Count,
		Offset: response.Offset, Limit: response.Limit, Files: make([]driver.File, 0, len(response.Files))}
	for _, fileInfo := range response.Files {
		page.Files = append(page.Files, *(&driver.File{}).From(&fileInfo))
	}
	return page, nil
}

func (client *nativeCopyClient) Copy(destinationID string, fileIDs ...string) error {
	return client.client.Copy(destinationID, fileIDs...)
}

func (client *nativeCopyClient) Mkdir(parentID string, name string) (string, error) {
	return client.client.Mkdir(parentID, name)
}

var _ CopyBackend = (*CookiedBackend)(nil)

func (backend *CookiedBackend) InspectFile(ctx context.Context, fileID string) (FileEvidence, error) {
	if err := ctx.Err(); err != nil {
		return FileEvidence{}, err
	}
	if backend.copyClient == nil {
		return FileEvidence{}, contracts.ErrUnsupportedCapability
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	file, err := backend.copyClient.GetFile(fileID)
	if err != nil {
		return FileEvidence{}, err
	}
	if file == nil {
		return FileEvidence{}, ErrInvalidObjectEvidence
	}
	return evidenceFromDriverFile(*file), nil
}

func (backend *CookiedBackend) ListFiles(ctx context.Context, parentID string) ([]FileEvidence, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if backend.copyClient == nil {
		return nil, contracts.ErrUnsupportedCapability
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	parent, err := backend.copyClient.GetFile(parentID)
	if err != nil {
		return nil, err
	}
	if parent == nil || !parent.IsDirectory || parent.FileID != parentID {
		return nil, ErrObjectIdentity
	}
	result := make([]FileEvidence, 0)
	seenPages := make(map[string]struct{})
	expectedCount := -1
	for offset := int64(0); offset < copyListMaxItems; {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		page, err := backend.copyClient.ListCopyPage(parentID, offset, copyListPageSize)
		if err != nil {
			return nil, err
		}
		if page.CategoryID != parentID {
			return nil, ErrObjectIdentity
		}
		if page.Offset != int(offset) || page.Limit <= 0 || page.Count < 0 {
			return nil, ErrObjectList
		}
		if expectedCount < 0 {
			expectedCount = page.Count
		}
		if page.Count != expectedCount || page.Count > copyListMaxItems || offset > int64(page.Count) {
			return nil, ErrObjectList
		}
		if len(page.Files) == 0 {
			if offset == int64(page.Count) {
				return result, nil
			}
			return nil, ErrObjectList
		}
		ids := make([]string, 0, len(page.Files))
		for _, file := range page.Files {
			ids = append(ids, file.FileID)
		}
		sort.Strings(ids)
		fingerprint := fmt.Sprintf("%v", ids)
		if _, exists := seenPages[fingerprint]; exists {
			return nil, ErrObjectList
		}
		seenPages[fingerprint] = struct{}{}
		for _, file := range page.Files {
			result = append(result, evidenceFromDriverFile(file))
			if len(result) > copyListMaxItems {
				return nil, ErrObjectList
			}
		}
		next := offset + int64(len(page.Files))
		if next <= offset {
			return nil, ErrObjectList
		}
		offset = next
		if offset >= int64(page.Count) {
			if offset != int64(page.Count) {
				return nil, ErrObjectList
			}
			return result, nil
		}
	}
	return nil, ErrObjectList
}

func (backend *CookiedBackend) CopyFile(ctx context.Context, destinationID string, fileID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if backend.copyClient == nil {
		return contracts.ErrUnsupportedCapability
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if err := backend.copyClient.Copy(destinationID, fileID); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

func evidenceFromDriverFile(file driver.File) FileEvidence {
	return FileEvidence{
		FileID: file.FileID, ParentID: file.ParentID, Name: file.Name,
		SizeBytes: file.Size, SHA1: file.Sha1, IsDirectory: file.IsDirectory,
	}
}
