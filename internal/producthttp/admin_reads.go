package producthttp

import (
	"errors"
	"mime"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/nathanxiangang-web/panta/internal/adminread"
	"github.com/nathanxiangang-web/panta/internal/auth"
)

func WithAdminReads(reads adminread.Reads) WorkspaceOption {
	return func(o *workspaceOptions) { o.adminReads = reads }
}
func adminReadsConfigured(reads adminread.Reads) bool {
	return reads != nil && !(reflect.ValueOf(reads).Kind() == reflect.Pointer && reflect.ValueOf(reads).IsNil())
}

// Authorization is fresh per request, not inferred from a previous descriptor.
// Absent composition fails closed instead of returning a fabricated empty list.
func registerAdminReads(h *Handler, reads adminread.Reads) {
	registerAdminUserDetail(h, reads)
	for _, kind := range []string{"users", "downloads"} {
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
			if kind == "users" {
				keys = append(keys, "q", "status")
			}
			if kind == "downloads" {
				keys = append(keys, "user_id")
			}
			query, err := strictQuery(r, keys...)
			if err != nil {
				adminReadFailure(w, adminread.ErrInvalid)
				return
			}
			limit, err := readLimit(query)
			if err != nil || !adminread.ValidPage(limit, query.Get("cursor")) || query.Has("cursor") && query.Get("cursor") == "" || query.Has("user_id") && !adminread.ValidID(query.Get("user_id")) {
				adminReadFailure(w, adminread.ErrInvalid)
				return
			}
			if !adminReadsConfigured(reads) {
				adminReadFailure(w, adminread.ErrUnavailable)
				return
			}
			if kind == "users" {
				if directory, ok := reads.(adminread.DirectoryReads); ok {
					page, e := directory.SearchUsers(r.Context(), p, adminread.UserFilter{Limit: limit, Cursor: query.Get("cursor"), Search: strings.ToLower(query.Get("q")), Status: query.Get("status")})
					if e != nil {
						adminReadFailure(w, e)
						return
					}
					respond(w, 200, page)
					return
				}
				if query.Has("q") || query.Has("status") {
					adminReadFailure(w, adminread.ErrUnavailable)
					return
				}
				page, err := reads.Users(r.Context(), p, limit, query.Get("cursor"))
				if err != nil {
					adminReadFailure(w, err)
					return
				}
				respond(w, 200, page)
			} else {
				page, err := reads.Downloads(r.Context(), p, limit, query.Get("cursor"), query.Get("user_id"))
				if err != nil {
					adminReadFailure(w, err)
					return
				}
				respond(w, 200, page)
			}
		})))
	}
}
func adminReadFailure(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrUnauthorized):
		failure(w, 401, "AUTH_REQUIRED")
	case errors.Is(err, adminread.ErrForbidden):
		failure(w, 403, "ADMIN_REQUIRED")
	case errors.Is(err, adminread.ErrInvalid):
		failure(w, 400, "INVALID_REQUEST")
	case errors.Is(err, adminread.ErrNotFound):
		failure(w, 404, "USER_NOT_FOUND")
	default:
		failure(w, 503, "READ_UNAVAILABLE")
	}
}

// Honor the most specific JSON media range, including an explicit q=0 denial.
func adminAcceptsJSON(headers []string) bool {
	if len(headers) == 0 {
		return true
	}
	best := -1
	quality := 0.0
	for _, value := range headers {
		for _, part := range strings.Split(value, ",") {
			media, params, err := mime.ParseMediaType(strings.TrimSpace(part))
			if err != nil {
				return false
			}
			q := 1.0
			if text, ok := params["q"]; ok {
				q, err = strconv.ParseFloat(text, 64)
				if err != nil || q < 0 || q > 1 {
					return false
				}
			}
			priority := -1
			switch media {
			case "application/json":
				priority = 2
			case "application/*":
				priority = 1
			case "*/*":
				priority = 0
			}
			if priority > best {
				best = priority
				quality = q
			} else if priority == best && q > quality {
				quality = q
			}
		}
	}
	return best >= 0 && quality > 0
}
