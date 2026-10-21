// Package adminobserve owns bounded, provider-neutral configuration and Q9 evidence.
package adminobserve

import (
	"context"
	"errors"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/nathanxiangang-web/panta/internal/auth"
)

var (
	ErrInvalid      = errors.New("invalid observation request")
	ErrForbidden    = errors.New("admin required")
	ErrUnavailable  = errors.New("observation unavailable")
	ErrNotFound     = errors.New("configured binding not found")
	ErrNotConnected = errors.New("Q9 not connected")
	ErrRootNotFound = errors.New("Q9 root not found")
	ErrNotReady     = errors.New("Q9 root not ready")
	ErrBusy         = errors.New("Q9 read budget busy")
	uuid            = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

const EvidenceSource = "INDEXCORE_Q9_ROOT_STATUS"

func ValidID(id string) bool         { return uuid.MatchString(id) }
func ValidPage(n int, c string) bool { return n >= 1 && n <= 50 && (c == "" || ValidID(c)) }
func Missing(port any) bool {
	return port == nil || reflect.ValueOf(port).Kind() == reflect.Pointer && reflect.ValueOf(port).IsNil()
}

type Display struct{ BindingID, Label string }
type Storage struct {
	BindingID           string    `json:"binding_id"`
	DisplayLabel        string    `json:"display_label"`
	ConnectionID        string    `json:"connection_id"`
	ProviderType        string    `json:"provider_type"`
	BindingStatus       string    `json:"binding_config_status"`
	ConnectionStatus    string    `json:"connection_config_status"`
	BindingUpdatedAt    time.Time `json:"binding_updated_at"`
	ConnectionUpdatedAt time.Time `json:"connection_updated_at"`
	RootConfigured      bool      `json:"indexcore_root_configured"`
}
type Page struct {
	Items      []Storage `json:"items"`
	NextCursor string    `json:"next_cursor"`
}

// Mapping is server-only and cannot serialize its opaque root coordinate.
type Mapping struct {
	Storage
	RootID string `json:"-"`
}
type MappingReader interface {
	List(context.Context, []string, int, string) ([]Storage, error)
	Get(context.Context, []string, string) (Mapping, error)
}
type RootState struct {
	RootID, Lifecycle string
	Generation        int64
	LastAdmission     *int64
}
type RootReader interface {
	ReadRoot(context.Context, string) (RootState, error)
}
type Observation struct {
	BindingID      string    `json:"binding_id"`
	Lifecycle      string    `json:"root_lifecycle_state"`
	Generation     int64     `json:"current_generation"`
	LastAdmission  *int64    `json:"last_applied_admission_seq"`
	ObservedAt     time.Time `json:"observed_at"`
	EvidenceSource string    `json:"evidence_source"`
}
type Reads interface {
	Storage(context.Context, auth.Principal, int, string) (Page, error)
	Root(context.Context, auth.Principal, string) (Observation, error)
}
type Service struct {
	mappings MappingReader
	roots    RootReader
	ids      []string
	labels   map[string]string
	slots    chan struct{}
	mu       sync.Mutex
	window   time.Time
	used     int
	now      func() time.Time
}

func NewService(reader MappingReader, roots RootReader, scope []Display) (*Service, error) {
	if Missing(reader) || len(scope) < 1 || len(scope) > 64 {
		return nil, ErrInvalid
	}
	s := &Service{mappings: reader, roots: roots, labels: map[string]string{}, slots: make(chan struct{}, 2), now: time.Now}
	for _, d := range scope {
		if !ValidID(d.BindingID) || s.labels[d.BindingID] != "" || !utf8.ValidString(d.Label) || strings.TrimSpace(d.Label) == "" || utf8.RuneCountInString(d.Label) > 100 || strings.ContainsAny(d.Label, "\x00\r\n") {
			return nil, ErrInvalid
		}
		s.ids = append(s.ids, d.BindingID)
		s.labels[d.BindingID] = d.Label
	}
	return s, nil
}
func authorize(p auth.Principal) error {
	if !p.Valid() {
		return auth.ErrUnauthorized
	}
	if p.Role != auth.RoleAdmin {
		return ErrForbidden
	}
	return nil
}
func readError(err error) error {
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	return ErrUnavailable
}
func validStorage(row Storage) bool {
	validStatus := func(s string) bool { return s == "ACTIVE" || s == "DISABLED" }
	switch row.ProviderType {
	case "115", "openlist", "rclone", "local", "mock", "OTHER":
	default:
		return false
	}
	return ValidID(row.BindingID) && ValidID(row.ConnectionID) && validStatus(row.BindingStatus) && validStatus(row.ConnectionStatus) && !row.BindingUpdatedAt.IsZero() && !row.ConnectionUpdatedAt.IsZero()
}
func (s *Service) Storage(ctx context.Context, p auth.Principal, n int, cursor string) (Page, error) {
	if err := authorize(p); err != nil {
		return Page{}, err
	}
	if !ValidPage(n, cursor) {
		return Page{}, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return Page{}, err
	}
	rows, err := s.mappings.List(ctx, append([]string(nil), s.ids...), n+1, cursor)
	if err != nil {
		return Page{}, readError(err)
	}
	if err = ctx.Err(); err != nil {
		return Page{}, err
	}
	if len(rows) > n+1 {
		return Page{}, ErrUnavailable
	}
	page := Page{Items: []Storage{}}
	previous := cursor
	for _, row := range rows {
		label, ok := s.labels[row.BindingID]
		if !ok || row.BindingID <= previous || !validStorage(row) {
			return Page{}, ErrUnavailable
		}
		row.DisplayLabel = label
		page.Items = append(page.Items, row)
		previous = row.BindingID
	}
	if len(rows) > n {
		page.Items = page.Items[:n]
		page.NextCursor = page.Items[n-1].BindingID
	}
	return page, nil
}
func (s *Service) mapping(ctx context.Context, id string) (Mapping, error) {
	m, err := s.mappings.Get(ctx, append([]string(nil), s.ids...), id)
	if err != nil {
		return Mapping{}, readError(err)
	}
	if m.BindingID != id || !validStorage(m.Storage) || m.RootConfigured != (m.RootID != "") || m.RootConfigured && strings.TrimSpace(m.RootID) == "" || len(m.RootID) > 1024 || !utf8.ValidString(m.RootID) || strings.ContainsAny(m.RootID, "\x00\r\n") {
		return Mapping{}, ErrUnavailable
	}
	return m, nil
}
func (s *Service) Root(ctx context.Context, p auth.Principal, id string) (Observation, error) {
	if err := authorize(p); err != nil {
		return Observation{}, err
	}
	if !ValidID(id) {
		return Observation{}, ErrInvalid
	}
	if _, ok := s.labels[id]; !ok {
		return Observation{}, ErrNotFound
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return Observation{}, err
	}
	if Missing(s.roots) {
		return Observation{}, ErrNotConnected
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		return Observation{}, ErrBusy
	}
	m, err := s.mapping(ctx, id)
	if err != nil {
		return Observation{}, err
	}
	if !m.RootConfigured {
		return Observation{}, ErrNotConnected
	}
	s.mu.Lock()
	now := s.now()
	if s.window.IsZero() || now.Sub(s.window) >= time.Minute {
		s.window = now
		s.used = 0
	}
	busy := s.used >= 8
	if !busy {
		s.used++
	}
	s.mu.Unlock()
	if busy {
		return Observation{}, ErrBusy
	}
	state, err := s.roots.ReadRoot(ctx, m.RootID)
	if err != nil {
		switch {
		case errors.Is(err, ErrRootNotFound):
			return Observation{}, ErrRootNotFound
		case errors.Is(err, ErrNotReady):
			return Observation{}, ErrNotReady
		default:
			return Observation{}, readError(err)
		}
	}
	if err = ctx.Err(); err != nil {
		return Observation{}, err
	}
	switch state.Lifecycle {
	case "NEW", "ACTIVE", "DEPRECATED", "DELETED":
	default:
		return Observation{}, ErrUnavailable
	}
	if state.RootID != m.RootID || state.Generation < 0 || state.LastAdmission != nil && *state.LastAdmission < 0 {
		return Observation{}, ErrUnavailable
	}
	// A remap during the upstream read invalidates the result, not a new root probe.
	latest, err := s.mapping(ctx, id)
	if err != nil {
		return Observation{}, err
	}
	if latest != m {
		return Observation{}, ErrUnavailable
	}
	if err = ctx.Err(); err != nil {
		return Observation{}, err
	}
	return Observation{BindingID: id, Lifecycle: state.Lifecycle, Generation: state.Generation, LastAdmission: state.LastAdmission, ObservedAt: s.now().UTC(), EvidenceSource: EvidenceSource}, nil
}
