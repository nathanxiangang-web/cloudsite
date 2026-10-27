package producthttp

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"unicode/utf8"

	"github.com/nathanxiangang-web/panta/internal/adminops"
	"github.com/nathanxiangang-web/panta/internal/auth"
)

func WithAdminOperations(ops adminops.Operations) WorkspaceOption {
	return func(o *workspaceOptions) { o.operations = ops }
}
func registerAdminOperations(h *Handler, ops adminops.Operations) {
	for pattern, action := range map[string]string{
		"POST /api/v1/admin/users":                                  adminops.CreateUser,
		"PATCH /api/v1/admin/users/{user_id}":                       adminops.SetEnabled,
		"POST /api/v1/admin/users/{user_id}/rename":                 adminops.RenameUser,
		"POST /api/v1/admin/users/{user_id}/password":               adminops.ResetPassword,
		"DELETE /api/v1/admin/users/{user_id}":                      adminops.DeleteUser,
		"POST /api/v1/admin/users/{user_id}/restore":                adminops.RestoreUser,
		"POST /api/v1/admin/storage/{binding_id}/probe":             adminops.ProbeAccount,
		"POST /api/v1/admin/shares/{share_id}/revoke":               adminops.RevokeShare,
		"GET /api/v1/admin/actions/{request_id}":                    "STATUS",
		"POST /api/v1/admin/downloads/{manifest_id}/cancel-pending": adminops.CancelPending,
	} {
		h.mux.Handle(pattern, h.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, _ := Principal(r.Context())
			if p.Role != auth.RoleAdmin {
				failure(w, 403, "ADMIN_REQUIRED")
				return
			}
			if r.Method == "HEAD" {
				failure(w, 405, "METHOD_NOT_ALLOWED")
				return
			}
			if !adminAcceptsJSON(r.Header.Values("Accept")) {
				failure(w, 406, "NOT_ACCEPTABLE")
				return
			}
			if _, e := strictQuery(r); e != nil {
				failure(w, 400, "INVALID_REQUEST")
				return
			}
			if action != "STATUS" && !h.ValidMutationOrigin(r) {
				failure(w, 403, "ORIGIN_REJECTED")
				return
			}
			if ops == nil {
				failure(w, 503, "ADMIN_OPERATION_UNAVAILABLE")
				return
			}
			actor := adminops.Actor{Principal: p, SessionDigest: auth.TokenDigest(h.cookie(r))}
			var result adminops.Result
			var e error
			if action == "STATUS" {
				result, e = ops.Status(r.Context(), actor, r.PathValue("request_id"))
			} else {
				fields := []string{"request_id"}
				if action == adminops.CreateUser || action == adminops.RenameUser {
					fields = append(fields, "login")
				}
				if action == adminops.CreateUser || action == adminops.ResetPassword || action == adminops.RestoreUser {
					fields = append(fields, "password")
				}
				if action == adminops.SetEnabled {
					fields = append(fields, "enabled")
				}
				if action != adminops.CreateUser && action != adminops.ProbeAccount && action != adminops.RevokeShare && action != adminops.CancelPending {
					fields = append(fields, "expected_version")
				}
				if action == adminops.CancelPending {
					fields = append(fields, "expected_owner_user_id", "expected_binding_id", "expected_updated_at")
				}
				body, err := adminCommandBody(w, r, fields)
				if err != nil {
					failure(w, 400, "INVALID_REQUEST")
					return
				}
				c := adminops.Command{Action: action, TargetID: r.PathValue("user_id")}
				if action == adminops.CancelPending {
					c.TargetID = r.PathValue("manifest_id")
					c.Pending = &adminops.PendingPrecondition{}
					if json.Unmarshal(body["expected_owner_user_id"], &c.Pending.OwnerUserID) != nil || json.Unmarshal(body["expected_binding_id"], &c.Pending.BindingID) != nil || json.Unmarshal(body["expected_updated_at"], &c.Pending.UpdatedAt) != nil {
						failure(w, 400, "INVALID_REQUEST")
						return
					}
				}
				if action == adminops.ProbeAccount {
					c.TargetID = r.PathValue("binding_id")
				}
				if action == adminops.RevokeShare {
					c.TargetID = r.PathValue("share_id")
				}
				if json.Unmarshal(body["request_id"], &c.RequestID) != nil {
					failure(w, 400, "INVALID_REQUEST")
					return
				}
				if _, ok := body["login"]; ok {
					if json.Unmarshal(body["login"], &c.Login) != nil {
						failure(w, 400, "INVALID_REQUEST")
						return
					}
				}
				if _, ok := body["password"]; ok {
					if json.Unmarshal(body["password"], &c.Password) != nil {
						failure(w, 400, "INVALID_REQUEST")
						return
					}
				}
				if _, ok := body["expected_version"]; ok {
					if json.Unmarshal(body["expected_version"], &c.ExpectedVersion) != nil {
						failure(w, 400, "INVALID_REQUEST")
						return
					}
				}
				if action == adminops.SetEnabled {
					if bytes.Equal(body["enabled"], []byte("null")) || json.Unmarshal(body["enabled"], &c.Enabled) != nil || bytes.Equal(body["expected_version"], []byte("null")) || json.Unmarshal(body["expected_version"], &c.ExpectedVersion) != nil {
						failure(w, 400, "INVALID_REQUEST")
						return
					}
				}
				defer func() {
					for _, raw := range body {
						clear(raw)
					}
				}()
				result, e = ops.Execute(r.Context(), actor, c)
				c.Password = ""
			}
			if e != nil {
				adminOperationFailure(w, e)
				return
			}
			status := 200
			if result.Status == "UNKNOWN" {
				status = 202
			}
			if action != "STATUS" && result.Action == adminops.CancelPending && result.Status == "REJECTED" {
				status = 409
				if result.Code == "MANIFEST_NOT_FOUND" {
					status = 404
				}
			}
			respond(w, status, result)
		})))
	}
}
func adminCommandBody(w http.ResponseWriter, r *http.Request, fields []string) (map[string]json.RawMessage, error) {
	media, _, e := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if e != nil || media != "application/json" || r.Body == nil {
		return nil, adminops.ErrInvalid
	}
	raw, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
	defer clear(raw)
	if e != nil || !utf8.Valid(raw) {
		return nil, adminops.ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, e := decoder.Token()
	if e != nil || token != json.Delim('{') {
		return nil, adminops.ErrInvalid
	}
	result := map[string]json.RawMessage{}
	allowed := map[string]bool{}
	for _, k := range fields {
		allowed[k] = true
	}
	for decoder.More() {
		token, e := decoder.Token()
		k, ok := token.(string)
		if e != nil || !ok || !allowed[k] || result[k] != nil {
			return nil, adminops.ErrInvalid
		}
		var v json.RawMessage
		if decoder.Decode(&v) != nil || bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			return nil, adminops.ErrInvalid
		}
		result[k] = v
	}
	if token, e = decoder.Token(); e != nil || token != json.Delim('}') || decoder.Decode(new(any)) != io.EOF || len(result) != len(fields) {
		return nil, adminops.ErrInvalid
	}
	return result, nil
}
func adminOperationFailure(w http.ResponseWriter, e error) {
	switch {
	case errors.Is(e, auth.ErrUnauthorized):
		failure(w, 401, "AUTH_REQUIRED")
	case errors.Is(e, adminops.ErrForbidden):
		failure(w, 403, "ADMIN_REQUIRED")
	case errors.Is(e, adminops.ErrInvalid):
		failure(w, 400, "INVALID_REQUEST")
	case errors.Is(e, adminops.ErrConflict):
		failure(w, 409, "ACTION_CONFLICT")
	case errors.Is(e, adminops.ErrNotFound):
		failure(w, 404, "ACTION_NOT_FOUND")
	default:
		failure(w, 503, "ACTION_UNCONFIRMED")
	}
}
