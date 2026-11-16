package acquisition

import (
	"context"
	"errors"
	"testing"

	"github.com/nathanxiangang-web/panta/internal/storage"
)

type privateTargetReaderDouble struct {
	space PrivateTargetSpace
	err   error
}

func (reader privateTargetReaderDouble) GetPrivateTargetSpace(context.Context, string) (PrivateTargetSpace, error) {
	if reader.err != nil {
		return PrivateTargetSpace{}, reader.err
	}
	return reader.space, nil
}

func TestValidatePrivateTargetSpace(t *testing.T) {
	owner := UserID("owner-a")
	space := PrivateTargetSpace{
		ID: "space-a", OwnerUserID: string(owner), BindingID: "binding-a", ProviderConnectionID: "connection-a",
		IndexCoreRootID: "root-a", NativeDirID: "directory-a", CanonicalPrefix: "/云下载/_users/a",
		Status: PrivateTargetStatusActive, OwnerState: PrivateTargetOwnerActive,
	}
	binding := storage.Binding{ID: "binding-a", ConnectionID: "connection-a", IndexCoreRootID: "root-a"}
	connection := storage.Connection{ID: "connection-a"}
	if err := ValidatePrivateTargetSpace(space, "space-a", binding, connection, "directory-a", "/云下载/_users/a", &owner); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*PrivateTargetSpace){
		"wrong owner":            func(value *PrivateTargetSpace) { value.OwnerUserID = "owner-b" },
		"wrong binding":          func(value *PrivateTargetSpace) { value.BindingID = "binding-b" },
		"wrong connection":       func(value *PrivateTargetSpace) { value.ProviderConnectionID = "connection-b" },
		"wrong root":             func(value *PrivateTargetSpace) { value.IndexCoreRootID = "root-b" },
		"wrong native directory": func(value *PrivateTargetSpace) { value.NativeDirID = "directory-b" },
		"wrong canonical prefix": func(value *PrivateTargetSpace) { value.CanonicalPrefix = "/云下载/_users/b" },
		"blocked space":          func(value *PrivateTargetSpace) { value.Status = "BLOCKED" },
		"deleted owner":          func(value *PrivateTargetSpace) { value.OwnerState = "OWNER_DELETED" },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := space
			mutate(&candidate)
			if err := ValidatePrivateTargetSpace(candidate, "space-a", binding, connection, "directory-a", "/云下载/_users/a", &owner); !errors.Is(err, ErrExecutionIdentityMismatch) {
				t.Fatalf("error = %v, want ErrExecutionIdentityMismatch", err)
			}
		})
	}
	if err := ValidatePrivateTargetSpace(space, "space-a", binding, connection, "directory-a", "/云下载/_users/a", nil); !errors.Is(err, ErrExecutionIdentityMismatch) {
		t.Fatalf("nil owner error = %v, want ErrExecutionIdentityMismatch", err)
	}
}
