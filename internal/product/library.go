package product

import (
	"context"
	"errors"
	"time"

	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/integrations/indexcore"
	"github.com/nathanxiangang-web/panta/internal/libraryview"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

type LibraryRequest struct {
	DetailedCategories                bool
	Surface, Category, Search, Cursor string
	Page                              int
}

var ErrLibraryScopeChanged = errors.New("library scope changed")

type LibraryPage struct {
	Unknown    int                 `json:"unknown"`
	Items      []Resource          `json:"items"`
	Counts     *libraryview.Counts `json:"counts"`
	Ready      bool                `json:"ready"`
	Stale      bool                `json:"stale"`
	Revision   string              `json:"revision"`
	UpdatedAt  *time.Time          `json:"updated_at"`
	NextCursor string              `json:"next_cursor"`
	Offset     int                 `json:"offset"`
	Filtered   int                 `json:"filtered"`
	Limit      int                 `json:"limit"`
}
type LibraryReads interface {
	Library(context.Context, auth.Principal, LibraryRequest) (LibraryPage, error)
}
type LibraryService struct {
	reads     *ReadService
	engine    *libraryview.Engine
	main      []storage.BindingID
	cloud     storage.BindingID
	cloudPath string
}

func NewLibraryService(reads *ReadService, engine *libraryview.Engine, main []storage.BindingID, cloud storage.BindingID, cloudPath string) (*LibraryService, error) {
	if reads == nil || engine == nil {
		return nil, ErrInvalid
	}
	if len(main) == 0 {
		main = reads.allowed
	}
	seen := map[storage.BindingID]bool{}
	for _, id := range main {
		if !reads.bindings[id] || seen[id] {
			return nil, ErrInvalid
		}
		seen[id] = true
	}
	if cloud != "" && (!reads.bindings[cloud] || cloudPath == "" || cloudPath == "/") {
		return nil, ErrInvalid
	}
	return &LibraryService{reads, engine, append([]storage.BindingID(nil), main...), cloud, cloudPath}, nil
}
func (s *LibraryService) Library(ctx context.Context, p auth.Principal, q LibraryRequest) (LibraryPage, error) {
	ctx, cancel := context.WithTimeout(ctx, ReadTimeout)
	defer cancel()
	if err := principalValid(p); err != nil {
		return LibraryPage{}, err
	}
	if q.Surface != "library" && q.Surface != "downloads" {
		return LibraryPage{}, ErrInvalid
	}
	ids := s.main
	if q.Surface == "downloads" {
		if s.cloud == "" {
			return LibraryPage{}, ErrNotFound
		}
		ids = []storage.BindingID{s.cloud}
	}
	scopes := []libraryview.Scope{}
	bindings := map[storage.BindingID]storage.Binding{}
	for _, id := range ids {
		b, err := s.reads.binding(ctx, p, id)
		if errors.Is(err, ErrNotFound) {
			return LibraryPage{}, ErrLibraryScopeChanged
		}
		if err != nil {
			return LibraryPage{}, err
		}
		if b.Status != storage.BindingStatusActive {
			return LibraryPage{}, ErrLibraryScopeChanged
		}
		bindings[id] = b
		scopes = append(scopes, libraryview.Scope{BindingID: id, RootID: b.IndexCoreRootID})
	}
	page, err := s.engine.Page(libraryview.Request{Scopes: scopes, Owner: string(p.UserID), CloudBinding: s.cloud, CloudPath: s.cloudPath, Downloads: q.Surface == "downloads", Category: q.Category, DetailedCategories: q.DetailedCategories, Search: q.Search, Cursor: q.Cursor, Limit: 50, Page: q.Page})
	if err != nil {
		return LibraryPage{}, err
	}
	out := LibraryPage{Items: []Resource{}, Unknown: page.Unknown, Counts: page.Counts, Ready: page.Ready, Stale: page.Stale, Revision: page.Revision, NextCursor: page.NextCursor, Offset: page.Offset, Filtered: page.Filtered, Limit: 50}
	if !page.UpdatedAt.IsZero() {
		at := page.UpdatedAt
		out.UpdatedAt = &at
	}
	// Group only the requested page; the PostgreSQL adapter performs one exact
	// identity batch per Binding, including absence checks for physical-only files.
	groups := map[storage.BindingID][]indexcore.ResourceContext{}
	for _, item := range page.Items {
		groups[item.BindingID] = append(groups[item.BindingID], item.Resource)
	}
	projected := map[storage.BindingID]map[string]Resource{}
	for id, items := range groups {
		rows, err := s.reads.resources(ctx, p, bindings[id], items)
		if err != nil {
			return LibraryPage{}, err
		}
		projected[id] = map[string]Resource{}
		for _, row := range rows {
			projected[id][row.ResourceID] = row
		}
	}
	for _, item := range page.Items {
		out.Items = append(out.Items, projected[item.BindingID][item.Resource.ResourceID])
	}
	return out, nil
}
