// Package adminread owns provider-neutral, read-only administration projections.
// It does not reuse owner reads or expose authentication/provider domain records.
package adminread

import (
	"context"
	"errors"
	"reflect"
	"regexp"
	"time"

	"github.com/nathanxiangang-web/panta/internal/auth"
)

var (
	ErrInvalid     = errors.New("invalid admin read request")
	ErrForbidden   = errors.New("admin required")
	ErrUnavailable = errors.New("admin read unavailable")
	ErrNotFound    = errors.New("admin user not found")
	idPattern      = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

func ValidID(id string) bool { return idPattern.MatchString(id) }
func ValidPage(limit int, cursor string) bool {
	return limit >= 1 && limit <= 50 && (cursor == "" || ValidID(cursor))
}
func nilPort(port any) bool {
	return port == nil || reflect.ValueOf(port).Kind() == reflect.Pointer && reflect.ValueOf(port).IsNil()
}

type User struct {
	Version   int64      `json:"version"`
	UserID    string     `json:"user_id"`
	Login     string     `json:"login"`
	Role      auth.Role  `json:"role"`
	Enabled   bool       `json:"enabled"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at"`
	DeletedBy *string    `json:"deleted_by"`
}
type Download struct {
	ManifestID    string    `json:"manifest_id"`
	OwnerUserID   string    `json:"owner_user_id"`
	BindingID     string    `json:"binding_id"`
	JobID         *string   `json:"job_id"`
	ManifestState string    `json:"manifest_state"`
	JobState      *string   `json:"job_state"`
	ResultCopyID  *string   `json:"result_copy_id"`
	ClaimAttempts int       `json:"claim_attempts"`
	FailureCount  int       `json:"failure_count"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// DownloadRecord contains only projection metadata. The adapter verifies the
// frozen acquisition linkage in SQL without returning payloads or operation keys.
type DownloadRecord struct {
	Download
	LinkValid   bool `json:"-"`
	MaxAttempts int  `json:"-"`
}
type UserPage struct {
	Items      []User `json:"items"`
	NextCursor string `json:"next_cursor"`
}
type DownloadPage struct {
	Items      []Download `json:"items"`
	NextCursor string     `json:"next_cursor"`
}
type Reader interface {
	ListUsers(context.Context, int, string) ([]User, error)
	ListDownloads(context.Context, []string, int, string, string) ([]DownloadRecord, error)
}
type Reads interface {
	Users(context.Context, auth.Principal, int, string) (UserPage, error)
	Downloads(context.Context, auth.Principal, int, string, string) (DownloadPage, error)
}
type Service struct {
	reader   Reader
	bindings []string
}

func NewService(reader Reader, bindings []string) (*Service, error) {
	if nilPort(reader) || len(bindings) == 0 || len(bindings) > 64 {
		return nil, ErrInvalid
	}
	seen := map[string]bool{}
	for _, id := range bindings {
		if !ValidID(id) || seen[id] {
			return nil, ErrInvalid
		}
		seen[id] = true
	}
	return &Service{reader: reader, bindings: append([]string(nil), bindings...)}, nil
}
func authorize(p auth.Principal, limit int, cursor string) error {
	if !p.Valid() {
		return auth.ErrUnauthorized
	}
	if p.Role != auth.RoleAdmin {
		return ErrForbidden
	}
	if !ValidPage(limit, cursor) {
		return ErrInvalid
	}
	return nil
}
func readError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return ErrUnavailable
}
func validTimes(created, updated time.Time) bool {
	return !created.IsZero() && !updated.IsZero() && !updated.Before(created)
}
func (s *Service) Users(ctx context.Context, p auth.Principal, limit int, cursor string) (UserPage, error) {
	if err := authorize(p, limit, cursor); err != nil {
		return UserPage{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return UserPage{}, err
	}
	rows, err := s.reader.ListUsers(ctx, limit+1, cursor)
	if err != nil {
		return UserPage{}, readError(err)
	}
	if err = ctx.Err(); err != nil {
		return UserPage{}, err
	}
	if len(rows) > limit+1 {
		return UserPage{}, ErrUnavailable
	}
	previous := cursor
	for _, row := range rows {
		if !validDirectoryUser(row) || row.UserID <= previous {
			return UserPage{}, ErrUnavailable
		}
		previous = row.UserID
	}
	page := UserPage{Items: append([]User{}, rows...)}
	if len(rows) > limit {
		page.Items = page.Items[:limit]
		page.NextCursor = page.Items[limit-1].UserID
	}
	return page, nil
}
func (s *Service) Downloads(ctx context.Context, p auth.Principal, limit int, cursor, user string) (DownloadPage, error) {
	if err := authorize(p, limit, cursor); err != nil {
		return DownloadPage{}, err
	}
	if user != "" && !ValidID(user) {
		return DownloadPage{}, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return DownloadPage{}, err
	}
	rows, err := s.reader.ListDownloads(ctx, append([]string(nil), s.bindings...), limit+1, cursor, user)
	if err != nil {
		return DownloadPage{}, readError(err)
	}
	if err = ctx.Err(); err != nil {
		return DownloadPage{}, err
	}
	if len(rows) > limit+1 {
		return DownloadPage{}, ErrUnavailable
	}
	page := DownloadPage{Items: []Download{}}
	previous := cursor
	for _, row := range rows {
		allowed := false
		for _, id := range s.bindings {
			if row.BindingID == id {
				allowed = true
			}
		}
		if !allowed || !ValidID(row.ManifestID) || row.ManifestID <= previous || !ValidID(row.OwnerUserID) || user != "" && row.OwnerUserID != user || !validDownload(row) {
			return DownloadPage{}, ErrUnavailable
		}
		previous = row.ManifestID
		page.Items = append(page.Items, row.Download)
	}
	if len(rows) > limit {
		page.Items = page.Items[:limit]
		page.NextCursor = page.Items[limit-1].ManifestID
	}
	return page, nil
}
func validDownload(r DownloadRecord) bool {
	if !validTimes(r.CreatedAt, r.UpdatedAt) || r.ClaimAttempts < 0 || r.FailureCount < 0 {
		return false
	}
	if r.ManifestState == "READY" {
		if r.ResultCopyID == nil || !ValidID(*r.ResultCopyID) {
			return false
		}
	} else if r.ResultCopyID != nil {
		return false
	}
	if r.ManifestState == "PENDING" {
		return r.JobID == nil && r.JobState == nil && r.ClaimAttempts == 0 && r.FailureCount == 0 && r.LinkValid
	}
	if r.JobID == nil || !ValidID(*r.JobID) || r.JobState == nil || !r.LinkValid || r.MaxAttempts < 1 || r.FailureCount > r.MaxAttempts {
		return false
	}
	switch r.ManifestState {
	case "ACTIVE":
		return *r.JobState == "QUEUED" || *r.JobState == "RUNNING" || *r.JobState == "RETRY_WAIT"
	case "AWAITING_VISIBILITY", "AWAITING_CANONICAL":
		return *r.JobState == "RUNNING" || *r.JobState == "RETRY_WAIT"
	case "READY":
		return *r.JobState == "SUCCEEDED"
	case "FAILED":
		return *r.JobState == "FAILED"
	case "CANCELED":
		return *r.JobState == "CANCELED"
	case "RECOVERY_REQUIRED":
		return *r.JobState == "RECOVERY_REQUIRED"
	default:
		return false
	}
}
