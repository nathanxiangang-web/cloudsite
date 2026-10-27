package adminops

import (
	"context"
	"github.com/nathanxiangang-web/panta/internal/adminread"
	"time"
)

// PendingPrecondition comes from the exact detail read, not a list-page guess.
// UpdatedAt is a PostgreSQL timestamp CAS, not a caller-supplied wall clock.
type PendingPrecondition struct {
	OwnerUserID string    `json:"owner_user_id"`
	BindingID   string    `json:"binding_id"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (p *PendingPrecondition) valid() bool {
	return p != nil && adminread.ValidID(p.OwnerUserID) && adminread.ValidID(p.BindingID) && !p.UpdatedAt.IsZero() && p.UpdatedAt.Year() >= 1 && p.UpdatedAt.Year() <= 9999 && p.UpdatedAt.Equal(p.UpdatedAt.Truncate(time.Microsecond))
}

// ManifestOutcome contains only already-authorized state evidence. Hidden or
// unmapped targets never receive this projection; no source/payload is read.
type ManifestOutcome struct {
	State     string    `json:"state"`
	JobID     *string   `json:"job_id"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PendingCanceler is deliberately separate from user, share and provider ports.
// Only the service's fresh Reserve path may invoke it; UNKNOWN is status-only.
type PendingCanceler interface {
	ApplyPendingCancellation(context.Context, Actor, Command, []string) (Result, error)
}
