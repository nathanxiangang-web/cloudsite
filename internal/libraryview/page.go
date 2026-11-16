package libraryview

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nathanxiangang-web/panta/internal/integrations/indexcore"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

type Scope struct {
	BindingID storage.BindingID
	RootID    string
}
type Request struct {
	Scopes       []Scope
	Owner        string
	CloudBinding storage.BindingID
	CloudPath    string
	Downloads    bool
	Category     string
	Search       string
	Cursor       string
	Limit        int
	Page         int
}
type Item struct {
	BindingID storage.BindingID
	Resource  indexcore.ResourceContext
}
type Counts struct {
	All      int `json:"all"`
	Software int `json:"software"`
	Video    int `json:"video"`
	Document int `json:"document"`
	Other    int `json:"other"`
}
type Page struct {
	Items      []Item
	Counts     *Counts
	Ready      bool
	Stale      bool
	Unknown    int
	Revision   string
	UpdatedAt  time.Time
	NextCursor string
	Offset     int
	Filtered   int
}
type cursor struct {
	Revision string `json:"r"`
	Offset   int    `json:"o"`
	Filter   string `json:"f"`
}

func (e *Engine) Page(q Request) (Page, error) {
	if q.Limit < 1 || q.Limit > 50 || q.Page < 0 || q.Page >= MaxResources/50 || q.Cursor != "" && q.Page != 0 || len(q.Scopes) == 0 || len(q.Scopes) > 64 || len(q.Search) > 1024 || !utf8.ValidString(q.Search) || len(q.Cursor) > 2048 {
		return Page{}, ErrInvalid
	}
	if q.Category != "all" && q.Category != "software" && q.Category != "video" && q.Category != "document" && q.Category != "other" {
		return Page{}, ErrInvalid
	}
	snapshots := make([]*Snapshot, 0, len(q.Scopes))
	ready := true
	seen := map[storage.BindingID]bool{}
	e.mu.RLock()
	for _, scope := range q.Scopes {
		if seen[scope.BindingID] || e.blocked[scope.BindingID] {
			e.mu.RUnlock()
			return Page{}, ErrUnavailable
		}
		seen[scope.BindingID] = true
		s := e.snapshots[scope.BindingID]
		if s == nil || s.RootID != scope.RootID {
			s = e.previews[scope.BindingID]
			ready = false
		}
		if s != nil && s.RootID == scope.RootID {
			snapshots = append(snapshots, s)
		}
	}
	e.mu.RUnlock()
	result := Page{Items: []Item{}, Ready: ready}
	count := Counts{}
	items := []Item{}
	var revision strings.Builder
	e.mu.RLock()
	for _, scope := range q.Scopes {
		result.Stale = result.Stale || e.failed[scope.BindingID]
	}
	e.mu.RUnlock()
	needle := strings.ToLower(strings.TrimSpace(q.Search))
	for _, s := range snapshots {
		fmt.Fprintf(&revision, "%s:%s:%s;", s.BindingID, s.RootID, s.Revision)
		if result.UpdatedAt.IsZero() || s.UpdatedAt.Before(result.UpdatedAt) {
			result.UpdatedAt = s.UpdatedAt
		}
		for _, r := range s.Resources {
			within := s.BindingID == q.CloudBinding && q.CloudPath != "" && r.CanonicalPath != nil && strings.HasPrefix(*r.CanonicalPath, q.CloudPath+"/")
			if q.Downloads != within {
				continue
			}
			if r.Directory == nil {
				result.Unknown++
				continue
			}
			if *r.Directory {
				continue
			}
			kind := Category(r)
			count.All++
			switch kind {
			case "software":
				count.Software++
			case "video":
				count.Video++
			case "document":
				count.Document++
			default:
				count.Other++
			}
			name := r.ResourceID
			if r.Name != nil && *r.Name != "" {
				name = *r.Name
			} else if r.CanonicalPath != nil && *r.CanonicalPath != "" {
				name = *r.CanonicalPath
			}
			if (q.Category == "all" || q.Category == kind) && (needle == "" || strings.Contains(strings.ToLower(name), needle)) {
				items = append(items, Item{s.BindingID, r})
			}
		}
	}
	fmt.Fprintf(&revision, "%s:%s:%t", q.CloudBinding, q.CloudPath, q.Downloads)
	sum := sha256.Sum256([]byte(revision.String()))
	result.Revision = hex.EncodeToString(sum[:])
	filterSum := sha256.Sum256([]byte(q.Owner + "\x00" + q.Category + "\x00" + q.Search))
	filter := hex.EncodeToString(filterSum[:])
	offset := 0
	if ready && q.Page > 0 && len(items) > 0 {
		offset = min(q.Page*q.Limit, ((len(items)-1)/q.Limit)*q.Limit)
	}
	if q.Cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(q.Cursor)
		if err != nil {
			return Page{}, ErrInvalid
		}
		var c cursor
		if json.Unmarshal(raw, &c) != nil || c.Offset < 0 || c.Offset%q.Limit != 0 || c.Filter != filter {
			return Page{}, ErrInvalid
		}
		if !ready || c.Revision != result.Revision || c.Offset >= len(items) {
			return Page{}, ErrStale
		}
		offset = c.Offset
	}
	end := min(offset+q.Limit, len(items))
	result.Items = append(result.Items, items[offset:end]...)
	result.Offset = offset
	result.Filtered = len(items)
	if ready {
		result.Counts = &count
		if end < len(items) {
			raw, _ := json.Marshal(cursor{result.Revision, end, filter})
			result.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
		}
	}
	return result, nil
}

// Category mirrors the existing consumer ResourceDisplay classification.
func Category(r indexcore.ResourceContext) string {
	name := ""
	if r.Name != nil {
		name = *r.Name
	}
	if name == "" && r.CanonicalPath != nil {
		name = *r.CanonicalPath
	}
	parts := strings.Split(name, "/")
	name = parts[len(parts)-1]
	dot := strings.LastIndex(name, ".")
	if dot <= 0 || dot == len(name)-1 {
		return "other"
	}
	// JavaScript lowercasing expands U+0130; it cannot form an ASCII extension.
	if strings.ContainsRune(name[dot+1:], 'İ') {
		return "other"
	}
	ext := strings.ToLower(name[dot+1:])
	for kind, extensions := range map[string]string{
		"software": " exe msi msix msixbundle appx appxbundle apk aab dmg pkg deb rpm appimage ",
		"video":    " mp4 mkv webm mov avi m4v mpeg mpg wmv ts m2ts flv 3gp ogv ",
		"document": " pdf doc docx odt rtf md markdown txt ppt pptx odp xls xlsx ods csv epub ",
	} {
		if strings.Contains(extensions, " "+ext+" ") {
			return kind
		}
	}
	return "other"
}
