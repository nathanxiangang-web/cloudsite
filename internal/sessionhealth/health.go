// Package sessionhealth bounds read-only authentication probes for one immutable
// runtime session. It owns no secrets, database, provider actions or goroutines.
package sessionhealth

import (
	"context"
	"sync"
	"time"
)

type Status string

const (
	SessionAuthenticated Status = "AUTHENTICATED"
	SessionLoginRequired Status = "LOGIN_REQUIRED"
	SessionUnknown       Status = "UNKNOWN"
)

type Checker interface{ CheckSession(context.Context) Status }

const MaxAge = 15 * time.Second
const Timeout = 5 * time.Second

type View struct {
	Status     Status    `json:"status"`
	CheckedAt  time.Time `json:"checked_at"`
	ValidUntil time.Time `json:"valid_until"`
}

type Reader interface{ Read(context.Context) View }

type Health struct {
	mu       sync.Mutex
	checker  Checker
	view     View
	inflight chan struct{}
	now      func() time.Time
}

func New(checker Checker) *Health {
	return &Health{checker: checker, now: time.Now}
}

// Read coalesces all visitors for a session. Canceled waiters do not cancel
// another request's probe. The initiating request owns and joins its bounded
// probe; failures are cached too, so an outage cannot cause a request storm.
func (h *Health) Read(ctx context.Context) View {
	if h == nil || ctx.Err() != nil {
		return View{Status: SessionUnknown}
	}
	for {
		h.mu.Lock()
		if h.now().Before(h.view.ValidUntil) {
			v := h.view
			h.mu.Unlock()
			return v
		}
		if pending := h.inflight; pending != nil {
			h.mu.Unlock()
			select {
			case <-pending:
				continue
			case <-ctx.Done():
				return View{Status: SessionUnknown}
			}
		}
		h.inflight = make(chan struct{})
		h.mu.Unlock()
		probeCtx, cancel := context.WithTimeout(ctx, Timeout)
		status := SessionUnknown
		if h.checker != nil {
			status = h.checker.CheckSession(probeCtx)
		}
		if probeCtx.Err() != nil || (status != SessionAuthenticated && status != SessionLoginRequired) {
			status = SessionUnknown
		}
		cancel()
		h.mu.Lock()
		now := h.now().UTC()
		h.view = View{Status: status, CheckedAt: now, ValidUntil: now.Add(MaxAge)}
		v := h.view
		close(h.inflight)
		h.inflight = nil
		h.mu.Unlock()
		return v
	}
}
