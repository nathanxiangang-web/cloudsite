package libraryview

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"testing"

	"github.com/nathanxiangang-web/panta/internal/integrations/indexcore"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

type fixture struct {
	inventory                []indexcore.ResourceContext
	events                   []indexcore.JournalEvent
	browses, journals, saves int
	failPage, failSave       bool
	emptyNext                bool
	churn                    func()
	state                    indexcore.RootLifecycleState
	saved                    Snapshot
}

func (f *fixture) GetBinding(context.Context, storage.BindingID) (storage.Binding, error) {
	return storage.Binding{ID: "one", IndexCoreRootID: "root", Status: storage.BindingStatusActive}, nil
}
func (f *fixture) RootStatus(context.Context, indexcore.RootStatusRequest) (indexcore.RootStatus, error) {
	return indexcore.RootStatus{RootID: "root", LifecycleState: f.state}, nil
}
func (f *fixture) Resolve(context.Context, indexcore.ResolveRequest) (indexcore.ResolveResult, error) {
	return indexcore.ResolveResult{}, nil
}
func (f *fixture) ReadJournal(_ context.Context, q indexcore.JournalRequest) ([]indexcore.JournalEvent, error) {
	f.journals++
	out := []indexcore.JournalEvent{}
	for _, e := range f.events {
		if e.EventSeq > q.AfterSeq && len(out) < q.Limit {
			out = append(out, e)
		}
	}
	return out, nil
}
func (f *fixture) Browse(_ context.Context, q indexcore.BrowseRequest) (indexcore.ResourcePage, error) {
	f.browses++
	if f.emptyNext {
		return indexcore.ResourcePage{Items: []indexcore.ResourceContext{}, NextCursor: "not-complete"}, nil
	}
	if f.failPage {
		return indexcore.ResourcePage{}, ErrUnavailable
	}
	offset := 0
	if q.Cursor != "" {
		offset, _ = strconv.Atoi(q.Cursor)
	}
	end := min(offset+q.Limit, len(f.inventory))
	next := ""
	if end < len(f.inventory) {
		next = strconv.Itoa(end)
	}
	out := append([]indexcore.ResourceContext(nil), f.inventory[offset:end]...)
	if f.churn != nil {
		fn := f.churn
		f.churn = nil
		fn()
	}
	return indexcore.ResourcePage{Items: out, NextCursor: next}, nil
}
func (f *fixture) LoadLibrarySnapshot(context.Context, storage.BindingID, string) (Snapshot, bool, error) {
	return f.saved, f.saved.Revision != "", nil
}
func (f *fixture) SaveLibrarySnapshot(_ context.Context, s Snapshot) error {
	if f.failSave {
		return ErrUnavailable
	}
	f.saves++
	f.saved = s
	return nil
}
func row(n int, dir bool) indexcore.ResourceContext {
	name := fmt.Sprintf("file-%04d.pdf", n)
	path := "/" + name
	return indexcore.ResourceContext{RootID: "root", ResourceID: fmt.Sprintf("id-%04d", n), Name: &name, CanonicalPath: &path, Directory: &dir, Presence: indexcore.ResourcePresent}
}
func setup(t *testing.T, files, dirs int) (*Engine, *fixture) {
	t.Helper()
	f := &fixture{state: indexcore.RootActive}
	for i := 0; i < files; i++ {
		f.inventory = append(f.inventory, row(i, false))
	}
	for i := 0; i < dirs; i++ {
		f.inventory = append(f.inventory, row(files+i, true))
	}
	e, err := New(f, f, f, []storage.BindingID{"one"})
	if err != nil {
		t.Fatal(err)
	}
	return e, f
}
func query() Request {
	return Request{Scopes: []Scope{{BindingID: "one", RootID: "root"}}, Owner: "owner", Category: "all", Limit: 50}
}
func TestLibraryWarmPaginationAndNoChangeCost(t *testing.T) {
	ctx := context.Background()
	e, f := setup(t, 343, 515)
	if err := e.Sync(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	if f.browses != 18 {
		t.Fatal(f.browses)
	}
	q := query()
	n := 0
	for {
		p, err := e.Page(q)
		if err != nil || !p.Ready || p.Counts.All != 343 || len(p.Items) > 50 {
			t.Fatal(p, err)
		}
		n += len(p.Items)
		if p.NextCursor == "" {
			break
		}
		q.Cursor = p.NextCursor
	}
	if n != 343 {
		t.Fatal(n)
	}
	for i := 0; i < 20; i++ {
		if _, err := e.Page(query()); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.Sync(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	if f.browses != 18 || f.saves != 1 {
		t.Fatal("unchanged root enumerated again", f.browses, f.saves)
	}
	restored, _ := New(f, f, f, []storage.BindingID{"one"})
	if err := restored.Restore(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := restored.Page(query())
	if err != nil || p.Counts.All != 343 || f.browses != 18 {
		t.Fatal(p, err)
	}
	t.Logf("858 canonical records: baseline=%d Q6 pages; 20 cached page reads + unchanged sync=0 additional Q6 pages; persistent restore=0 source reads", f.browses)
}
func TestLibraryFailedCandidateAndChurnNeverReplacePublication(t *testing.T) {
	e, f := setup(t, 55, 2)
	ctx := context.Background()
	if err := e.Sync(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	before, _ := e.Page(query())
	f.inventory = append(f.inventory, row(100, false))
	f.events = []indexcore.JournalEvent{{EventSeq: 1}}
	for _, mode := range []string{"read", "save", "churn"} {
		f.failPage = mode == "read"
		f.failSave = mode == "save"
		if mode == "churn" {
			f.churn = func() { f.events = append(f.events, indexcore.JournalEvent{EventSeq: 2}) }
		}
		if e.Sync(ctx, "one") == nil {
			t.Fatal("failed candidate published", mode)
		}
		after, _ := e.Page(query())
		if !after.Stale || after.Revision != before.Revision || after.Counts.All != 55 {
			t.Fatal(mode, after)
		}
	}
	f.failSave = false
	if err := e.Sync(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	after, _ := e.Page(query())
	if after.Stale || after.Counts.All != 56 {
		t.Fatal(after)
	}
}
func TestLibraryScopePartitionSearchAndExpiredCursor(t *testing.T) {
	e, f := setup(t, 70, 0)
	cloud := "/cloud/nested/a.mp4"
	f.inventory[0].CanonicalPath = &cloud
	video := "a.mp4"
	f.inventory[0].Name = &video
	if err := e.Sync(context.Background(), "one"); err != nil {
		t.Fatal(err)
	}
	q := query()
	q.CloudBinding = "one"
	q.CloudPath = "/cloud"
	p, _ := e.Page(q)
	if p.Counts.All != 69 || p.Counts.Video != 0 {
		t.Fatal(p)
	}
	old := p.NextCursor
	q.Downloads = true
	p, _ = e.Page(q)
	if p.Counts.All != 1 || p.Counts.Video != 1 {
		t.Fatal(p)
	}
	q.Downloads = false
	q.Search = "FILE-0069"
	p, _ = e.Page(q)
	if p.Filtered != 1 || p.Counts.All != 69 {
		t.Fatal(p)
	}
	q.Search = ""
	q.Cursor = old
	q.Owner = "different-owner"
	if _, err := e.Page(q); !errors.Is(err, ErrInvalid) {
		t.Fatal("cursor crossed owner", err)
	}
	q.Owner = "owner"
	f.inventory = append(f.inventory, row(100, false))
	f.events = append(f.events, indexcore.JournalEvent{EventSeq: 1})
	if err := e.Sync(context.Background(), "one"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Page(q); !errors.Is(err, ErrStale) {
		t.Fatal("old page mixed with new counts", err)
	}
	q.Cursor = ""
	q.Page = 1
	p, err := e.Page(q)
	if err != nil || p.Offset != 50 || p.Counts.All != 70 {
		t.Fatal("new publication did not retain page number", p, err)
	}
	q = query()
	q.Scopes[0].RootID = "replacement-root"
	p, err = e.Page(q)
	if err != nil || p.Ready || len(p.Items) != 0 || p.Counts != nil {
		t.Fatal("Root rebind exposed old cache", p, err)
	}
}
func TestLibraryCheckpointGapRebuildAndLifecycleFence(t *testing.T) {
	e, f := setup(t, 3, 0)
	if err := e.Sync(context.Background(), "one"); err != nil {
		t.Fatal(err)
	}
	f.events = []indexcore.JournalEvent{{EventSeq: 2}}
	f.inventory = append(f.inventory, row(4, false))
	if err := e.Sync(context.Background(), "one"); err != nil {
		t.Fatal(err)
	}
	p, _ := e.Page(query())
	if p.Counts.All != 4 || f.saved.Sequence != 2 {
		t.Fatal(p)
	}
	f.state = indexcore.RootDeprecated
	if e.Sync(context.Background(), "one") == nil {
		t.Fatal("inactive Root accepted")
	}
	if _, err := e.Page(query()); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
}
func TestLibraryConcurrentRegistrationsAndReaders(t *testing.T) {
	e, f := setup(t, 120, 20)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := e.Sync(context.Background(), "one"); err != nil {
				t.Error(err)
			}
		}()
	}
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 30; n++ {
				p, err := e.Page(query())
				if err != nil || p.Ready && p.Counts.All != 120 {
					t.Error(p, err)
				}
			}
		}()
	}
	wg.Wait()
	if f.saves != 1 || f.browses != 3 {
		t.Fatal("duplicate builder", f.saves, f.browses)
	}
}
func TestLibraryMetadataConfirmationDoesNotChangeContentRevision(t *testing.T) {
	e, f := setup(t, 3, 1)
	if err := e.Sync(context.Background(), "one"); err != nil {
		t.Fatal(err)
	}
	before, _ := e.Page(query())
	f.inventory[0].LastConfirmedGeneration = 500
	f.events = []indexcore.JournalEvent{{EventSeq: 1}}
	if err := e.Sync(context.Background(), "one"); err != nil {
		t.Fatal(err)
	}
	after, _ := e.Page(query())
	if before.Revision != after.Revision || !before.UpdatedAt.Equal(after.UpdatedAt) {
		t.Fatal("confirmation invalidated presentation")
	}
}

type deltaFixture struct {
	*fixture
	exact       int
	unavailable bool
	removed     map[string]indexcore.ResourceContext
}

func (f *deltaFixture) Resource(_ context.Context, id string, removed bool) (indexcore.ResourceContext, error) {
	f.exact++
	if !removed {
		return indexcore.ResourceContext{}, ErrInvalid
	}
	if f.unavailable {
		return indexcore.ResourceContext{}, ErrUnavailable
	}
	for _, r := range f.inventory {
		if r.ResourceID == id {
			return r, nil
		}
	}
	if r, ok := f.removed[id]; ok {
		return r, nil
	}
	return indexcore.ResourceContext{}, indexcore.ErrNotFound
}
func TestLibraryFileDeltaAvoidsWholeRootAndRequiresExplicitRemoval(t *testing.T) {
	e, f := setup(t, 300, 20)
	d := &deltaFixture{fixture: f, removed: map[string]indexcore.ResourceContext{}}
	e.source = d
	ctx := context.Background()
	if err := e.Sync(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	baseline := f.browses
	added := row(999, false)
	f.inventory = append(f.inventory, added)
	id := added.ResourceID
	f.events = []indexcore.JournalEvent{{EventSeq: 1, ResourceID: &id, EventType: indexcore.EventResourceAdded}}
	if err := e.Sync(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	p, _ := e.Page(query())
	if p.Counts.All != 301 || f.browses != baseline || d.exact != 1 {
		t.Fatal("add enumerated Root", p.Counts, f.browses, d.exact)
	}
	removed := added
	removed.Presence = indexcore.ResourceRemoved
	d.removed[id] = removed
	f.inventory = f.inventory[:len(f.inventory)-1]
	f.events = append(f.events, indexcore.JournalEvent{EventSeq: 2, ResourceID: &id, EventType: indexcore.EventResourceRemoved})
	if err := e.Sync(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	p, _ = e.Page(query())
	if p.Counts.All != 300 || f.browses != baseline || d.exact != 2 {
		t.Fatal("remove enumerated Root", p.Counts, f.browses, d.exact)
	}
	fileID := f.inventory[0].ResourceID
	f.events = append(f.events, indexcore.JournalEvent{EventSeq: 3, ResourceID: &fileID, EventType: indexcore.EventResourceUpdated})
	d.unavailable = true
	if e.Sync(ctx, "one") == nil {
		t.Fatal("failed exact read published")
	}
	p, _ = e.Page(query())
	if p.Counts.All != 300 || !p.Stale {
		t.Fatal(p)
	}
	d.unavailable = false
	dirID := f.inventory[300].ResourceID
	f.events = append(f.events, indexcore.JournalEvent{EventSeq: 4, ResourceID: &dirID, EventType: indexcore.EventResourceMoved})
	if err := e.Sync(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	if f.browses <= baseline {
		t.Fatal("directory change skipped safe reconstruction")
	}
	t.Logf("baseline Q6=%d; file add/remove each used one Q7 and zero Q6 pages", baseline)
}
func TestLibraryJournalRewindOrRewriteRebuildsPersistedIdentity(t *testing.T) {
	e, f := setup(t, 2, 0)
	id := f.inventory[0].ResourceID
	f.events = []indexcore.JournalEvent{{EventSeq: 1, ResourceID: &id}}
	if err := e.Sync(context.Background(), "one"); err != nil {
		t.Fatal(err)
	}
	f.events = nil
	f.inventory = []indexcore.ResourceContext{row(888, false)}
	if err := e.Sync(context.Background(), "one"); err != nil {
		t.Fatal(err)
	}
	p, _ := e.Page(query())
	if p.Counts.All != 1 || p.Items[0].Resource.ResourceID == id || f.saved.Sequence != 0 {
		t.Fatal("rewound source retained old publication", p)
	}
}

func TestLibraryEmptyNonterminalPageCannotPublishDeletion(t *testing.T) {
	e, f := setup(t, 3, 0)
	if err := e.Sync(context.Background(), "one"); err != nil {
		t.Fatal(err)
	}
	f.events = []indexcore.JournalEvent{{EventSeq: 1}}
	f.emptyNext = true
	if e.Sync(context.Background(), "one") == nil {
		t.Fatal("incomplete empty page accepted")
	}
	p, _ := e.Page(query())
	if p.Counts.All != 3 || !p.Stale {
		t.Fatal(p)
	}
}
