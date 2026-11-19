package producthttp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/nathanxiangang-web/panta/internal/libraryview"
	"github.com/nathanxiangang-web/panta/internal/product"
)

func WithLibraryReads(reads product.LibraryReads) WorkspaceOption {
	return func(o *workspaceOptions) { o.library = reads }
}
func registerLibrary(h *Handler, reads product.LibraryReads) {
	if reads == nil {
		return
	}
	h.mux.Handle("GET /api/v1/library", h.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q, err := strictQuery(r, "surface", "category", "q", "cursor", "page", "taxonomy")
		if err != nil {
			readFailure(w, err)
			return
		}
		if q.Has("taxonomy") && q.Get("taxonomy") != "2" {
			failure(w, 400, "INVALID_REQUEST")
			return
		}
		surface := q.Get("surface")
		if surface == "" {
			surface = "library"
		}
		category := q.Get("category")
		if category == "" {
			category = "all"
		}
		p, _ := Principal(r.Context())
		number := 0
		if q.Has("page") {
			number, err = strconv.Atoi(q.Get("page"))
			if err != nil || number < 0 || number >= libraryview.MaxResources/50 || q.Get("cursor") != "" {
				failure(w, 400, "INVALID_REQUEST")
				return
			}
		}
		page, err := reads.Library(r.Context(), p, product.LibraryRequest{Surface: surface, Category: category, DetailedCategories: q.Get("taxonomy") == "2", Search: q.Get("q"), Cursor: q.Get("cursor"), Page: number})
		if err != nil {
			switch {
			case errors.Is(err, product.ErrLibraryScopeChanged):
				failure(w, 403, "LIBRARY_SCOPE_CHANGED")
			case errors.Is(err, libraryview.ErrInvalid):
				failure(w, 400, "INVALID_REQUEST")
			case errors.Is(err, libraryview.ErrStale):
				failure(w, 409, "LIBRARY_VERSION_EXPIRED")
			case errors.Is(err, libraryview.ErrUnavailable):
				failure(w, 503, "READ_UNAVAILABLE")
			default:
				readFailure(w, err)
			}
			return
		}
		raw, err := json.Marshal(page)
		if err != nil {
			failure(w, 503, "READ_UNAVAILABLE")
			return
		}
		// Authorization and current Binding checks precede every conditional hit.
		sum := sha256.Sum256(append([]byte(p.UserID), raw...))
		tag := "\"" + hex.EncodeToString(sum[:]) + "\""
		w.Header().Set("Cache-Control", "private, no-cache")
		w.Header().Add("Vary", "Cookie")
		w.Header().Set("ETag", tag)
		if r.Header.Get("If-None-Match") == tag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(raw)
	})))
}
