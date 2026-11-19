package product

import (
	"context"
	"errors"
	"testing"

	"github.com/nathanxiangang-web/panta/internal/acquisition"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/catalog"
	"github.com/nathanxiangang-web/panta/internal/integrations/indexcore"
	"github.com/nathanxiangang-web/panta/internal/resourceview"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

const privateFilesBindingID storage.BindingID = "95000000-0000-4000-8000-000000000001"

type privateFilesTaskReader struct{}

func (privateFilesTaskReader) ListTasks(context.Context, auth.UserID, []storage.BindingID, int, string) ([]TaskRecord, error) {
	return nil, nil
}
func (privateFilesTaskReader) GetTask(context.Context, auth.UserID, []storage.BindingID, acquisition.ManifestID) (TaskRecord, error) {
	return TaskRecord{}, ErrNotFound
}

type privateFilesTopology struct {
	binding storage.Binding
}

func (topology privateFilesTopology) GetBinding(context.Context, storage.BindingID) (storage.Binding, error) {
	return topology.binding, nil
}

type privateFilesIndex struct {
	page        indexcore.ResourcePage
	browseCalls int
}

func (index *privateFilesIndex) Resolve(context.Context, indexcore.ResolveRequest) (indexcore.ResolveResult, error) {
	return indexcore.ResolveResult{}, nil
}
func (index *privateFilesIndex) Browse(context.Context, indexcore.BrowseRequest) (indexcore.ResourcePage, error) {
	index.browseCalls++
	return index.page, nil
}

type privateFilesPhysical struct {
	prefix string
}

func (physical privateFilesPhysical) Resolve(context.Context, resourceview.PhysicalRequest) (resourceview.PhysicalDetail, error) {
	directory := true
	return resourceview.PhysicalDetail{Matches: []resourceview.PhysicalMatch{{Physical: indexcore.ResourceContext{
		ResourceID: "private-dir", RootID: "root-a", CanonicalPath: &physical.prefix, Directory: &directory, Presence: indexcore.ResourcePresent,
	}}}}, nil
}

type privateFilesResolverFunc func(resourceview.PhysicalRequest) (resourceview.PhysicalDetail, error)

func (resolve privateFilesResolverFunc) Resolve(_ context.Context, request resourceview.PhysicalRequest) (resourceview.PhysicalDetail, error) {
	return resolve(request)
}

func privateFilesService(t *testing.T, store *cloudSpaceStoreFake, index *privateFilesIndex) *ReadService {
	t.Helper()
	scope := "native-parent"
	fake := &readFake{binding: storage.Binding{
		ID: privateFilesBindingID, ConnectionID: "connection-a", IndexCoreRootID: "root-a", Status: storage.BindingStatusActive, ProviderScope: &scope,
	}, copyErr: catalog.ErrNotFound}
	service, err := NewReadService(fake, fake, index, privateFilesPhysical{prefix: "/云下载/_users/a"}, fake, []storage.BindingID{privateFilesBindingID},
		WithPrivateResourceAuthorizer(CloudSpaceResourceAuthorizer{Spaces: store}))
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func privateFilesFixture() (auth.Principal, *cloudSpaceStoreFake, *privateFilesIndex, string) {
	principal := cloudSpaceTestPrincipal()
	store := &cloudSpaceStoreFake{space: CloudUserSpace{
		ID: "space-a", OwnerUserID: string(principal.UserID), BindingID: privateFilesBindingID, ProviderConnectionID: "connection-a",
		IndexCoreRootID: "root-a", NativeParentID: "parent", CanonicalPrefix: "/云下载/_users/a",
		DirectoryName: "a", Status: UserSpaceActive, OwnerState: UserSpaceOwnerActive,
	}}
	prefix := "/云下载/_users/a"
	parent := "private-dir"
	name := "result.bin"
	directory := false
	size := int64(42)
	path := prefix + "/result.bin"
	index := &privateFilesIndex{page: indexcore.ResourcePage{Items: []indexcore.ResourceContext{{
		ResourceID: "file-a", RootID: "root-a", CanonicalPath: &path, ParentResourceID: &parent,
		Name: &name, Directory: &directory, SizeBytes: &size, Presence: indexcore.ResourcePresent,
	}}, NextCursor: "inner-next"}}
	return principal, store, index, prefix
}

func TestPrivateFilesUsesOwnerScopeAndBoundCursor(t *testing.T) {
	principal, store, index, prefix := privateFilesFixture()
	service := privateFilesService(t, store, index)
	page, err := service.PrivateFiles(context.Background(), principal, PrivateFilesRequest{BindingID: privateFilesBindingID, Limit: 50})
	if err != nil || len(page.Items) != 1 || page.NextCursor == "" {
		t.Fatalf("page = %#v, error = %v", page, err)
	}
	inner, err := decodePrivateFileCursor(page.NextCursor, principal.UserID, prefix, "private-dir")
	if err != nil || inner != "inner-next" {
		t.Fatalf("inner = %q, error = %v", inner, err)
	}
	other, err := encodePrivateFileCursor(privateFileCursor{Owner: "other", Prefix: prefix, Parent: "private-dir", Inner: "inner-next"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.PrivateFiles(context.Background(), principal, PrivateFilesRequest{BindingID: privateFilesBindingID, Cursor: other, Limit: 50}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("foreign cursor error = %v, want ErrInvalid", err)
	}
}

func TestPrivateFilesRejectsMissingSpaceAndRootMismatch(t *testing.T) {
	principal, store, index, _ := privateFilesFixture()
	store.space = CloudUserSpace{}
	service := privateFilesService(t, store, index)
	if _, err := service.PrivateFiles(context.Background(), principal, PrivateFilesRequest{BindingID: privateFilesBindingID, Limit: 50}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing space error = %v, want ErrNotFound", err)
	}
	if index.browseCalls != 0 {
		t.Fatal("missing space performed a browse")
	}
	_, store, index, _ = privateFilesFixture()
	store.space.IndexCoreRootID = "root-b"
	service = privateFilesService(t, store, index)
	if _, err := service.PrivateFiles(context.Background(), principal, PrivateFilesRequest{BindingID: privateFilesBindingID, Limit: 50}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("root mismatch error = %v, want ErrNotFound", err)
	}
}

func TestPrivateFilesRejectsParentMismatch(t *testing.T) {
	principal, store, index, _ := privateFilesFixture()
	wrong := "other-parent"
	index.page.Items[0].ParentResourceID = &wrong
	service := privateFilesService(t, store, index)
	if _, err := service.PrivateFiles(context.Background(), principal, PrivateFilesRequest{BindingID: privateFilesBindingID, Limit: 50}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("parent mismatch error = %v, want ErrUnavailable", err)
	}
}

func TestPrivateFilesRejectsNonDirectChild(t *testing.T) {
	principal, store, index, prefix := privateFilesFixture()
	nestedPath := prefix + "/nested/result.bin"
	index.page.Items[0].CanonicalPath = &nestedPath
	service := privateFilesService(t, store, index)
	if _, err := service.PrivateFiles(context.Background(), principal, PrivateFilesRequest{BindingID: privateFilesBindingID, Limit: 50}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("nested child error = %v, want ErrUnavailable", err)
	}
}

func TestPrivateFilesNavigatesOnlyWithinOwnSpace(t *testing.T) {
	principal, store, index, prefix := privateFilesFixture()
	service := privateFilesService(t, store, index)
	rootRequest := PrivateFilesRequest{BindingID: privateFilesBindingID, Limit: 50}
	rootPage, err := service.PrivateFiles(context.Background(), principal, rootRequest)
	if err != nil || rootPage.NextCursor == "" {
		t.Fatalf("root page = %#v, error = %v", rootPage, err)
	}
	folder := prefix + "/downloaded-folder"
	file := folder + "/result.bin"
	index.page.Items[0].CanonicalPath = &file
	childParent := "private-child"
	index.page.Items[0].ParentResourceID = &childParent
	resolveCalls := 0
	service.physical = privateFilesResolverFunc(func(request resourceview.PhysicalRequest) (resourceview.PhysicalDetail, error) {
		resolveCalls++
		if request.Path != folder || request.RootID != "root-a" {
			t.Fatalf("unexpected Q5 request = %#v", request)
		}
		detail, _ := (privateFilesPhysical{prefix: folder}).Resolve(context.Background(), request)
		detail.Matches[0].Physical.ResourceID = childParent
		return detail, nil
	})
	for _, candidate := range []struct {
		path string
		err  error
	}{
		{"/云下载/_users/b/folder", ErrNotFound},
		{prefix + "-other/folder", ErrNotFound},
		{prefix + "/../b/folder", ErrInvalid},
		{prefix + "//folder", ErrInvalid},
	} {
		request := rootRequest
		request.Folder = candidate.path
		if _, err := service.PrivateFiles(context.Background(), principal, request); !errors.Is(err, candidate.err) {
			t.Fatalf("folder %q => %v, want %v", candidate.path, err, candidate.err)
		}
	}
	if resolveCalls != 0 || index.browseCalls != 1 {
		t.Fatal("rejected folders touched IndexCore")
	}
	request := rootRequest
	request.Folder, request.Cursor = folder, rootPage.NextCursor
	if _, err := service.PrivateFiles(context.Background(), principal, request); !errors.Is(err, ErrInvalid) || index.browseCalls != 1 {
		t.Fatalf("root cursor accepted in child, error = %v", err)
	}
	request.Cursor = ""
	page, err := service.PrivateFiles(context.Background(), principal, request)
	if err != nil || len(page.Items) != 1 || page.Items[0].Path == nil || *page.Items[0].Path != file || index.browseCalls != 2 {
		t.Fatalf("child page = %#v, error = %v", page, err)
	}
}

func TestPrivateFilesNewSpaceBeforeFirstIndexUpdate(t *testing.T) {
	principal, store, index, _ := privateFilesFixture()
	service := privateFilesService(t, store, index)
	service.physical = privateFilesResolverFunc(func(resourceview.PhysicalRequest) (resourceview.PhysicalDetail, error) {
		return resourceview.PhysicalDetail{}, nil
	})
	page, err := service.PrivateFiles(context.Background(), principal, PrivateFilesRequest{BindingID: privateFilesBindingID, Limit: 50})
	if err != nil || page.Items == nil || len(page.Items) != 0 || index.browseCalls != 0 {
		t.Fatalf("new space page = %#v, error = %v", page, err)
	}
}

func TestGenericBrowseFailsClosedWhenPrivateRowsAreFiltered(t *testing.T) {
	principal, store, index, _ := privateFilesFixture()
	otherPath := "/云下载/_users/b/secret.bin"
	publicPath := "/shared/public.bin"
	index.page = indexcore.ResourcePage{Items: []indexcore.ResourceContext{
		{ResourceID: "private-b", RootID: "root-a", CanonicalPath: &otherPath},
		{ResourceID: "public-a", RootID: "root-a", CanonicalPath: &publicPath, Presence: indexcore.ResourcePresent},
	}}
	service := privateFilesService(t, store, index)
	if _, err := service.Browse(context.Background(), principal, BrowseRequest{WholeRoot: true, BindingID: privateFilesBindingID, Limit: 50}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("mixed private browse error = %v, want ErrNotFound", err)
	}
}
