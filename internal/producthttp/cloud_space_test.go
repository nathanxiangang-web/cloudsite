package producthttp

import (
	"context"

	"testing"

	"github.com/nathanxiangang-web/panta/internal/acquisition"
	"github.com/nathanxiangang-web/panta/internal/product"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

type cloudSpaceHTTPStore struct {
	space product.CloudUserSpace
}

func (store *cloudSpaceHTTPStore) CreateSpace(_ context.Context, space product.CloudUserSpace) error {
	store.space = space
	return nil
}

func (store *cloudSpaceHTTPStore) GetSpace(context.Context, product.UserSpaceID) (product.CloudUserSpace, error) {
	if store.space.ID == "" {
		return product.CloudUserSpace{}, product.ErrUserSpaceNotFound
	}
	return store.space, nil
}

func (store *cloudSpaceHTTPStore) GetSpaceByOwner(context.Context, string, storage.BindingID) (product.CloudUserSpace, error) {
	if store.space.ID == "" {
		return product.CloudUserSpace{}, product.ErrUserSpaceNotFound
	}
	return store.space, nil
}

func (store *cloudSpaceHTTPStore) UpdateSpace(_ context.Context, space product.CloudUserSpace, _ int64) error {
	store.space = space
	return nil
}

type cloudSpaceHTTPProvider struct {
	createCalls int
}

func (provider *cloudSpaceHTTPProvider) FindDirectory(context.Context, string, string) (product.CloudDirectoryMatch, error) {
	return product.CloudDirectoryMatch{}, nil
}

func (provider *cloudSpaceHTTPProvider) CreateDirectory(context.Context, string, string) (string, error) {
	provider.createCalls++
	return "new-directory", nil
}

func TestCloudSpaceV2GetAndProvision(t *testing.T) {
	store := &cloudSpaceHTTPStore{}
	provider := &cloudSpaceHTTPProvider{}
	service, err := product.NewCloudSpaceService(store, provider)
	if err != nil {
		t.Fatal(err)
	}
	target := product.CloudSpaceTarget{BindingID: workspaceBinding.ID, ProviderConnectionID: "connection-a",
		IndexCoreRootID: "root-a", NativeParentID: "native-parent", CanonicalBase: "/operator-target"}
	handler, err := NewWorkspaceHandler(&fakeAccess{p: goodPrincipal()}, "http://127.0.0.1:8180", &readAccessFake{},
		[]DisplayBinding{workspaceBinding}, 22, WithCloudSpace(service, target))
	if err != nil {
		t.Fatal(err)
	}
	if response := request(handler, "GET", "/api/v2/cloud-downloads/space", "", testToken, "http://127.0.0.1:8180"); response.Code != 200 {
		t.Fatalf("GET space status = %d, body = %s", response.Code, response.Body.String())
	}
	if provider.createCalls != 0 {
		t.Fatal("GET created a directory")
	}
	body := `{"operation_key":"once"}`
	created := request(handler, "POST", "/api/v2/cloud-downloads/space/provision", body, testToken, "http://127.0.0.1:8180")
	if created.Code != 200 || provider.createCalls != 1 {
		t.Fatalf("provision status = %d, calls = %d, body = %s", created.Code, provider.createCalls, created.Body.String())
	}
	replayed := request(handler, "POST", "/api/v2/cloud-downloads/space/provision", body, testToken, "http://127.0.0.1:8180")
	if replayed.Code != 200 || provider.createCalls != 1 {
		t.Fatalf("replay status = %d, calls = %d", replayed.Code, provider.createCalls)
	}
	invalid := request(handler, "POST", "/api/v2/cloud-downloads/space/provision", `{"operation_key":""}`, testToken, "http://127.0.0.1:8180")
	if invalid.Code != 400 || provider.createCalls != 1 {
		t.Fatalf("invalid status = %d, calls = %d", invalid.Code, provider.createCalls)
	}
}

func TestCloudSpaceV2RejectsMalformedProvision(t *testing.T) {
	store := &cloudSpaceHTTPStore{}
	provider := &cloudSpaceHTTPProvider{}
	service, err := product.NewCloudSpaceService(store, provider)
	if err != nil {
		t.Fatal(err)
	}
	target := product.CloudSpaceTarget{BindingID: workspaceBinding.ID, ProviderConnectionID: "connection-a",
		IndexCoreRootID: "root-a", NativeParentID: "native-parent", CanonicalBase: "/operator-target"}
	handler, err := NewWorkspaceHandler(&fakeAccess{p: goodPrincipal()}, "http://127.0.0.1:8180", &readAccessFake{},
		[]DisplayBinding{workspaceBinding}, 22, WithCloudSpace(service, target))
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{`{}`, `null`, `{"operation_key":"a","extra":true}`, `{"operation_key":"a"}{}`} {
		response := request(handler, "POST", "/api/v2/cloud-downloads/space/provision", body, testToken, "http://127.0.0.1:8180")
		if response.Code != 400 {
			t.Fatalf("body = %q, status = %d", body, response.Code)
		}
	}
	if provider.createCalls != 0 {
		t.Fatal("malformed provision performed a write")
	}
}
func TestCloudSpaceV2SubmitUsesFrozenPrivateTarget(t *testing.T) {
	store := &cloudSpaceHTTPStore{}
	provider := &cloudSpaceHTTPProvider{}
	service, err := product.NewCloudSpaceService(store, provider)
	if err != nil {
		t.Fatal(err)
	}
	target := product.CloudSpaceTarget{BindingID: workspaceBinding.ID, ProviderConnectionID: "connection-a",
		IndexCoreRootID: "root-a", NativeParentID: "native-parent", CanonicalBase: "/operator-target"}
	admissions := &admissionFake{}
	handler, err := NewWorkspaceHandler(&fakeAccess{p: goodPrincipal()}, "http://127.0.0.1:8180", &readAccessFake{},
		[]DisplayBinding{workspaceBinding}, 22, WithCloudSpace(service, target, admissions))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Provision(context.Background(), goodPrincipal(), target, "provision-once"); err != nil {
		t.Fatal(err)
	}
	body := `{"operation_key":"x","source":"https://example.invalid/PRIVATE"}`
	response := request(handler, "POST", "/api/v2/cloud-downloads", body, testToken, "http://127.0.0.1:8180")
	if response.Code != 201 || admissions.calls != 1 {
		t.Fatalf("submit status = %d, calls = %d, body = %s", response.Code, admissions.calls, response.Body.String())
	}
	input := admissions.input
	if input.BindingID != workspaceBinding.ID || input.TargetPath != store.space.CanonicalPrefix ||
		input.UserSpaceID == nil || *input.UserSpaceID != string(store.space.ID) ||
		input.TargetProviderConnectionID == nil || *input.TargetProviderConnectionID != "connection-a" ||
		input.TargetProviderScope == nil || *input.TargetProviderScope != "new-directory" {
		t.Fatalf("frozen input = %#v", input)
	}
	admissions.status = acquisition.SubmissionReplay
	replayed := request(handler, "POST", "/api/v2/cloud-downloads", body, testToken, "http://127.0.0.1:8180")
	if replayed.Code != 200 {
		t.Fatalf("replay status = %d, body = %s", replayed.Code, replayed.Body.String())
	}
}

func TestCloudSpaceV2SubmitRejectsNonActiveSpace(t *testing.T) {
	store := &cloudSpaceHTTPStore{}
	provider := &cloudSpaceHTTPProvider{}
	service, err := product.NewCloudSpaceService(store, provider)
	if err != nil {
		t.Fatal(err)
	}
	target := product.CloudSpaceTarget{BindingID: workspaceBinding.ID, ProviderConnectionID: "connection-a",
		IndexCoreRootID: "root-a", NativeParentID: "native-parent", CanonicalBase: "/operator-target"}
	admissions := &admissionFake{}
	handler, err := NewWorkspaceHandler(&fakeAccess{p: goodPrincipal()}, "http://127.0.0.1:8180", &readAccessFake{},
		[]DisplayBinding{workspaceBinding}, 22, WithCloudSpace(service, target, admissions))
	if err != nil {
		t.Fatal(err)
	}
	body := `{"operation_key":"x","source":"https://example.invalid/PRIVATE"}`
	response := request(handler, "POST", "/api/v2/cloud-downloads", body, testToken, "http://127.0.0.1:8180")
	if response.Code == 201 || admissions.calls != 0 {
		t.Fatalf("non-active submit status = %d, calls = %d", response.Code, admissions.calls)
	}
}
