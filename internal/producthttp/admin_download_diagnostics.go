package producthttp

import (
	"github.com/nathanxiangang-web/panta/internal/adminread"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"net/http"
)

func registerAdminDownloadDetail(h *Handler, reads adminread.Reads) {
	h.mux.Handle("GET /api/v1/admin/downloads/{manifestID}", h.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		if _, e := strictQuery(r); e != nil || !adminread.ValidID(r.PathValue("manifestID")) {
			adminReadFailure(w, adminread.ErrInvalid)
			return
		}
		if !adminReadsConfigured(reads) {
			adminReadFailure(w, adminread.ErrUnavailable)
			return
		}
		port, ok := reads.(adminread.DownloadDiagnosticReads)
		if !ok {
			adminReadFailure(w, adminread.ErrUnavailable)
			return
		}
		d, e := port.DownloadDetail(r.Context(), p, r.PathValue("manifestID"))
		if e != nil {
			adminReadFailure(w, e)
			return
		}
		respond(w, 200, d)
	})))
}
