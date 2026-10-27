// Package adminaudit reads the current administrator's product action ledger.
// It deliberately has no execution, provider, replay or raw-receipt port.
package adminaudit

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/nathanxiangang-web/panta/internal/adminops"
	"github.com/nathanxiangang-web/panta/internal/adminread"
	"github.com/nathanxiangang-web/panta/internal/auth"
)

var (
	ErrInvalid     = errors.New("invalid audit query")
	ErrForbidden   = errors.New("audit requires current ADMIN scope")
	ErrNotFound    = errors.New("audit action not visible")
	ErrUnavailable = errors.New("audit unavailable")
)

const Source = "CLOUDSITE_POSTGRES_ADMIN_ACTION_LEDGER"
const Scope = "CURRENT_ADMIN_ACTIONS_IN_CONFIGURED_BINDINGS"

type Filter struct {
	Limit                           int
	Cursor, Action, Status, ActorID string
	From, Before                    time.Time
}

func ValidAction(s string) bool {
	switch s {
	case "", "ALL", adminops.CreateUser, adminops.SetEnabled, adminops.RenameUser, adminops.ResetPassword, adminops.DeleteUser, adminops.RestoreUser, adminops.RevokeShare, adminops.ProbeAccount, adminops.CancelPending:
		return true
	}
	return false
}
func ValidStatus(s string) bool {
	return s == "" || s == "ALL" || s == "UNKNOWN" || s == "SUCCEEDED" || s == "REJECTED"
}
func validTime(t time.Time) bool {
	return !t.IsZero() && t.Year() >= 1 && t.Year() <= 9999 && t.Equal(t.Truncate(time.Microsecond))
}
func (f Filter) Valid() bool {
	return f.Limit >= 1 && f.Limit <= 50 && len(f.Cursor) <= 2048 && ValidAction(f.Action) && ValidStatus(f.Status) && (f.ActorID == "" || adminread.ValidID(f.ActorID)) && (f.From.IsZero() || validTime(f.From)) && (f.Before.IsZero() || validTime(f.Before)) && (f.From.IsZero() || f.Before.IsZero() || f.From.Before(f.Before))
}

type Item struct {
	ActorID       string     `json:"actor_id"`
	RequestID     string     `json:"request_id"`
	Action        string     `json:"action"`
	Status        string     `json:"status"`
	Code          string     `json:"code"`
	CreatedAt     time.Time  `json:"created_at"`
	RecordedAt    *time.Time `json:"recorded_at"`
	TargetStatus  string     `json:"target_status"`
	ManifestState string     `json:"manifest_state,omitempty"`
}
type Record struct {
	Item
	Valid bool
}
type Snapshot struct {
	Items  []Record
	ReadAt time.Time
}
type Position struct {
	CreatedAt time.Time
	RequestID string
}
type Reader interface {
	ReadAudit(context.Context, adminops.Actor, []string, Filter, Position, string) (Snapshot, error)
}
type Page struct {
	Items      []Item    `json:"items"`
	NextCursor string    `json:"next_cursor"`
	Source     string    `json:"source"`
	Scope      string    `json:"scope"`
	ReadAt     time.Time `json:"read_at"`
}
type Detail struct {
	Item
	Source string    `json:"source"`
	Scope  string    `json:"scope"`
	ReadAt time.Time `json:"read_at"`
}
type Reads interface {
	Search(context.Context, adminops.Actor, Filter) (Page, error)
	Detail(context.Context, adminops.Actor, string, string) (Detail, error)
}
type Service struct {
	reader   Reader
	bindings []string
}

func New(reader Reader, bindings []string) (*Service, error) {
	if reader == nil || reflect.ValueOf(reader).Kind() == reflect.Pointer && reflect.ValueOf(reader).IsNil() || len(bindings) < 1 || len(bindings) > 64 {
		return nil, ErrUnavailable
	}
	b := append([]string(nil), bindings...)
	sort.Strings(b)
	for i, id := range b {
		if !adminread.ValidID(id) || i > 0 && b[i-1] == id {
			return nil, ErrInvalid
		}
	}
	return &Service{reader, b}, nil
}
func authorize(a adminops.Actor) error {
	if !adminread.ValidID(string(a.Principal.UserID)) || a.Principal.Login == "" || a.SessionDigest == [32]byte{} {
		return auth.ErrUnauthorized
	}
	if a.Principal.Role != auth.RoleAdmin {
		return ErrForbidden
	}
	return nil
}
func safe(e error) error {
	if e == nil || errors.Is(e, auth.ErrUnauthorized) || errors.Is(e, ErrForbidden) || errors.Is(e, ErrNotFound) || errors.Is(e, context.Canceled) || errors.Is(e, context.DeadlineExceeded) {
		return e
	}
	return ErrUnavailable
}
func ValidCode(action, status, code string) bool {
	if status == "UNKNOWN" {
		return code == "RESERVED_NO_CONFIRMED_RESULT"
	}
	successes := map[string]string{adminops.CreateUser: "USER_CREATED", adminops.SetEnabled: "USER_STATE_CHANGED", adminops.RenameUser: "USER_RENAMED", adminops.ResetPassword: "USER_PASSWORD_RESET", adminops.DeleteUser: "USER_DELETED", adminops.RestoreUser: "USER_RESTORED", adminops.RevokeShare: "SHARE_REVOKED", adminops.ProbeAccount: "PROBE_RECORDED", adminops.CancelPending: "PENDING_CANCELED"}
	if status == "SUCCEEDED" {
		return successes[action] == code && code != ""
	}
	if status != "REJECTED" {
		return false
	}
	switch action {
	case adminops.CreateUser:
		return code == "CONFLICT"
	case adminops.ProbeAccount:
		return code == "SCOPE_REJECTED" || code == "NOT_CONNECTED" || code == "RATE_LIMITED"
	case adminops.RevokeShare:
		return code == "TARGET_FORBIDDEN" || code == "SHARE_NOT_FOUND" || code == "SHARE_ALREADY_REVOKED"
	case adminops.CancelPending:
		return code == "MANIFEST_NOT_FOUND" || code == "OWNER_UNAVAILABLE" || code == "PENDING_CHANGED" || code == "PENDING_NOT_ISOLATED" || code == "MANIFEST_UNAVAILABLE"
	default:
		return code == "TARGET_FORBIDDEN" || code == "VERSION_CONFLICT" || code == "USER_STATE_CONFLICT" || action == adminops.RestoreUser && code == "PASSWORD_UNCHANGED" || action == adminops.DeleteUser && (code == "ACTIVE_DOWNLOADS" || code == "DOWNLOAD_STATE_CONFLICT") || action == adminops.RenameUser && code == "LOGIN_CONFLICT"
	}
}
func valid(r Record, a adminops.Actor) bool {
	if !r.Valid || r.ActorID != string(a.Principal.UserID) || !adminread.ValidID(r.RequestID) || r.Action == "" || r.Action == "ALL" || !ValidAction(r.Action) || !validTime(r.CreatedAt) || !ValidCode(r.Action, r.Status, r.Code) {
		return false
	}
	if r.TargetStatus != "NOT_RECORDED" && r.TargetStatus != "REDACTED" && r.TargetStatus != "DELETED_OR_MISSING" {
		return false
	}
	if r.ManifestState != "" && (!adminread.ValidDownloadStatus(r.ManifestState) || r.ManifestState == "ALL" || r.Action != adminops.CancelPending) {
		return false
	}
	if r.Status == "UNKNOWN" {
		return r.RecordedAt == nil && r.ManifestState == ""
	}
	return r.RecordedAt != nil && validTime(*r.RecordedAt) && !r.RecordedAt.Before(r.CreatedAt)
}

type cursor struct {
	At                                                     time.Time
	Request, Actor, Action, Status, From, Before, Bindings string
}

func (s *Service) scope(a adminops.Actor, f Filter) cursor {
	action, status := f.Action, f.Status
	if action == "" {
		action = "ALL"
	}
	if status == "" {
		status = "ALL"
	}
	stamp := func(t time.Time) string {
		if t.IsZero() {
			return ""
		}
		return t.UTC().Format(time.RFC3339Nano)
	}
	return cursor{Actor: string(a.Principal.UserID), Action: action, Status: status, From: stamp(f.From), Before: stamp(f.Before), Bindings: fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(s.bindings, ","))))}
}
func encode(c cursor) string {
	raw, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(raw)
}
func (s *Service) Search(ctx context.Context, a adminops.Actor, f Filter) (Page, error) {
	if e := authorize(a); e != nil {
		return Page{}, e
	}
	if !f.Valid() {
		return Page{}, ErrInvalid
	}
	if f.ActorID != "" && f.ActorID != string(a.Principal.UserID) {
		return Page{}, ErrForbidden
	}
	scope := s.scope(a, f)
	position := Position{}
	if f.Cursor != "" {
		raw, e := base64.RawURLEncoding.DecodeString(f.Cursor)
		var c cursor
		if e != nil || json.Unmarshal(raw, &c) != nil || !validTime(c.At) || !adminread.ValidID(c.Request) || encode(c) != f.Cursor {
			return Page{}, ErrInvalid
		}
		position = Position{c.At, c.Request}
		c.At = time.Time{}
		c.Request = ""
		if c != scope {
			return Page{}, ErrInvalid
		}
	}
	ctx, stop := context.WithTimeout(ctx, 5*time.Second)
	defer stop()
	f.Action = scope.Action
	f.Status = scope.Status
	f.ActorID = scope.Actor
	if e := ctx.Err(); e != nil {
		return Page{}, e
	}
	snapshot, e := s.reader.ReadAudit(ctx, a, append([]string(nil), s.bindings...), f, position, "")
	if e != nil {
		return Page{}, safe(e)
	}
	if e := ctx.Err(); e != nil {
		return Page{}, e
	}
	if !validTime(snapshot.ReadAt) || len(snapshot.Items) > f.Limit+1 {
		return Page{}, ErrUnavailable
	}
	p := Page{Items: []Item{}, Source: Source, Scope: Scope, ReadAt: snapshot.ReadAt}
	previous := position
	for _, r := range snapshot.Items {
		if !valid(r, a) || f.Action != "ALL" && r.Action != f.Action || f.Status != "ALL" && r.Status != f.Status || !f.From.IsZero() && r.CreatedAt.Before(f.From) || !f.Before.IsZero() && !r.CreatedAt.Before(f.Before) || !previous.CreatedAt.IsZero() && (!r.CreatedAt.Before(previous.CreatedAt) && (!r.CreatedAt.Equal(previous.CreatedAt) || r.RequestID >= previous.RequestID)) {
			return Page{}, ErrUnavailable
		}
		previous = Position{r.CreatedAt, r.RequestID}
		p.Items = append(p.Items, r.Item)
	}
	if len(p.Items) > f.Limit {
		p.Items = p.Items[:f.Limit]
		last := p.Items[len(p.Items)-1]
		scope.At = last.CreatedAt
		scope.Request = last.RequestID
		p.NextCursor = encode(scope)
	}
	return p, nil
}
func (s *Service) Detail(ctx context.Context, a adminops.Actor, actor, request string) (Detail, error) {
	if e := authorize(a); e != nil {
		return Detail{}, e
	}
	if !adminread.ValidID(actor) || !adminread.ValidID(request) {
		return Detail{}, ErrInvalid
	}
	if actor != string(a.Principal.UserID) {
		return Detail{}, ErrNotFound
	}
	ctx, stop := context.WithTimeout(ctx, 5*time.Second)
	defer stop()
	if e := ctx.Err(); e != nil {
		return Detail{}, e
	}
	snapshot, e := s.reader.ReadAudit(ctx, a, append([]string(nil), s.bindings...), Filter{Limit: 1, Action: "ALL", Status: "ALL", ActorID: actor}, Position{}, request)
	if e != nil {
		return Detail{}, safe(e)
	}
	if e := ctx.Err(); e != nil {
		return Detail{}, e
	}
	if len(snapshot.Items) == 0 {
		return Detail{}, ErrNotFound
	}
	if len(snapshot.Items) != 1 || !validTime(snapshot.ReadAt) || !valid(snapshot.Items[0], a) || snapshot.Items[0].RequestID != request {
		return Detail{}, ErrUnavailable
	}
	return Detail{snapshot.Items[0].Item, Source, Scope, snapshot.ReadAt}, nil
}
