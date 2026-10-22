package producthttp

import (
	"errors"
	"net/http"

	"github.com/nathanxiangang-web/panta/internal/adminobserve"
	"github.com/nathanxiangang-web/panta/internal/auth"
)

func WithAdminObservation(reads adminobserve.Reads) WorkspaceOption {
	return func(o *workspaceOptions) { o.observation = reads }
}
func registerAdminObservation(h *Handler, reads adminobserve.Reads) {
	for _, kind := range []string{"storage", "indexcore"} {
		h.mux.Handle("GET /api/v1/admin/"+kind, h.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, _ := Principal(r.Context())
			if p.Role != auth.RoleAdmin {
				failure(w, 403, "ADMIN_REQUIRED")
				return
			}
			if r.Method != http.MethodGet {
				w.Header().Set("Allow", "GET")
				failure(w, 405, "METHOD_NOT_ALLOWED")
				return
			}
			if !adminAcceptsJSON(r.Header.Values("Accept")) {
				failure(w, 406, "NOT_ACCEPTABLE")
				return
			}
			keys := []string{"limit", "cursor"}
			if kind == "indexcore" {
				keys = []string{"binding_id"}
			}
			q, err := strictQuery(r, keys...)
			if err != nil {
				observeFailure(w, adminobserve.ErrInvalid)
				return
			}
			n := 20
			if kind == "storage" {
				n, err = readLimit(q)
				if err != nil || !adminobserve.ValidPage(n, q.Get("cursor")) || q.Has("cursor") && q.Get("cursor") == "" {
					observeFailure(w, adminobserve.ErrInvalid)
					return
				}
			} else if !adminobserve.ValidID(q.Get("binding_id")) {
				observeFailure(w, adminobserve.ErrInvalid)
				return
			}
			if adminobserve.Missing(reads) {
				failure(w, 503, "READ_UNAVAILABLE")
				return
			}
			if kind == "storage" {
				page, err := reads.Storage(r.Context(), p, n, q.Get("cursor"))
				if err != nil {
					observeFailure(w, err)
					return
				}
				respond(w, 200, page)
			} else {
				result, err := reads.Root(r.Context(), p, q.Get("binding_id"))
				if err != nil {
					observeFailure(w, err)
					return
				}
				respond(w, 200, result)
			}
		})))
	}
}
func observeFailure(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrUnauthorized):
		failure(w, 401, "AUTH_REQUIRED")
	case errors.Is(err, adminobserve.ErrForbidden):
		failure(w, 403, "ADMIN_REQUIRED")
	case errors.Is(err, adminobserve.ErrInvalid):
		failure(w, 400, "INVALID_REQUEST")
	case errors.Is(err, adminobserve.ErrNotFound):
		failure(w, 404, "NOT_FOUND")
	case errors.Is(err, adminobserve.ErrRootNotFound):
		failure(w, 404, "ROOT_NOT_FOUND")
	case errors.Is(err, adminobserve.ErrNotConnected):
		failure(w, 503, "NOT_CONNECTED")
	case errors.Is(err, adminobserve.ErrNotReady):
		failure(w, 503, "ROOT_NOT_READY")
	case errors.Is(err, adminobserve.ErrBusy):
		w.Header().Set("Retry-After", "60")
		failure(w, 429, "OBSERVATION_BUSY")
	default:
		failure(w, 503, "OBSERVATION_UNKNOWN")
	}
}
