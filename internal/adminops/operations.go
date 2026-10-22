// Package adminops owns specific audited management commands, not a generic
// executor. Persistence, HTTP and provider SDKs are replaceable ports.
package adminops

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"reflect"
	"time"

	"github.com/nathanxiangang-web/panta/internal/adminread"
	"github.com/nathanxiangang-web/panta/internal/auth"
)

var (
	ErrInvalid     = errors.New("invalid admin command")
	ErrForbidden   = errors.New("admin action forbidden")
	ErrConflict    = errors.New("admin command conflict")
	ErrUnavailable = errors.New("admin operation unavailable")
	ErrNotFound    = errors.New("admin action not found")
)

const (
	CreateUser   = "CREATE_USER"
	SetEnabled   = "SET_USER_ENABLED"
	ProbeAccount = "PROBE_115"
)

// Actor carries the authenticated session digest, never the session bearer.
type Actor struct {
	Principal     auth.Principal
	SessionDigest [32]byte
}
type Command struct {
	RequestID       string `json:"request_id"`
	Action          string `json:"action"`
	TargetID        string `json:"target_id"`
	Login           string `json:"login"`
	Password        string `json:"-"`
	Enabled         bool   `json:"enabled"`
	ExpectedVersion int64  `json:"expected_version"`
}

func (c Command) Valid() bool {
	if !adminread.ValidID(c.RequestID) {
		return false
	}
	switch c.Action {
	case CreateUser:
		login, e := auth.NormalizeLogin(c.Login)
		return e == nil && login == c.Login && auth.ValidPassword(c.Password) && c.TargetID == "" && c.ExpectedVersion == 0 && !c.Enabled
	case SetEnabled:
		return adminread.ValidID(c.TargetID) && c.ExpectedVersion >= 0 && c.Login == "" && c.Password == ""
	case ProbeAccount:
		return adminread.ValidID(c.TargetID) && c.Login == "" && c.Password == "" && c.ExpectedVersion == 0 && !c.Enabled
	}
	return false
}
func (c Command) Digest() [32]byte {
	// Passwords (including password-derived fast hashes) never enter the ledger.
	// A repeated create key refers to the original creation, not a password reset.
	c.Password = ""
	c.RequestID = ""
	raw, _ := json.Marshal(c)
	return sha256.Sum256(raw)
}

type User struct {
	UserID  string `json:"user_id"`
	Login   string `json:"login"`
	Enabled bool   `json:"enabled"`
	Version int64  `json:"version"`
}
type Capacity struct {
	Status         string    `json:"status"`
	TotalBytes     string    `json:"total_bytes,omitempty"`
	UsedBytes      string    `json:"used_bytes,omitempty"`
	RemainingBytes string    `json:"remaining_bytes,omitempty"`
	Source         string    `json:"source"`
	ProbedAt       time.Time `json:"probed_at"`
}
type Result struct {
	ProbeTarget *ProbeTarget `json:"-"`
	RequestID   string       `json:"request_id"`
	Action      string       `json:"action"`
	TargetID    string       `json:"target_id,omitempty"`
	Status      string       `json:"status"` // UNKNOWN means reserved, never permission to retry.
	Code        string       `json:"code"`
	RecordedAt  time.Time    `json:"recorded_at"`
	User        *User        `json:"user,omitempty"`
	Capacity    *Capacity    `json:"capacity,omitempty"`
}

// ProbeTarget is server-only exact protected Connection+CredentialRef binding.
type ProbeTarget struct{ BindingID, ConnectionID, CredentialRef string }
type Store interface {
	Reserve(context.Context, Actor, Command, []string) (Result, bool, error)
	ApplyUser(context.Context, Actor, Command) (Result, error)
	ProbeTarget(context.Context, Actor, string) (ProbeTarget, error)
	CompleteProbe(context.Context, Actor, Command, ProbeTarget, Capacity) (Result, error)
	Status(context.Context, Actor, string) (Result, error)
}
type AccountProbe interface {
	Probe(context.Context, ProbeTarget) Capacity
}
type Operations interface {
	Execute(context.Context, Actor, Command) (Result, error)
	Status(context.Context, Actor, string) (Result, error)
}
type Service struct {
	store    Store
	probe    AccountProbe
	bindings []string
}

func NewService(store Store, probe AccountProbe, bindings []string) (*Service, error) {
	if store == nil || reflect.ValueOf(store).Kind() == reflect.Pointer && reflect.ValueOf(store).IsNil() || len(bindings) < 1 || len(bindings) > 64 {
		return nil, ErrInvalid
	}
	seen := map[string]bool{}
	for _, id := range bindings {
		if !adminread.ValidID(id) || seen[id] {
			return nil, ErrInvalid
		}
		seen[id] = true
	}
	return &Service{store, probe, append([]string(nil), bindings...)}, nil
}
func authorize(a Actor) error {
	if !a.Principal.Valid() {
		return auth.ErrUnauthorized
	}
	if a.Principal.Role != auth.RoleAdmin {
		return ErrForbidden
	}
	if a.SessionDigest == ([32]byte{}) {
		return auth.ErrUnauthorized
	}
	return nil
}
func (s *Service) Execute(ctx context.Context, a Actor, c Command) (Result, error) {
	if e := authorize(a); e != nil {
		return Result{}, e
	}
	if !c.Valid() {
		return Result{}, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	result, fresh, e := s.store.Reserve(ctx, a, c, s.bindings)
	if e != nil || !fresh {
		return result, safe(e)
	}
	if c.Action != ProbeAccount {
		result, e = s.store.ApplyUser(ctx, a, c)
		return result, safe(e)
	}
	capacity := Capacity{Status: "NOT_CONNECTED", Source: "115_GET_INFO"}
	target, e := s.store.ProbeTarget(ctx, a, c.TargetID)
	if e == nil && result.ProbeTarget != nil && target == *result.ProbeTarget && s.probe != nil {
		probeCtx, stop := context.WithTimeout(ctx, 8*time.Second)
		capacity = s.probe.Probe(probeCtx, target)
		stop()
	} else if e == nil && result.ProbeTarget != nil && target != *result.ProbeTarget {
		capacity.Status = "UNKNOWN"
	}
	if capacity.Source != "115_GET_INFO" || !validCapacity(capacity) {
		capacity = Capacity{Status: "UNKNOWN", Source: "115_GET_INFO", ProbedAt: time.Now().UTC()}
	}
	// A disconnected port is a real failure, never a healthy zero-capacity account.
	if capacity.ProbedAt.IsZero() {
		capacity.ProbedAt = time.Now().UTC()
	}
	finish, stop := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer stop()
	result, e = s.store.CompleteProbe(finish, a, c, target, capacity)
	return result, safe(e)
}
func (s *Service) Status(ctx context.Context, a Actor, id string) (Result, error) {
	if e := authorize(a); e != nil {
		return Result{}, e
	}
	if !adminread.ValidID(id) {
		return Result{}, ErrInvalid
	}
	ctx, stop := context.WithTimeout(ctx, 5*time.Second)
	defer stop()
	r, e := s.store.Status(ctx, a, id)
	return r, safe(e)
}
func safe(e error) error {
	if e == nil {
		return nil
	}
	for _, v := range []error{auth.ErrUnauthorized, ErrForbidden, ErrInvalid, ErrConflict, ErrNotFound, context.Canceled, context.DeadlineExceeded} {
		if errors.Is(e, v) {
			return v
		}
	}
	return ErrUnavailable
}
