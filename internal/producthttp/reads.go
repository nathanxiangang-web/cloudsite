package producthttp

import (
	"errors"
	"net/http"
	"net/url"
	"reflect"
	"strconv"

	"github.com/nathanxiangang-web/panta/internal/acquisition"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/product"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

// NewReadHandler composes the accepted auth boundary and bounded read routes.
// It opens no listener and does not claim jobs or resolve provider credentials.
func NewReadHandler(access auth.Access, origin string, reads product.Reads) (*Handler, error) {
	if reads == nil || (reflect.ValueOf(reads).Kind() == reflect.Pointer && reflect.ValueOf(reads).IsNil()) {
		return nil, product.ErrUnavailable
	}
	h, err := NewAuthHandler(access, origin)
	if err != nil {
		return nil, err
	}
	h.mux.Handle("GET /api/v1/tasks", h.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query, err := strictQuery(r, "limit", "cursor")
		if err != nil {
			readFailure(w, err)
			return
		}
		limit, err := readLimit(query)
		if err != nil {
			readFailure(w, err)
			return
		}
		p, _ := Principal(r.Context())
		page, err := reads.Tasks(r.Context(), p, limit, query.Get("cursor"))
		if err != nil {
			readFailure(w, err)
			return
		}
		respond(w, 200, page)
	})))
	h.mux.Handle("GET /api/v1/tasks/{manifest_id}", h.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := strictQuery(r); err != nil {
			readFailure(w, err)
			return
		}
		p, _ := Principal(r.Context())
		item, err := reads.Task(r.Context(), p, acquisition.ManifestID(r.PathValue("manifest_id")))
		if err != nil {
			readFailure(w, err)
			return
		}
		respond(w, 200, item)
	})))
	h.mux.Handle("GET /api/v1/resources", h.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query, err := strictQuery(r, "binding_id", "parent_resource_id", "cursor", "limit", "whole_root")
		if err != nil {
			readFailure(w, err)
			return
		}
		limit, err := readLimit(query)
		if err != nil {
			readFailure(w, err)
			return
		}
		if query.Has("whole_root") && query.Get("whole_root") != "true" {
			readFailure(w, product.ErrInvalid)
			return
		}
		request := product.BrowseRequest{WholeRoot: query.Get("whole_root") == "true", BindingID: storage.BindingID(query.Get("binding_id")), Cursor: query.Get("cursor"), Limit: limit}
		if query.Has("parent_resource_id") {
			parent := query.Get("parent_resource_id")
			request.ParentResourceID = &parent
		}
		p, _ := Principal(r.Context())
		page, err := reads.Browse(r.Context(), p, request)
		if err != nil {
			readFailure(w, err)
			return
		}
		respond(w, 200, page)
	})))
	h.mux.Handle("GET /api/v1/resources/resolve", h.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query, err := strictQuery(r, "binding_id", "path")
		if err != nil {
			readFailure(w, err)
			return
		}
		p, _ := Principal(r.Context())
		item, err := reads.Resolve(r.Context(), p, storage.BindingID(query.Get("binding_id")), query.Get("path"))
		if err != nil {
			readFailure(w, err)
			return
		}
		respond(w, 200, item)
	})))
	return h, nil
}
func strictQuery(r *http.Request, allowed ...string) (url.Values, error) {
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return nil, product.ErrInvalid
	}
	keys := make(map[string]bool, len(allowed))
	for _, key := range allowed {
		keys[key] = true
	}
	for key, values := range query {
		if !keys[key] || len(values) != 1 {
			return nil, product.ErrInvalid
		}
	}
	return query, nil
}
func readLimit(query url.Values) (int, error) {
	if !query.Has("limit") {
		return 20, nil
	}
	limit, err := strconv.Atoi(query.Get("limit"))
	if err != nil || limit < 1 || limit > 50 {
		return 0, product.ErrInvalid
	}
	return limit, nil
}
func readFailure(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrUnauthorized):
		failure(w, 401, "AUTH_REQUIRED")
	case errors.Is(err, product.ErrInvalid):
		failure(w, 400, "INVALID_REQUEST")
	case errors.Is(err, product.ErrNotFound):
		failure(w, 404, "NOT_FOUND")
	case errors.Is(err, product.ErrAmbiguous):
		failure(w, 409, "RESOURCE_AMBIGUOUS")
	default:
		failure(w, 503, "READ_UNAVAILABLE")
	}
}
