package producthttp

import (
	"github.com/nathanxiangang-web/panta/internal/adminread"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"net/http"
)

func registerAdminUserDetail(h *Handler, reads adminread.Reads) {
	h.mux.Handle("GET /api/v1/admin/users/{user_id}", h.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, _ := Principal(r.Context())
		if p.Role != auth.RoleAdmin {
			failure(w, 403, "ADMIN_REQUIRED")
			return
		}
		if r.Method != http.MethodGet {
			failure(w, 405, "METHOD_NOT_ALLOWED")
			return
		}
		if !adminAcceptsJSON(r.Header.Values("Accept")) {
			failure(w, 406, "NOT_ACCEPTABLE")
			return
		}
		if _, e := strictQuery(r); e != nil {
			adminReadFailure(w, adminread.ErrInvalid)
			return
		}
		if !adminread.ValidID(r.PathValue("user_id")) {
			adminReadFailure(w, adminread.ErrInvalid)
			return
		}
		directory, ok := reads.(adminread.DirectoryReads)
		if !ok || !adminReadsConfigured(reads) {
			adminReadFailure(w, adminread.ErrUnavailable)
			return
		}
		u, e := directory.UserDetail(r.Context(), p, r.PathValue("user_id"))
		if e != nil {
			adminReadFailure(w, e)
			return
		}
		respond(w, 200, u)
	})))
}
