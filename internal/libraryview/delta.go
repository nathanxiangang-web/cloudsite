package libraryview

import (
	"context"
	"errors"

	"github.com/nathanxiangang-web/panta/internal/integrations/indexcore"
)

// delta uses only exact Q7 identity. Directory mutations, uncertain metadata or
// an excessive batch fall back to complete Q6 reconstruction; no payload-derived
// paths, path-only identity matching or guessed descendant deletion is used.
func (e *Engine) delta(ctx context.Context, old *Snapshot, root string, from, to int64) ([]indexcore.ResourceContext, bool, error) {
	reader, ok := e.source.(indexcore.ResourceReadPort)
	if !ok || old == nil || old.RootID != root {
		return nil, false, nil
	}
	ids := map[string]bool{}
	seq := from
	for seq < to {
		events, err := e.source.ReadJournal(ctx, indexcore.JournalRequest{RootID: root, AfterSeq: seq, Limit: 100})
		if err != nil {
			return nil, false, ErrUnavailable
		}
		if len(events) == 0 {
			return nil, false, ErrUnavailable
		}
		previous := seq
		for _, event := range events {
			if event.EventSeq > to {
				break
			}
			if event.EventSeq != seq+1 {
				return nil, false, ErrUnavailable
			}
			seq = event.EventSeq
			if event.ResourceID == nil || *event.ResourceID == "" {
				return nil, false, nil
			}
			ids[*event.ResourceID] = true
			if len(ids) > 100 {
				return nil, false, nil
			}
		}
		if seq == previous {
			return nil, false, ErrUnavailable
		}
	}
	rows := make(map[string]indexcore.ResourceContext, len(old.Resources))
	for _, r := range old.Resources {
		rows[r.ResourceID] = r
	}
	for id := range ids {
		before, known := rows[id]
		if known && (before.Directory == nil || *before.Directory) {
			return nil, false, nil
		}
		r, err := reader.Resource(ctx, id, true)
		if errors.Is(err, indexcore.ErrNotFound) {
			return nil, false, nil
		}
		if err != nil {
			return nil, false, ErrUnavailable
		}
		if r.RootID != root || r.ResourceID != id {
			return nil, false, ErrUnavailable
		}
		if r.Directory == nil || *r.Directory {
			return nil, false, nil
		}
		switch r.Presence {
		case indexcore.ResourceRemoved:
			delete(rows, id) // explicit committed canonical removal, never HTTP404/partial evidence
		case indexcore.ResourcePresent:
			if !validResource(r, root) {
				return nil, false, ErrUnavailable
			}
			rows[id] = normalize(r)
		default:
			return nil, false, ErrUnavailable
		}
	}
	out := make([]indexcore.ResourceContext, 0, len(rows))
	for _, r := range rows {
		out = append(out, r)
	}
	if len(out) > MaxResources {
		return nil, false, ErrUnavailable
	}
	return out, true, nil
}
