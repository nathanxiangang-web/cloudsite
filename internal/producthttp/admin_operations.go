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
	for pattern, action := range map[string]string{"POST /api/v1/admin/users": adminops.CreateUser, "PATCH /api/v1/admin/users/{user_id}": adminops.SetEnabled, "POST /api/v1/admin/storage/{binding_id}/probe": adminops.ProbeAccount, "GET /api/v1/admin/actions/{request_id}": "STATUS"} {
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
				if action == adminops.CreateUser {
					fields = append(fields, "login", "password")
				}
				if action == adminops.SetEnabled {
					fields = append(fields, "enabled", "expected_version")
				}
				body, err := adminCommandBody(w, r, fields)
				if err != nil {
					failure(w, 400, "INVALID_REQUEST")
					return
				}
				c := adminops.Command{Action: action, TargetID: r.PathValue("user_id")}
				if action == adminops.ProbeAccount {
					c.TargetID = r.PathValue("binding_id")
				}
				if json.Unmarshal(body["request_id"], &c.RequestID) != nil {
					failure(w, 400, "INVALID_REQUEST")
					return
				}
				if action == adminops.CreateUser {
					if json.Unmarshal(body["login"], &c.Login) != nil || json.Unmarshal(body["password"], &c.Password) != nil {
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
