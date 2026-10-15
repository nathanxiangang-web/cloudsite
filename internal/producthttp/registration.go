package producthttp

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"reflect"
	"unicode/utf8"

	"github.com/nathanxiangang-web/panta/internal/auth"
)

// Registration is an explicit composition capability; legacy deployments stay
// closed. The public transport never receives an operator Provision capability.
func registerAccounts(h *Handler, access auth.Access, enabled bool) error {
	registrar, ok := access.(auth.Registrar)
	if enabled && (!ok || registrar == nil || (reflect.ValueOf(registrar).Kind() == reflect.Ptr && reflect.ValueOf(registrar).IsNil())) {
		return auth.ErrUnavailable
	}
	h.mux.HandleFunc("GET /api/v1/auth/capabilities", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			failure(w, 400, "INVALID_REQUEST")
			return
		}
		respond(w, 200, struct {
			Enabled bool `json:"registration_enabled"`
		}{enabled})
	})
	if !enabled {
		return nil
	}
	h.mux.HandleFunc("POST /api/v1/auth/register", func(w http.ResponseWriter, r *http.Request) {
		login, password, valid := registrationCredentials(w, r)
		if !valid {
			failure(w, 400, "INVALID_REQUEST")
			return
		}
		principal, err := registrar.Register(r.Context(), login, password)
		password = ""
		if errors.Is(err, auth.ErrConflict) {
			failure(w, 409, "ACCOUNT_EXISTS")
			return
		}
		if err != nil {
			authFailure(w, err)
			return
		}
		if !principal.Valid() || principal.Role != auth.RoleUser {
			failure(w, 503, "AUTH_UNAVAILABLE")
			return
		}
		// Creation does not issue a session. Login retains its independent checks.
		respond(w, http.StatusCreated, principal)
	})
	return nil
}

func registrationCredentials(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || r.Body == nil || r.URL.RawQuery != "" {
		return "", "", false
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8192))
	defer clear(raw)
	if err != nil || !utf8.Valid(raw) {
		return "", "", false
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return "", "", false
	}
	values := map[string]string{}
	for d.More() {
		key, err := d.Token()
		name, ok := key.(string)
		if err != nil || !ok || (name != "login" && name != "password") {
			return "", "", false
		}
		if _, duplicate := values[name]; duplicate {
			return "", "", false
		}
		var value string
		if d.Decode(&value) != nil {
			return "", "", false
		}
		values[name] = value
	}
	if token, err = d.Token(); err != nil || token != json.Delim('}') || d.Decode(new(any)) != io.EOF {
		return "", "", false
	}
	login, err := auth.NormalizeLogin(values["login"])
	if err != nil || !auth.ValidPassword(values["password"]) {
		return "", "", false
	}
	return login, values["password"], true
}
