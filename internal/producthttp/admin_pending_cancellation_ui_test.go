package producthttp

import (
	"encoding/json"
	"testing"

	"github.com/nathanxiangang-web/panta/internal/adminops"
	"github.com/nathanxiangang-web/panta/internal/adminread"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

type cancellationCapabilityFake struct {
	operationsFake
	enabled bool
}

func (f *cancellationCapabilityFake) PendingCancellationEnabled() bool { return f.enabled }

func TestPendingCancellationCapabilityRequiresActualPortsAndSchema19(t *testing.T) {
	for _, tc := range []struct {
		name    string
		version int64
		ops     adminops.Operations
		reads   adminread.Reads
		want    bool
	}{
		{"ready", 19, &cancellationCapabilityFake{enabled: true}, &downloadHTTPFake{}, true},
		{"schema16", 16, &cancellationCapabilityFake{enabled: true}, &downloadHTTPFake{}, false},
		{"no-operation-port", 19, nil, &downloadHTTPFake{}, false},
		{"legacy-operation-port", 19, &operationsFake{}, &downloadHTTPFake{}, false},
		{"unsupported-operation-port", 19, &cancellationCapabilityFake{}, &downloadHTTPFake{}, false},
		{"no-read-port", 19, &cancellationCapabilityFake{enabled: true}, nil, false},
		{"legacy-read-port", 19, &cancellationCapabilityFake{enabled: true}, &adminReadsFake{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			actor := goodPrincipal()
			actor.Role = auth.RoleAdmin
			h, e := NewWorkspaceHandler(&fakeAccess{p: actor}, "http://127.0.0.1:8180", &readAccessFake{}, []DisplayBinding{{ID: storage.BindingID("98000000-0000-4000-8000-000000000002"), Label: "Fixture"}}, tc.version, WithAdminOperations(tc.ops), WithAdminReads(tc.reads))
			if e != nil {
				t.Fatal(e)
			}
			w := request(h, "GET", "/api/v1/admin/workspace", "", testToken, "")
			var d struct {
				Enabled bool `json:"admin_pending_cancellation_enabled"`
			}
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &d) != nil || d.Enabled != tc.want {
				t.Fatal(w.Code, w.Body.String())
			}
			asset := request(h, "GET", "/assets/admin-pending-cancel.js", "", "", "")
			if asset.Code != 200 {
				t.Fatal("missing embedded cancellation module", asset.Code)
			}
		})
	}
}
