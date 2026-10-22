package adminread

import (
	"context"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"strings"
	"time"
)

type UserFilter struct {
	Limit                  int
	Cursor, Search, Status string
}

func (f UserFilter) Valid() bool {
	if !ValidPage(f.Limit, f.Cursor) || len(f.Search) > 64 || strings.TrimSpace(f.Search) != f.Search {
		return false
	}
	for _, r := range f.Search {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-') {
			return false
		}
	}
	return f.Status == "" || f.Status == "ALL" || f.Status == "ENABLED" || f.Status == "DISABLED" || f.Status == "DELETED"
}

type UserDetail struct {
	User
	DownloadCount   int64 `json:"download_count"`
	ActiveDownloads int64 `json:"active_downloads"`
}
type UserDirectory struct {
	Items      []User    `json:"items"`
	NextCursor string    `json:"next_cursor"`
	Total      int64     `json:"total"`
	ReadAt     time.Time `json:"read_at"`
	Scope      string    `json:"scope"`
}

// Separate additive port keeps legacy owner/user-list contracts replaceable.
type DirectoryReader interface {
	SearchUsers(context.Context, UserFilter) (UserDirectory, error)
	UserDetail(context.Context, string) (UserDetail, error)
}
type DirectoryReads interface {
	SearchUsers(context.Context, auth.Principal, UserFilter) (UserDirectory, error)
	UserDetail(context.Context, auth.Principal, string) (UserDetail, error)
}

func (s *Service) SearchUsers(ctx context.Context, p auth.Principal, f UserFilter) (UserDirectory, error) {
	if e := authorize(p, f.Limit, f.Cursor); e != nil {
		return UserDirectory{}, e
	}
	if !f.Valid() {
		return UserDirectory{}, ErrInvalid
	}
	reader, ok := s.reader.(DirectoryReader)
	if !ok {
		return UserDirectory{}, ErrUnavailable
	}
	ctx, stop := context.WithTimeout(ctx, 10*time.Second)
	defer stop()
	page, e := reader.SearchUsers(ctx, f)
	if e != nil {
		return UserDirectory{}, readError(e)
	}
	if ctx.Err() != nil {
		return UserDirectory{}, ctx.Err()
	}
	if page.Total < 0 || page.ReadAt.IsZero() || page.Scope != "FILTERED_PRODUCT_USERS" || len(page.Items) > f.Limit || int64(len(page.Items)) > page.Total {
		return UserDirectory{}, ErrUnavailable
	}
	previous := f.Cursor
	for _, u := range page.Items {
		if !validDirectoryUser(u) || u.UserID <= previous {
			return UserDirectory{}, ErrUnavailable
		}
		previous = u.UserID
	}
	if page.NextCursor != "" && (len(page.Items) != f.Limit || page.NextCursor != previous) {
		return UserDirectory{}, ErrUnavailable
	}
	return page, nil
}
func (s *Service) UserDetail(ctx context.Context, p auth.Principal, id string) (UserDetail, error) {
	if e := authorize(p, 1, ""); e != nil {
		return UserDetail{}, e
	}
	if !ValidID(id) {
		return UserDetail{}, ErrInvalid
	}
	reader, ok := s.reader.(DirectoryReader)
	if !ok {
		return UserDetail{}, ErrUnavailable
	}
	ctx, stop := context.WithTimeout(ctx, 10*time.Second)
	defer stop()
	u, e := reader.UserDetail(ctx, id)
	if e != nil {
		if e == ErrNotFound {
			return UserDetail{}, e
		}
		return UserDetail{}, readError(e)
	}
	if ctx.Err() != nil {
		return UserDetail{}, ctx.Err()
	}
	if u.UserID != id || !validDirectoryUser(u.User) || u.DownloadCount < 0 || u.ActiveDownloads < 0 || u.ActiveDownloads > u.DownloadCount {
		return UserDetail{}, ErrUnavailable
	}
	return u, nil
}
func validDirectoryUser(u User) bool {
	login, e := auth.NormalizeLogin(u.Login)
	return ValidID(u.UserID) && e == nil && login == u.Login && u.Role.Valid() && u.Version >= 0 && validTimes(u.CreatedAt, u.UpdatedAt) && (u.DeletedAt == nil && u.DeletedBy == nil || u.DeletedAt != nil && u.DeletedBy != nil && !u.Enabled && ValidID(*u.DeletedBy) && !u.DeletedAt.Before(u.CreatedAt) && !u.UpdatedAt.Before(*u.DeletedAt))
}
