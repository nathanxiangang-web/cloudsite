package product

import (
	"context"
	"testing"

	"github.com/nathanxiangang-web/panta/internal/catalog"
	"github.com/nathanxiangang-web/panta/internal/integrations/indexcore"
)

type batchCopies struct {
	calls int
	rows  map[string]catalog.Copy
}

func (b *batchCopies) GetCopyByPhysicalIdentity(context.Context, string, string) (catalog.Copy, error) {
	panic("N+1 lookup")
}
func (b *batchCopies) GetCopiesByPhysicalIdentity(context.Context, string, []string) (map[string]catalog.Copy, error) {
	b.calls++
	return b.rows, nil
}
func TestProductBrowseUsesOneIdentityBatchAndRejectsForeignRows(t *testing.T) {
	service, f := newFixture(t)
	f.page = indexcore.ResourcePage{Items: []indexcore.ResourceContext{physical()}}
	batch := &batchCopies{rows: map[string]catalog.Copy{}}
	service.copies = batch
	page, err := service.Browse(context.Background(), principal(), BrowseRequest{BindingID: bindingID, Limit: 50})
	if err != nil || len(page.Items) != 1 || batch.calls != 1 {
		t.Fatal(page, err, batch.calls)
	}
	batch.rows = map[string]catalog.Copy{"foreign": {IndexCoreRootID: "another", IndexCoreResourceID: "foreign"}}
	if _, err = service.Browse(context.Background(), principal(), BrowseRequest{BindingID: bindingID, Limit: 50}); err == nil {
		t.Fatal("foreign physical identity accepted")
	}
}
