package producthttp

import (
	"net/http"

	"github.com/nathanxiangang-web/panta/internal/adminread"
	"github.com/nathanxiangang-web/panta/internal/auth"
)

func registerAdminOverview(h *Handler, reads adminread.Reads) {
	h.mux.Handle("GET /api/v1/admin/overview", h.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		q, err := strictQuery(r, "user_id")
		if err != nil || q.Has("user_id") && !adminread.ValidID(q.Get("user_id")) {
			adminReadFailure(w, adminread.ErrInvalid)
			return
		}
		if !adminReadsConfigured(reads) {
			adminReadFailure(w, adminread.ErrUnavailable)
			return
		}
		overview, ok := reads.(adminread.OverviewReads)
		if !ok {
			adminReadFailure(w, adminread.ErrUnavailable)
			return
		}
		result, err := overview.Overview(r.Context(), p, q.Get("user_id"))
		if err != nil {
			adminReadFailure(w, err)
			return
		}
		respond(w, 200, result)
	})))
}
