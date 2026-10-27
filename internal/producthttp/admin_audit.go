package producthttp

import (
	"errors"
	"net/http"
	"reflect"
	"time"

	"github.com/nathanxiangang-web/panta/internal/adminaudit"
	"github.com/nathanxiangang-web/panta/internal/adminops"
	"github.com/nathanxiangang-web/panta/internal/adminread"
	"github.com/nathanxiangang-web/panta/internal/auth"
)

func WithAdminAuditReads(reads adminaudit.Reads) WorkspaceOption {
	return func(o *workspaceOptions) { o.audit = reads }
}
func auditReady(reads adminaudit.Reads) bool {
	return reads != nil && !(reflect.ValueOf(reads).Kind() == reflect.Pointer && reflect.ValueOf(reads).IsNil())
}
func registerAdminAudit(h *Handler, reads adminaudit.Reads) {
	for _, route := range []string{"GET /api/v1/admin/audit/actions", "GET /api/v1/admin/audit/actions/{actor_id}/{request_id}"} {
		h.mux.Handle(route, h.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, _ := Principal(r.Context())
			if p.Role != auth.RoleAdmin {
				failure(w, 403, "ADMIN_REQUIRED")
				return
			}
			if r.Method != "GET" {
				w.Header().Set("Allow", "GET")
				failure(w, 405, "METHOD_NOT_ALLOWED")
				return
			}
			if !adminAcceptsJSON(r.Header.Values("Accept")) {
				failure(w, 406, "NOT_ACCEPTABLE")
				return
			}
			exact := r.PathValue("request_id") != ""
			keys := []string{"limit", "cursor", "action", "status", "actor_id", "from", "before"}
			if exact {
				if !adminread.ValidID(r.PathValue("actor_id")) || !adminread.ValidID(r.PathValue("request_id")) {
					auditFailure(w, adminaudit.ErrInvalid)
					return
				}
				if r.PathValue("actor_id") != string(p.UserID) {
					auditFailure(w, adminaudit.ErrNotFound)
					return
				}
				keys = nil
			}
			q, e := strictQuery(r, keys...)
			if e != nil {
				auditFailure(w, adminaudit.ErrInvalid)
				return
			}
			if !auditReady(reads) {
				auditFailure(w, adminaudit.ErrUnavailable)
				return
			}
			actor := adminops.Actor{Principal: p, SessionDigest: auth.TokenDigest(h.cookie(r))}
			if exact {
				d, e := reads.Detail(r.Context(), actor, r.PathValue("actor_id"), r.PathValue("request_id"))
				if e != nil {
					auditFailure(w, e)
					return
				}
				respond(w, 200, d)
				return
			}
			limit, e := readLimit(q)
			f := adminaudit.Filter{Limit: limit, Cursor: q.Get("cursor"), Action: q.Get("action"), Status: q.Get("status"), ActorID: q.Get("actor_id")}
			for _, key := range keys {
				if q.Has(key) && q.Get(key) == "" {
					e = adminaudit.ErrInvalid
				}
			}
			if e != nil {
				auditFailure(w, adminaudit.ErrInvalid)
				return
			}
			if q.Has("from") {
				f.From, e = parseAuditTime(q.Get("from"))
				if e != nil {
					auditFailure(w, e)
					return
				}
			}
			if q.Has("before") {
				f.Before, e = parseAuditTime(q.Get("before"))
				if e != nil {
					auditFailure(w, e)
					return
				}
			}
			if e != nil || !f.Valid() {
				auditFailure(w, adminaudit.ErrInvalid)
				return
			}
			if f.ActorID != "" && f.ActorID != string(p.UserID) {
				auditFailure(w, adminaudit.ErrForbidden)
				return
			}
			page, e := reads.Search(r.Context(), actor, f)
			if e != nil {
				auditFailure(w, e)
				return
			}
			respond(w, 200, page)
		})))
	}
}
func parseAuditTime(raw string) (time.Time, error) {
	v, e := time.Parse(time.RFC3339Nano, raw)
	if e != nil || v.IsZero() || v.Nanosecond()%1000 != 0 {
		return time.Time{}, adminaudit.ErrInvalid
	}
	return v.UTC(), nil
}
func auditFailure(w http.ResponseWriter, e error) {
	switch {
	case errors.Is(e, auth.ErrUnauthorized):
		failure(w, 401, "AUTH_REQUIRED")
	case errors.Is(e, adminaudit.ErrForbidden):
		failure(w, 403, "ADMIN_REQUIRED")
	case errors.Is(e, adminaudit.ErrInvalid):
		failure(w, 400, "INVALID_REQUEST")
	case errors.Is(e, adminaudit.ErrNotFound):
		failure(w, 404, "AUDIT_NOT_FOUND")
	default:
		failure(w, 503, "AUDIT_UNAVAILABLE")
	}
}
