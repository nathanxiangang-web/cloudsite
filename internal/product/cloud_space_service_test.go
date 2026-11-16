package product

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

type cloudDirectoryProviderFake struct {
	match        CloudDirectoryMatch
	findErr      error
	createErr    error
	createResult string
	findCalls    int
	createCalls  int
}

func (provider *cloudDirectoryProviderFake) FindDirectory(context.Context, string, string) (CloudDirectoryMatch, error) {
	provider.findCalls++
	return provider.match, provider.findErr
}

func (provider *cloudDirectoryProviderFake) CreateDirectory(context.Context, string, string) (string, error) {
	provider.createCalls++
	return provider.createResult, provider.createErr
}

type cloudSpaceStoreFake struct {
	space       CloudUserSpace
	getErr      error
	createErr   error
	updateErr   error
	createCalls int
	updateCalls int
}

func (store *cloudSpaceStoreFake) CreateSpace(_ context.Context, space CloudUserSpace) error {
	store.createCalls++
	store.space = space
	return store.createErr
}

func (store *cloudSpaceStoreFake) GetSpace(context.Context, UserSpaceID) (CloudUserSpace, error) {
	if store.space.ID == "" {
		return CloudUserSpace{}, ErrUserSpaceNotFound
	}
	return store.space, nil
}

func (store *cloudSpaceStoreFake) GetSpaceByOwner(context.Context, string, storage.BindingID) (CloudUserSpace, error) {
	if store.getErr != nil {
		return CloudUserSpace{}, store.getErr
	}
	if store.space.ID == "" {
		return CloudUserSpace{}, ErrUserSpaceNotFound
	}
	return store.space, nil
}

func (store *cloudSpaceStoreFake) UpdateSpace(_ context.Context, space CloudUserSpace, _ int64) error {
	store.updateCalls++
	store.space = space
	return store.updateErr
}

func cloudSpaceTestPrincipal() auth.Principal {
	return auth.Principal{UserID: "00000000-0000-4000-8000-000000000001", Login: "user", Role: auth.RoleUser}
}

func cloudSpaceTestTarget() CloudSpaceTarget {
	return CloudSpaceTarget{BindingID: "binding-a", ProviderConnectionID: "connection-a",
		IndexCoreRootID: "root-a", NativeParentID: "native-parent", CanonicalBase: "/云下载"}
}

func TestCloudSpaceNamesUsePrivateNamespace(t *testing.T) {
	_, prefix, name := cloudSpaceNames(cloudSpaceTestPrincipal(), cloudSpaceTestTarget())
	if !strings.Contains(prefix, "/_users/") || prefix != "/云下载/_users/"+name {
		t.Fatalf("unexpected private namespace: prefix=%q name=%q", prefix, name)
	}
}

func TestCloudSpaceGetDoesNotCreateOrProbe(t *testing.T) {
	store := &cloudSpaceStoreFake{}
	provider := &cloudDirectoryProviderFake{}
	service, _ := NewCloudSpaceService(store, provider)
	result, err := service.GetSpace(context.Background(), cloudSpaceTestPrincipal(), cloudSpaceTestTarget())
	if err != nil || result.Status != UserSpaceNotProvisioned {
		t.Fatalf("result = %#v, error = %v", result, err)
	}
	if provider.findCalls != 0 || provider.createCalls != 0 {
		t.Fatal("GET performed provider calls")
	}
}

func TestCloudSpaceProvisionCreatesOnceAndReplaysActive(t *testing.T) {
	store := &cloudSpaceStoreFake{}
	provider := &cloudDirectoryProviderFake{createResult: "new-directory"}
	service, _ := NewCloudSpaceService(store, provider)
	result, err := service.Provision(context.Background(), cloudSpaceTestPrincipal(), cloudSpaceTestTarget(), "once")
	if err != nil || result.Status != UserSpaceActive || provider.createCalls != 1 {
		t.Fatalf("result = %#v, create = %d, error = %v", result, provider.createCalls, err)
	}
	replayed, err := service.Provision(context.Background(), cloudSpaceTestPrincipal(), cloudSpaceTestTarget(), "once")
	if err != nil || replayed.Status != UserSpaceActive || provider.createCalls != 1 {
		t.Fatalf("replayed = %#v, create = %d, error = %v", replayed, provider.createCalls, err)
	}
}

func TestCloudSpaceProvisionUnknownOnlyReconcilesAfterwards(t *testing.T) {
	store := &cloudSpaceStoreFake{}
	provider := &cloudDirectoryProviderFake{createErr: errors.New("timeout")}
	service, _ := NewCloudSpaceService(store, provider)
	result, err := service.Provision(context.Background(), cloudSpaceTestPrincipal(), cloudSpaceTestTarget(), "once")
	if err != nil || result.Status != UserSpaceReconcileRequired || provider.createCalls != 1 {
		t.Fatalf("result = %#v, create = %d, error = %v", result, provider.createCalls, err)
	}
	provider.createErr = nil
	provider.match = CloudDirectoryMatch{DirectoryID: "existing", ParentID: "native-parent", Name: result.DirectoryName, Found: true}
	replayed, err := service.Provision(context.Background(), cloudSpaceTestPrincipal(), cloudSpaceTestTarget(), "once")
	if err != nil || replayed.Status != UserSpaceActive || provider.createCalls != 1 {
		t.Fatalf("replayed = %#v, create = %d, error = %v", replayed, provider.createCalls, err)
	}
}

func TestCloudSpaceProvisionExistingDirectorySkipsCreate(t *testing.T) {
	store := &cloudSpaceStoreFake{}
	provider := &cloudDirectoryProviderFake{}
	service, _ := NewCloudSpaceService(store, provider)
	_, prefix, name := cloudSpaceNames(cloudSpaceTestPrincipal(), cloudSpaceTestTarget())
	provider.match = CloudDirectoryMatch{DirectoryID: "existing", ParentID: "native-parent", Name: name, Found: true}
	result, err := service.Provision(context.Background(), cloudSpaceTestPrincipal(), cloudSpaceTestTarget(), "once")
	if err != nil || result.Status != UserSpaceActive || result.CanonicalPrefix != prefix || provider.createCalls != 0 {
		t.Fatalf("result = %#v, create = %d, error = %v", result, provider.createCalls, err)
	}
}
