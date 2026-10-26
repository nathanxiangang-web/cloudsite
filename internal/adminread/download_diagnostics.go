package adminread

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"sort"
	"strings"
	"time"
)

var ErrDownloadNotFound = errors.New("admin download not found")

func ValidDownloadStatus(s string) bool {
	switch s {
	case "", "ALL", "PENDING", "ACTIVE", "AWAITING_VISIBILITY", "AWAITING_CANONICAL", "READY", "FAILED", "CANCELED", "RECOVERY_REQUIRED":
		return true
	}
	return false
}

type DownloadFilter struct {
	Limit                 int
	Cursor, Owner, Status string
}

func (f DownloadFilter) Valid() bool {
	return f.Limit >= 1 && f.Limit <= 50 && len(f.Cursor) <= 1024 && (f.Owner == "" || ValidID(f.Owner)) && ValidDownloadStatus(f.Status)
}

type DownloadCopy struct {
	CopyID       string    `json:"copy_id"`
	BindingID    string    `json:"binding_id"`
	Availability string    `json:"availability"`
	UpdatedAt    time.Time `json:"updated_at"`
}
type DownloadDetail struct {
	Download
	ReadAt        time.Time     `json:"read_at"`
	Source        string        `json:"source"`
	MaxAttempts   int           `json:"max_attempts"`
	JobCreatedAt  *time.Time    `json:"job_created_at"`
	JobUpdatedAt  *time.Time    `json:"job_updated_at"`
	StartedAt     *time.Time    `json:"started_at"`
	FinishedAt    *time.Time    `json:"finished_at"`
	NextAttemptAt *time.Time    `json:"next_attempt_at"`
	ErrorRecorded bool          `json:"error_recorded"`
	ErrorCode     string        `json:"error_code"`
	Copy          *DownloadCopy `json:"copy"`
}
type DownloadDetailRecord struct {
	DownloadDetail
	LinkValid bool
}
type DownloadDiagnosticReader interface {
	FilterDownloads(context.Context, []string, int, string, string, string) ([]DownloadRecord, error)
	ReadDownloadDetail(context.Context, []string, string) (DownloadDetailRecord, error)
}
type DownloadDiagnosticReads interface {
	SearchDownloads(context.Context, auth.Principal, DownloadFilter) (DownloadPage, error)
	DownloadDetail(context.Context, auth.Principal, string) (DownloadDetail, error)
}

// Cursors bind accidental cross-filter/actor/config reuse, not authorization.
// Every request still reauthorizes and reapplies the configured Binding allowlist.
type downloadCursor struct{ After, Owner, Status, Actor, Bindings string }

func (s *Service) cursorScope(p auth.Principal, f DownloadFilter) downloadCursor {
	b := append([]string(nil), s.bindings...)
	sort.Strings(b)
	digest := sha256.Sum256([]byte(strings.Join(b, ",")))
	status := f.Status
	if status == "" {
		status = "ALL"
	}
	return downloadCursor{Owner: f.Owner, Status: status, Actor: string(p.UserID), Bindings: fmt.Sprintf("%x", digest)}
}
func encodeDownloadCursor(c downloadCursor) string {
	raw, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(raw)
}
func (s *Service) SearchDownloads(ctx context.Context, p auth.Principal, f DownloadFilter) (DownloadPage, error) {
	if err := authorize(p, 1, ""); err != nil {
		return DownloadPage{}, err
	}
	if !f.Valid() {
		return DownloadPage{}, ErrInvalid
	}
	scope := s.cursorScope(p, f)
	after := ""
	if f.Cursor != "" {
		raw, e := base64.RawURLEncoding.DecodeString(f.Cursor)
		var c downloadCursor
		if e != nil || json.Unmarshal(raw, &c) != nil || !ValidID(c.After) || encodeDownloadCursor(c) != f.Cursor {
			return DownloadPage{}, ErrInvalid
		}
		after = c.After
		c.After = ""
		if c != scope {
			return DownloadPage{}, ErrInvalid
		}
	}
	r, ok := s.reader.(DownloadDiagnosticReader)
	if !ok {
		return DownloadPage{}, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if ctx.Err() != nil {
		return DownloadPage{}, ctx.Err()
	}
	rows, e := r.FilterDownloads(ctx, append([]string(nil), s.bindings...), f.Limit+1, after, f.Owner, scope.Status)
	if e != nil {
		return DownloadPage{}, readError(e)
	}
	if ctx.Err() != nil {
		return DownloadPage{}, ctx.Err()
	}
	if len(rows) > f.Limit+1 {
		return DownloadPage{}, ErrUnavailable
	}
	page := DownloadPage{Items: []Download{}}
	previous := after
	for _, row := range rows {
		if !s.downloadInScope(row, f.Owner) || row.ManifestID <= previous || scope.Status != "ALL" && row.ManifestState != scope.Status {
			return DownloadPage{}, ErrUnavailable
		}
		previous = row.ManifestID
		page.Items = append(page.Items, row.Download)
	}
	if len(page.Items) > f.Limit {
		page.Items = page.Items[:f.Limit]
		scope.After = page.Items[f.Limit-1].ManifestID
		page.NextCursor = encodeDownloadCursor(scope)
	}
	return page, nil
}
func (s *Service) downloadInScope(row DownloadRecord, owner string) bool {
	allowed := false
	for _, id := range s.bindings {
		if row.BindingID == id {
			allowed = true
		}
	}
	return allowed && ValidID(row.ManifestID) && ValidID(row.OwnerUserID) && (owner == "" || owner == row.OwnerUserID) && validDownload(row)
}
func ValidDownloadErrorCode(code string) bool {
	switch code {
	case "NONE", "RECORDED_REDACTED", "POLL_TRANSPORT_TIMEOUT", "POLL_TRANSPORT_OTHER", "POLL_HTTP_STATUS", "POLL_DECODE", "POLL_PROVIDER_TEMPORARY", "POLL_PERMANENT_OR_UNKNOWN":
		return true
	}
	return false
}
func (s *Service) DownloadDetail(ctx context.Context, p auth.Principal, id string) (DownloadDetail, error) {
	if e := authorize(p, 1, ""); e != nil {
		return DownloadDetail{}, e
	}
	if !ValidID(id) {
		return DownloadDetail{}, ErrInvalid
	}
	r, ok := s.reader.(DownloadDiagnosticReader)
	if !ok {
		return DownloadDetail{}, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if ctx.Err() != nil {
		return DownloadDetail{}, ctx.Err()
	}
	record, e := r.ReadDownloadDetail(ctx, append([]string(nil), s.bindings...), id)
	if errors.Is(e, ErrDownloadNotFound) {
		return DownloadDetail{}, ErrDownloadNotFound
	}
	if e != nil {
		return DownloadDetail{}, readError(e)
	}
	if ctx.Err() != nil {
		return DownloadDetail{}, ctx.Err()
	}
	d := record.DownloadDetail
	if d.ManifestID != id || !s.downloadInScope(DownloadRecord{Download: d.Download, LinkValid: record.LinkValid, MaxAttempts: d.MaxAttempts}, "") || d.ReadAt.IsZero() || d.Source != "CLOUDSITE_POSTGRES_MANIFEST_JOB_COPY" || !ValidDownloadErrorCode(d.ErrorCode) || d.ErrorRecorded != (d.ErrorCode != "NONE") {
		return DownloadDetail{}, ErrUnavailable
	}
	if d.JobID == nil {
		if d.MaxAttempts != 0 || d.JobCreatedAt != nil || d.JobUpdatedAt != nil || d.StartedAt != nil || d.FinishedAt != nil || d.NextAttemptAt != nil || d.ErrorRecorded {
			return DownloadDetail{}, ErrUnavailable
		}
	} else {
		if d.JobCreatedAt == nil || d.JobUpdatedAt == nil || !validTimes(*d.JobCreatedAt, *d.JobUpdatedAt) {
			return DownloadDetail{}, ErrUnavailable
		}
		for _, stamp := range []*time.Time{d.StartedAt, d.FinishedAt} {
			if stamp != nil && (stamp.IsZero() || stamp.Before(*d.JobCreatedAt) || stamp.After(*d.JobUpdatedAt)) {
				return DownloadDetail{}, ErrUnavailable
			}
		}
		if d.NextAttemptAt != nil && d.NextAttemptAt.IsZero() || *d.JobState == "RETRY_WAIT" && d.NextAttemptAt == nil {
			return DownloadDetail{}, ErrUnavailable
		}
	}
	if d.ResultCopyID == nil {
		if d.Copy != nil {
			return DownloadDetail{}, ErrUnavailable
		}
	} else {
		c := d.Copy
		if c == nil || c.CopyID != *d.ResultCopyID || c.BindingID != d.BindingID || c.Availability != "PRESENT" && c.Availability != "REMOVED" || c.UpdatedAt.IsZero() {
			return DownloadDetail{}, ErrUnavailable
		}
	}
	return d, nil
}
