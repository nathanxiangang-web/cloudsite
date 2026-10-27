package producthttp

import (
	"encoding/json"
	"github.com/nathanxiangang-web/panta/internal/adminops"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/copypresentation"
	"net/http"
	"reflect"
)

func registerAdminCopyPresentation(h *Handler, ops adminops.Operations) {
	h.mux.Handle("/api/v1/admin/copies/{copy_id}/presentation", h.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, _ := Principal(r.Context())
		if p.Role != auth.RoleAdmin {
			failure(w, 403, "ADMIN_REQUIRED")
			return
		}
		if r.Method != "GET" && r.Method != "PUT" {
			failure(w, 405, "METHOD_NOT_ALLOWED")
			return
		}
		if !adminAcceptsJSON(r.Header.Values("Accept")) {
			failure(w, 406, "NOT_ACCEPTABLE")
			return
		}
		if ops == nil || reflect.ValueOf(ops).Kind() == reflect.Pointer && reflect.ValueOf(ops).IsNil() {
			failure(w, 503, "ADMIN_OPERATION_UNAVAILABLE")
			return
		}
		a := adminops.Actor{Principal: p, SessionDigest: auth.TokenDigest(h.cookie(r))}
		if r.Method == "GET" {
			q, e := strictQuery(r, "binding_id")
			if e != nil || !q.Has("binding_id") {
				failure(w, 400, "INVALID_REQUEST")
				return
			}
			reader, ok := ops.(adminops.CopyPresentationReads)
			if !ok {
				failure(w, 503, "ADMIN_OPERATION_UNAVAILABLE")
				return
			}
			d, e := reader.CopyPresentation(r.Context(), a, r.PathValue("copy_id"), q.Get("binding_id"))
			if e != nil {
				adminOperationFailure(w, e)
				return
			}
			respond(w, 200, d)
			return
		}
		if _, e := strictQuery(r); e != nil {
			failure(w, 400, "INVALID_REQUEST")
			return
		}
		if !h.ValidMutationOrigin(r) {
			failure(w, 403, "ORIGIN_REJECTED")
			return
		}
		if ops == nil {
			failure(w, 503, "ADMIN_OPERATION_UNAVAILABLE")
			return
		}
		raw, e := adminCommandBody(w, r, []string{"request_id", "expected_version", "binding_id", "display_name", "category", "hidden"})
		if e != nil {
			failure(w, 400, "INVALID_REQUEST")
			return
		}
		c := adminops.Command{Action: adminops.SetCopyPresentation, TargetID: r.PathValue("copy_id"), Presentation: &copypresentation.Change{}}
		for k, v := range map[string]any{"request_id": &c.RequestID, "expected_version": &c.ExpectedVersion, "binding_id": &c.Presentation.BindingID, "display_name": &c.Presentation.DisplayName, "category": &c.Presentation.Category, "hidden": &c.Presentation.Hidden} {
			if json.Unmarshal(raw[k], v) != nil {
				failure(w, 400, "INVALID_REQUEST")
				return
			}
		}
		// The name validator also rejects replacement characters from lone surrogates.
		if !c.Valid() {
			failure(w, 400, "INVALID_REQUEST")
			return
		}
		result, e := ops.Execute(r.Context(), a, c)
		if e != nil {
			adminOperationFailure(w, e)
			return
		}
		status := 200
		if result.Status == "UNKNOWN" {
			status = 202
		}
		if result.Status == "REJECTED" {
			status = 409
			if result.Code == "COPY_NOT_FOUND" {
				status = 404
			}
		}
		respond(w, status, result)
	})))
}
