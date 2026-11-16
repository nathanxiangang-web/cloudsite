package product

import (
	"context"
	"testing"

	"github.com/nathanxiangang-web/panta/internal/storage"
)

func TestPrivateAdmissionDigestIsSeparateAndStable(t *testing.T) {
	request := AdmissionRequest{OperationKey: "key", Source: "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567", BindingID: "95000000-0000-4000-8000-000000000001", TargetPath: "/云下载/_users/a"}
	legacy, err := AdmissionDigest(request)
	if err != nil {
		t.Fatal(err)
	}
	spaceID := "space-a"
	connectionID := storage.ConnectionID("connection-a")
	scope := "directory-a"
	request.UserSpaceID = &spaceID
	request.TargetProviderConnectionID = &connectionID
	request.TargetProviderScope = &scope
	private, err := AdmissionDigest(request)
	if err != nil {
		t.Fatal(err)
	}
	if legacy == private {
		t.Fatal("private intent reused the legacy digest domain")
	}
	again, err := AdmissionDigest(request)
	if err != nil || again != private {
		t.Fatalf("private digest is not deterministic: %v", err)
	}
}

func TestPrivateAdmissionRejectsPartialFrozenTarget(t *testing.T) {
	request := AdmissionRequest{OperationKey: "key", Source: "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567", BindingID: "95000000-0000-4000-8000-000000000001", TargetPath: "/云下载/_users/a"}
	spaceID := "space-a"
	request.UserSpaceID = &spaceID
	if err := ValidateAdmissionRequest(request); err == nil {
		t.Fatal("partial private target accepted")
	}
}

func TestCloudSpaceResourceAuthorizerOwnerIsolation(t *testing.T) {
	principal := cloudSpaceTestPrincipal()
	store := &cloudSpaceStoreFake{space: CloudUserSpace{
		ID: "space-a", OwnerUserID: string(principal.UserID), BindingID: "binding-a", ProviderConnectionID: "connection-a",
		IndexCoreRootID: "root-a", NativeParentID: "parent", CanonicalPrefix: "/云下载/_users/a",
		DirectoryName: "a", Status: UserSpaceActive, OwnerState: UserSpaceOwnerActive,
	}}
	authorizer := CloudSpaceResourceAuthorizer{Spaces: store}
	allowed, err := authorizer.Allowed(context.Background(), principal, "binding-a", "/云下载/_users/a/result.bin")
	if err != nil || !allowed {
		t.Fatalf("owner path rejected: allowed=%t error=%v", allowed, err)
	}
	allowed, err = authorizer.Allowed(context.Background(), principal, "binding-a", "/云下载/_users/b/result.bin")
	if err != nil || allowed {
		t.Fatalf("other user path allowed: allowed=%t error=%v", allowed, err)
	}
	allowed, err = authorizer.Allowed(context.Background(), principal, "binding-a", "/shared/result.bin")
	if err != nil || !allowed {
		t.Fatalf("public path rejected: allowed=%t error=%v", allowed, err)
	}
}
