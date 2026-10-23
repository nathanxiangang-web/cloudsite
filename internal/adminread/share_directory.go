package adminread

import (
	"context"
	"strings"
	"time"

	"github.com/nathanxiangang-web/panta/internal/auth"
)

// ShareFilter is the whole-site share directory query. Status is an exact,
// bounded enum; an empty status is treated as ALL by the adapter.
type ShareFilter struct {
	Limit                 int
	Cursor, Owner, Status string
}

func (f ShareFilter) Valid() bool {
	if !ValidPage(f.Limit, f.Cursor) || f.Owner != "" && !ValidID(f.Owner) {
		return false
	}
	return f.Status == "" || f.Status == "ALL" || f.Status == "ACTIVE" || f.Status == "REVOKED" || f.Status == "EXPIRED"
}

// Share is a provider-neutral projection. It never carries code_hash or
// token_hash: those are authentication secrets, not administration metadata.
type Share struct {
	ShareID       string    `json:"share_id"`
	OwnerID       string    `json:"owner_id"`
	BindingID     string    `json:"binding_id"`
	CanonicalPath string    `json:"canonical_path"`
	FileName      string    `json:"file_name"`
	SizeBytes     int64     `json:"size_bytes"`
	CreatedAt     time.Time `json:"created_at"`
	ExpiresAt     time.Time `json:"expires_at"`
	Active        bool      `json:"active"`
}
type ShareDirectory struct {
	Items      []Share   `json:"items"`
	NextCursor string    `json:"next_cursor"`
	Total      int64     `json:"total"`
	ReadAt     time.Time `json:"read_at"`
	Scope      string    `json:"scope"`
}

// Additive read port; the legacy owner list/detail contracts stay replaceable.
type ShareReader interface {
	SearchShares(context.Context, ShareFilter) (ShareDirectory, error)
}
type ShareReads interface {
	SearchShares(context.Context, auth.Principal, ShareFilter) (ShareDirectory, error)
}

func validDirectoryShare(s Share) bool {
	return ValidID(s.ShareID) && ValidID(s.OwnerID) && ValidID(s.BindingID) && s.SizeBytes >= 0 &&
		!s.CreatedAt.IsZero() && !s.ExpiresAt.IsZero() && !s.ExpiresAt.Before(s.CreatedAt) &&
		strings.TrimSpace(s.FileName) != "" && strings.TrimSpace(s.CanonicalPath) != "" &&
		!strings.ContainsAny(s.FileName+s.CanonicalPath, "\x00\r\n")
}

func (s *Service) SearchShares(ctx context.Context, p auth.Principal, f ShareFilter) (ShareDirectory, error) {
	if e := authorize(p, f.Limit, f.Cursor); e != nil {
		return ShareDirectory{}, e
	}
	if !f.Valid() {
		return ShareDirectory{}, ErrInvalid
	}
	reader, ok := s.reader.(ShareReader)
	if !ok {
		return ShareDirectory{}, ErrUnavailable
	}
	ctx, stop := context.WithTimeout(ctx, 10*time.Second)
	defer stop()
	page, e := reader.SearchShares(ctx, f)
	if e != nil {
		return ShareDirectory{}, readError(e)
	}
	if ctx.Err() != nil {
		return ShareDirectory{}, ctx.Err()
	}
	if page.Total < 0 || page.ReadAt.IsZero() || page.Scope != "FILTERED_PRODUCT_SHARES" || len(page.Items) > f.Limit || int64(len(page.Items)) > page.Total {
		return ShareDirectory{}, ErrUnavailable
	}
	previous := f.Cursor
	for _, item := range page.Items {
		if !validDirectoryShare(item) || item.ShareID <= previous {
			return ShareDirectory{}, ErrUnavailable
		}
		previous = item.ShareID
	}
	if page.NextCursor != "" && (len(page.Items) != f.Limit || page.NextCursor != previous) {
		return ShareDirectory{}, ErrUnavailable
	}
	return page, nil
}
