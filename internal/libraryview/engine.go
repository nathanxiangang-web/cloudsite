// Package libraryview owns a disposable, persistent presentation read model.
// It reads IndexCore through public ports and never writes physical truth.
package libraryview

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/nathanxiangang-web/panta/internal/integrations/indexcore"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

var ErrUnavailable = errors.New("library snapshot unavailable")
var ErrInvalid = errors.New("invalid library request")
var ErrStale = errors.New("library page version expired")
var errGap = errors.New("library journal gap")

const MaxResources = 100000
const MaxJournalEvents = 100000
const MaxInventoryPages = MaxResources/50 + 1

type Source interface {
	indexcore.ReadPort
	indexcore.JournalReadPort
	indexcore.RootStatusReadPort
}
type Bindings interface {
	GetBinding(context.Context, storage.BindingID) (storage.Binding, error)
}
type Snapshot struct {
	BindingID        storage.BindingID
	RootID           string
	Sequence         int64
	Checkpoint       string
	UpdatedAt        time.Time
	Resources        []indexcore.ResourceContext
	Revision         string
	PreviousSequence int64
	PreviousRevision string
}
type Store interface {
	LoadLibrarySnapshot(context.Context, storage.BindingID, string) (Snapshot, bool, error)
	SaveLibrarySnapshot(context.Context, Snapshot) error
}
type Engine struct {
	source    Source
	bindings  Bindings
	store     Store
	ids       []storage.BindingID
	syncMu    sync.Mutex // one owned background lane, never one builder per HTTP client
	mu        sync.RWMutex
	snapshots map[storage.BindingID]*Snapshot
	previews  map[storage.BindingID]*Snapshot
	blocked   map[storage.BindingID]bool
	failed    map[storage.BindingID]bool
}

func New(source Source, bindings Bindings, store Store, ids []storage.BindingID) (*Engine, error) {
	if source == nil || bindings == nil || store == nil || len(ids) == 0 || len(ids) > 64 {
		return nil, ErrInvalid
	}
	seen := map[storage.BindingID]bool{}
	for _, id := range ids {
		if id == "" || seen[id] {
			return nil, ErrInvalid
		}
		seen[id] = true
	}
	return &Engine{source: source, bindings: bindings, store: store, ids: append([]storage.BindingID(nil), ids...), snapshots: map[storage.BindingID]*Snapshot{}, previews: map[storage.BindingID]*Snapshot{}, blocked: map[storage.BindingID]bool{}, failed: map[storage.BindingID]bool{}}, nil
}

// Restore performs database-only startup work. It issues no IndexCore/provider IO.
func (e *Engine) Restore(ctx context.Context) error {
	total := 0
	for _, id := range e.ids {
		b, err := e.bindings.GetBinding(ctx, id)
		if err != nil {
			return ErrUnavailable
		}
		s, ok, err := e.store.LoadLibrarySnapshot(ctx, id, b.IndexCoreRootID)
		if errors.Is(err, ErrUnavailable) {
			continue
		}
		if err != nil {
			return ErrUnavailable
		}
		if ok && validateSnapshot(s) == nil {
			total += len(s.Resources)
			if total > MaxResources {
				return ErrUnavailable
			}
			e.mu.Lock()
			e.snapshots[id] = &s
			e.mu.Unlock()
		}
	}
	return nil
}

// Run owns and joins all source activity through its caller's context.
func (e *Engine) Run(ctx context.Context) {
	tick := time.NewTicker(15 * time.Second)
	defer tick.Stop()
	for {
		for _, id := range e.ids {
			if ctx.Err() != nil {
				return
			}
			one, cancel := context.WithTimeout(ctx, 2*time.Minute)
			_ = e.Sync(one, id)
			cancel()
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
func (e *Engine) Sync(ctx context.Context, id storage.BindingID) (resultErr error) {
	e.syncMu.Lock()
	defer e.syncMu.Unlock()
	defer func() { e.mu.Lock(); e.failed[id] = resultErr != nil; e.mu.Unlock() }()
	if err := ctx.Err(); err != nil {
		return err
	}
	b, err := e.bindings.GetBinding(ctx, id)
	if err != nil {
		return ErrUnavailable
	}
	status, err := e.source.RootStatus(ctx, indexcore.RootStatusRequest{RootID: b.IndexCoreRootID, IncludeDeprecatedRoot: true, IncludeDeletedRoot: true})
	if err != nil {
		return ErrUnavailable
	}
	if b.Status != storage.BindingStatusActive || status.RootID != b.IndexCoreRootID || status.LifecycleState != indexcore.RootActive {
		e.mu.Lock()
		e.blocked[id] = true
		e.mu.Unlock()
		return ErrUnavailable
	}
	e.mu.RLock()
	previous := e.snapshots[id]
	e.mu.RUnlock()
	var cursor int64
	if previous != nil && previous.RootID == b.IndexCoreRootID {
		cursor = previous.Sequence
	}
	reset := false
	if cursor > 0 {
		anchor, anchorErr := e.anchor(ctx, b.IndexCoreRootID, cursor)
		if anchorErr != nil {
			return anchorErr
		}
		if anchor == "" || anchor != previous.Checkpoint {
			cursor = 0
			reset = true
		}
	}
	// Q8 is an invalidation/checkpoint stream. Empty polls never enumerate Q6.
	end, changed, err := e.drain(ctx, b.IndexCoreRootID, cursor, previous == nil || reset)
	if errors.Is(err, errGap) {
		end, _, err = e.drain(ctx, b.IndexCoreRootID, 0, true)
		changed = true
		reset = true
	}
	if err != nil {
		return err
	}
	if previous != nil && previous.RootID == b.IndexCoreRootID && !changed && !reset {
		e.mu.Lock()
		e.blocked[id] = false
		e.mu.Unlock()
		return nil
	}
	// Root lifecycle can change after the first status read. Never publish an
	// empty inventory merely because an inactive Root became invisible.
	status, err = e.source.RootStatus(ctx, indexcore.RootStatusRequest{RootID: b.IndexCoreRootID, IncludeDeprecatedRoot: true, IncludeDeletedRoot: true})
	if err != nil {
		return ErrUnavailable
	}
	if status.RootID != b.IndexCoreRootID || status.LifecycleState != indexcore.RootActive {
		e.mu.Lock()
		e.blocked[id] = true
		e.mu.Unlock()
		return ErrUnavailable
	}
	// A directory move can change descendant presentation. Rebuild the changed
	// Root, rather than guessing event payload semantics or dropping descendants.
	s := Snapshot{BindingID: id, RootID: b.IndexCoreRootID, Sequence: end, UpdatedAt: time.Now().UTC(), Resources: []indexcore.ResourceContext{}}
	if end > 0 {
		s.Checkpoint, err = e.anchor(ctx, s.RootID, end)
		if err != nil || s.Checkpoint == "" {
			return ErrUnavailable
		}
	}
	var applied bool
	if !reset && previous != nil && previous.RootID == s.RootID {
		s.Resources, applied, err = e.delta(ctx, previous, s.RootID, cursor, end)
		if err != nil {
			return err
		}
	}
	if !applied {
		s.Resources = []indexcore.ResourceContext{}
		seen := map[string]bool{}
		cursors := map[string]bool{}
		next := ""
		pages := 0
		for {
			if pages >= MaxInventoryPages {
				return ErrUnavailable
			}
			pages++
			page, readErr := e.source.Browse(ctx, indexcore.BrowseRequest{WholeRoot: true, RootID: s.RootID, Limit: 50, Cursor: next})
			if readErr != nil || len(page.Items) > 50 || len(page.Items) == 0 && page.NextCursor != "" {
				return ErrUnavailable
			}
			for _, r := range page.Items {
				if !validResource(r, s.RootID) || seen[r.ResourceID] {
					return ErrUnavailable
				}
				seen[r.ResourceID] = true
				s.Resources = append(s.Resources, normalize(r))
				if len(s.Resources) > MaxResources {
					return ErrUnavailable
				}
			}
			if next == "" && previous == nil {
				preview := s
				preview.Resources = append([]indexcore.ResourceContext(nil), s.Resources...)
				e.mu.Lock()
				e.previews[id] = &preview
				e.mu.Unlock()
			}
			if page.NextCursor == "" {
				break
			}
			if cursors[page.NextCursor] || len(page.NextCursor) > 2048 {
				return ErrUnavailable
			}
			cursors[page.NextCursor] = true
			next = page.NextCursor
		}
	}
	// Q6 pages are not a frozen snapshot. A canonical change during traversal
	// invalidates the candidate; retain the complete old publication and retry.
	_, changed, err = e.drain(ctx, s.RootID, end, false)
	if err != nil || changed {
		return ErrUnavailable
	}
	if end > 0 {
		anchor, err := e.anchor(ctx, s.RootID, end)
		if err != nil || anchor != s.Checkpoint {
			return ErrUnavailable
		}
	}
	status, err = e.source.RootStatus(ctx, indexcore.RootStatusRequest{RootID: s.RootID, IncludeDeprecatedRoot: true, IncludeDeletedRoot: true})
	if err != nil {
		return ErrUnavailable
	}
	if status.RootID != s.RootID || status.LifecycleState != indexcore.RootActive {
		e.mu.Lock()
		e.blocked[id] = true
		e.mu.Unlock()
		return ErrUnavailable
	}
	sort.Slice(s.Resources, func(i, j int) bool { return s.Resources[i].ResourceID < s.Resources[j].ResourceID })
	s.Revision = contentRevision(s.Resources)
	if previous != nil && previous.RootID == s.RootID && previous.Revision == s.Revision {
		s.UpdatedAt = previous.UpdatedAt
	}
	s.PreviousSequence = -1
	if previous != nil {
		s.PreviousSequence = previous.Sequence
		s.PreviousRevision = previous.Revision
	}
	e.mu.RLock()
	total := len(s.Resources)
	for key, old := range e.snapshots {
		if key != id {
			total += len(old.Resources)
		}
	}
	e.mu.RUnlock()
	if total > MaxResources {
		return ErrUnavailable
	}
	if err = e.store.SaveLibrarySnapshot(ctx, s); err != nil {
		return ErrUnavailable
	}
	e.mu.Lock()
	e.snapshots[id] = &s
	delete(e.previews, id)
	e.blocked[id] = false
	e.mu.Unlock()
	return nil
}
func (e *Engine) anchor(ctx context.Context, root string, seq int64) (string, error) {
	events, err := e.source.ReadJournal(ctx, indexcore.JournalRequest{RootID: root, AfterSeq: seq - 1, Limit: 1})
	if err != nil {
		return "", ErrUnavailable
	}
	if len(events) != 1 || events[0].EventSeq != seq {
		return "", nil
	}
	raw, _ := json.Marshal(events[0])
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
func (e *Engine) drain(ctx context.Context, root string, cursor int64, baseline bool) (int64, bool, error) {
	initial := cursor
	read := 0
	for {
		events, err := e.source.ReadJournal(ctx, indexcore.JournalRequest{RootID: root, AfterSeq: cursor, Limit: 100})
		if err != nil || len(events) > 100 {
			return cursor, false, ErrUnavailable
		}
		for _, event := range events {
			// IndexCore allocates a contiguous per-Root committed journal sequence.
			if event.EventSeq <= cursor {
				return cursor, false, ErrUnavailable
			}
			if !baseline && event.EventSeq != cursor+1 {
				return cursor, false, errGap
			}
			cursor = event.EventSeq
			read++
			if read > MaxJournalEvents {
				return cursor, false, ErrUnavailable
			}
		}
		if len(events) < 100 {
			return cursor, cursor != initial, nil
		}
	}
}
func validResource(r indexcore.ResourceContext, root string) bool {
	if r.RootID != root || r.ResourceID == "" || len(r.ResourceID) > 2048 || !utf8.ValidString(r.ResourceID) || strings.ContainsRune(r.ResourceID, 0) || r.Presence != indexcore.ResourcePresent || r.SizeBytes != nil && *r.SizeBytes < 0 {
		return false
	}
	if r.Name != nil && (!utf8.ValidString(*r.Name) || len(*r.Name) > 2048 || strings.ContainsRune(*r.Name, 0)) {
		return false
	}
	if r.CanonicalPath != nil {
		p, err := storage.NormalizeMountPath(*r.CanonicalPath)
		if err != nil || p != *r.CanonicalPath || !utf8.ValidString(p) || strings.ContainsRune(p, 0) {
			return false
		}
	}
	return true
}

// Confirmation timestamps/generations are not presentation invalidators.
func normalize(r indexcore.ResourceContext) indexcore.ResourceContext {
	r.ModifiedAt = nil
	r.IntroducedAtGeneration = 0
	r.LastConfirmedGeneration = 0
	r.ParentResourceID = nil
	if r.Name != nil {
		v := *r.Name
		r.Name = &v
	}
	if r.CanonicalPath != nil {
		v := *r.CanonicalPath
		r.CanonicalPath = &v
	}
	if r.Directory != nil {
		v := *r.Directory
		r.Directory = &v
	}
	if r.SizeBytes != nil {
		v := *r.SizeBytes
		r.SizeBytes = &v
	}
	return r
}
func contentRevision(items []indexcore.ResourceContext) string {
	files := make([]indexcore.ResourceContext, 0, len(items))
	for _, r := range items {
		if r.Directory != nil && !*r.Directory {
			files = append(files, normalize(r))
		}
	}
	raw, _ := json.Marshal(files)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func validateSnapshot(s Snapshot) error {
	if s.BindingID == "" || s.RootID == "" || s.Sequence < 0 || s.Sequence > 0 && len(s.Checkpoint) != 64 || s.UpdatedAt.IsZero() || len(s.Resources) > MaxResources {
		return ErrUnavailable
	}
	seen := map[string]bool{}
	for _, r := range s.Resources {
		if !validResource(r, s.RootID) || seen[r.ResourceID] {
			return ErrUnavailable
		}
		seen[r.ResourceID] = true
	}
	if contentRevision(s.Resources) != s.Revision {
		return ErrUnavailable
	}
	return nil
}

// ValidateSnapshot is shared with persistence adapters. A corrupt derived cache
// can be rebuilt; it must never be promoted to a successful publication.
func ValidateSnapshot(s Snapshot) error { return validateSnapshot(s) }
