package producthttp

import (
	"context"
	"testing"

	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/product"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

type privateFilesHTTPReader struct {
	owner auth.UserID
}

func (reader privateFilesHTTPReader) PrivateFiles(_ context.Context, principal auth.Principal, _ storage.BindingID, _ string, _ int) (product.ResourcePage, error) {
	if principal.UserID != reader.owner {
		return product.ResourcePage{}, product.ErrNotFound
	}
	return product.ResourcePage{Items: []product.Resource{}}, nil
}

func TestCloudSpaceFilesRouteUsesCurrentPrincipal(t *testing.T) {
	store := &cloudSpaceHTTPStore{}
	provider := &cloudSpaceHTTPProvider{}
	service, err := product.NewCloudSpaceService(store, provider)
	if err != nil {
		t.Fatal(err)
	}
	target := product.CloudSpaceTarget{BindingID: workspaceBinding.ID, ProviderConnectionID: "connection-a",
		IndexCoreRootID: "root-a", NativeParentID: "native-parent", CanonicalBase: "/operator-target"}
	handler, err := NewWorkspaceHandler(&fakeAccess{p: goodPrincipal()}, "http://127.0.0.1:8180", &readAccessFake{},
		[]DisplayBinding{workspaceBinding}, 22, WithCloudSpace(service, target), WithCloudSpaceFiles(privateFilesHTTPReader{owner: goodPrincipal().UserID}))
	if err != nil {
		t.Fatal(err)
	}
	if response := request(handler, "GET", "/api/v2/cloud-downloads/files?limit=50", "", testToken, "http://127.0.0.1:8180"); response.Code != 200 {
		t.Fatalf("owner files status = %d, body = %s", response.Code, response.Body.String())
	}
	if response := request(handler, "GET", "/api/v2/cloud-downloads/files?cursor=opaque&limit=1", "", testToken, "http://127.0.0.1:8180"); response.Code != 200 {
		t.Fatalf("cursor files status = %d, body = %s", response.Code, response.Body.String())
	}
	for _, path := range []string{
		"/api/v2/cloud-downloads/files?limit=0",
		"/api/v2/cloud-downloads/files?limit=",
		"/api/v2/cloud-downloads/files?limit=1&limit=2",
		"/api/v2/cloud-downloads/files?cursor=one&cursor=two",
		"/api/v2/cloud-downloads/files?unknown=value",
	} {
		if response := request(handler, "GET", path, "", testToken, "http://127.0.0.1:8180"); response.Code != 400 {
			t.Fatalf("invalid query %q status = %d", path, response.Code)
		}
	}
}

func TestCloudSpaceFilesRouteRejectsOtherRole(t *testing.T) {
	store := &cloudSpaceHTTPStore{}
	provider := &cloudSpaceHTTPProvider{}
	service, err := product.NewCloudSpaceService(store, provider)
	if err != nil {
		t.Fatal(err)
	}
	target := product.CloudSpaceTarget{BindingID: workspaceBinding.ID, ProviderConnectionID: "connection-a",
		IndexCoreRootID: "root-a", NativeParentID: "native-parent", CanonicalBase: "/operator-target"}
	principal := goodPrincipal()
	principal.UserID = "00000000-0000-4000-8000-000000000002"
	principal.Login = "other-admin"
	principal.Role = auth.RoleAdmin
	handler, err := NewWorkspaceHandler(&fakeAccess{p: principal}, "http://127.0.0.1:8180", &readAccessFake{},
		[]DisplayBinding{workspaceBinding}, 22, WithCloudSpace(service, target), WithCloudSpaceFiles(privateFilesHTTPReader{owner: goodPrincipal().UserID}))
	if err != nil {
		t.Fatal(err)
	}
	response := request(handler, "GET", "/api/v2/cloud-downloads/files", "", testToken, "http://127.0.0.1:8180")
	if response.Code != 404 {
		t.Fatalf("other role files status = %d, body = %s", response.Code, response.Body.String())
	}
}
