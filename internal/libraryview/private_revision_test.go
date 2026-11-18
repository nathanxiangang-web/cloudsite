package libraryview

import (
	"context"
	"testing"
	"time"

	"github.com/nathanxiangang-web/panta/internal/integrations/indexcore"
)

func TestLibraryRevisionDoesNotChangeWithPrivateActivity(t *testing.T) {
	ctx := context.Background()
	engine, fixture := setup(t, 0, 0)
	public := row(1, false)
	publicPath := "/shared/public.bin"
	public.CanonicalPath = &publicPath
	visibleAt := time.Unix(100, 0).UTC()
	public.ModifiedAt = &visibleAt
	private := row(2, false)
	privatePath := "/云下载/_users/b/secret.bin"
	private.CanonicalPath = &privatePath
	privateAt := time.Unix(200, 0).UTC()
	private.ModifiedAt = &privateAt
	fixture.inventory = []indexcore.ResourceContext{public, private}
	if err := engine.Sync(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	first, err := engine.Page(query())
	if err != nil {
		t.Fatal(err)
	}
	privateAt = time.Unix(300, 0).UTC()
	private.ModifiedAt = &privateAt
	private.ResourceID = "new-private-id"
	fixture.inventory[1] = private
	if err := engine.Sync(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	second, err := engine.Page(query())
	if err != nil {
		t.Fatal(err)
	}
	if first.Revision != second.Revision {
		t.Fatal("private activity changed the public revision")
	}
	if !first.UpdatedAt.IsZero() || !second.UpdatedAt.IsZero() {
		t.Fatalf("private activity changed visible updated_at: first=%v second=%v", first.UpdatedAt, second.UpdatedAt)
	}
	if first.Counts == nil || first.Counts.All != 1 || second.Counts == nil || second.Counts.All != 1 {
		t.Fatalf("private resources leaked into counts: first=%#v second=%#v", first.Counts, second.Counts)
	}
}
