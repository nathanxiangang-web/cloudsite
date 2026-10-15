package producthttp

import (
	"context"
	"encoding/json"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/product"
	"github.com/nathanxiangang-web/panta/internal/storage"
	"testing"
)

type wholeReadSpy struct {
	product.Reads
	last product.BrowseRequest
}

func (s *wholeReadSpy) Browse(_ context.Context, _ auth.Principal, r product.BrowseRequest) (product.ResourcePage, error) {
	s.last = r
	return product.ResourcePage{Items: []product.Resource{}}, nil
}
func TestConsumerWholeRootSelectionKeepsAdminAndOldReadScopes(t *testing.T) {
	old := workspaceBinding
	other := DisplayBinding{ID: storage.BindingID("94000000-0000-4000-8000-000000000001"), Label: "CloudSite"}
	p := goodPrincipal()
	p.Role = auth.RoleAdmin
	spy := &wholeReadSpy{}
	h, err := NewWorkspaceHandler(&fakeAccess{p: p}, "http://127.0.0.1:8180", spy, []DisplayBinding{old, other}, 14, WithLibraryScope([]storage.BindingID{other.ID}, true))
	if err != nil {
		t.Fatal(err)
	}
	var consumer, admin workspaceInfo
	w := request(h, "GET", "/api/v1/workspace", "", testToken, "")
	json.Unmarshal(w.Body.Bytes(), &consumer)
	w = request(h, "GET", "/api/v1/admin/workspace", "", testToken, "")
	json.Unmarshal(w.Body.Bytes(), &admin)
	if len(consumer.Bindings) != 1 || consumer.Bindings[0].ID != other.ID || !consumer.WholeRootLibrary || len(admin.Bindings) != 2 {
		t.Fatal("scope separation")
	}
	w = request(h, "GET", "/api/v1/resources?binding_id="+string(other.ID)+"&whole_root=true", "", testToken, "")
	if w.Code != 200 || !spy.last.WholeRoot {
		t.Fatal("whole-root transport", w.Code)
	}
	w = request(h, "GET", "/api/v1/resources?binding_id="+string(other.ID)+"&whole_root=false", "", testToken, "")
	if w.Code != 400 {
		t.Fatal("ambiguous mode")
	}
	if _, err = NewWorkspaceHandler(&fakeAccess{p: p}, "http://127.0.0.1:8180", spy, []DisplayBinding{old}, 14, WithLibraryScope([]storage.BindingID{other.ID}, true)); err == nil {
		t.Fatal("unconfigured selection")
	}
}
