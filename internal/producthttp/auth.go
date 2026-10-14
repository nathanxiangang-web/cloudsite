// Package producthttp is the product transport boundary. It does not own
// credentials, persistence, provider execution or resource truth.
package producthttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nathanxiangang-web/panta/internal/auth"
)

const SessionCookieName = "panta_session"

type principalKey struct{}

func Principal(ctx context.Context) (auth.Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(auth.Principal)
	return p, ok && p.Valid()
}

type Handler struct {
	access auth.Access
	origin string
	host   string
	secure bool
	mux    *http.ServeMux
}

func NewAuthHandler(access auth.Access, origin string) (*Handler, error) {
	u, err := url.Parse(origin)
	if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || (u.Path != "" && u.Path != "/") || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, auth.ErrInvalid
	}
	if u.Scheme == "http" {
		ip := net.ParseIP(u.Hostname())
		if u.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return nil, auth.ErrInvalid
		}
	}
	h := &Handler{access: access, origin: u.Scheme + "://" + strings.ToLower(u.Host), host: strings.ToLower(u.Host), secure: u.Scheme == "https", mux: http.NewServeMux()}
	h.mux.HandleFunc("POST /api/v1/auth/login", h.login)
	h.mux.Handle("GET /api/v1/auth/me", h.Protect(http.HandlerFunc(h.me)))
	h.mux.HandleFunc("POST /api/v1/auth/logout", h.logout)
	return h, nil
}
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if strings.ToLower(r.Host) != h.host {
		failure(w, http.StatusForbidden, "ORIGIN_REJECTED")
		return
	}
	if r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" {
		if !h.ValidMutationOrigin(r) {
			failure(w, http.StatusForbidden, "ORIGIN_REJECTED")
			return
		}
	}
	h.mux.ServeHTTP(w, r)
}
func (h *Handler) ValidMutationOrigin(r *http.Request) bool {
	values := r.Header.Values("Origin")
	return len(values) == 1 && values[0] == h.origin && strings.ToLower(r.Host) == h.host && r.Header.Get("Sec-Fetch-Site") != "cross-site"
}
func (h *Handler) cookie(r *http.Request) string {
	var token string
	count := 0
	for _, c := range r.Cookies() {
		if c.Name == SessionCookieName {
			count++
			token = c.Value
		}
	}
	if count != 1 || !auth.ValidToken(token) {
		return ""
	}
	return token
}
func (h *Handler) Protect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.access == nil || next == nil {
			failure(w, 503, "AUTH_UNAVAILABLE")
			return
		}
		token := h.cookie(r)
		if token == "" {
			failure(w, 401, "AUTH_REQUIRED")
			return
		}
		p, err := h.access.Authenticate(r.Context(), token)
		if err != nil {
			authFailure(w, err)
			return
		}
		if !p.Valid() {
			failure(w, 503, "AUTH_UNAVAILABLE")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, p)))
	})
}
func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	if h.access == nil {
		failure(w, 503, "AUTH_UNAVAILABLE")
		return
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || r.Body == nil {
		failure(w, 400, "INVALID_REQUEST")
		return
	}
	var request struct {
		Login    string
		Password string
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8192))
	defer clear(raw)
	if err != nil || !utf8.Valid(raw) {
		failure(w, 400, "INVALID_REQUEST")
		return
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		failure(w, 400, "INVALID_REQUEST")
		return
	}
	seen := map[string]bool{}
	for decoder.More() {
		key, err := decoder.Token()
		name, ok := key.(string)
		if err != nil || !ok || seen[name] || (name != "login" && name != "password") {
			failure(w, 400, "INVALID_REQUEST")
			return
		}
		seen[name] = true
		var value string
		if decoder.Decode(&value) != nil {
			failure(w, 400, "INVALID_REQUEST")
			return
		}
		if name == "login" {
			request.Login = value
		} else {
			request.Password = value
		}
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') || decoder.Decode(new(any)) != io.EOF {
		failure(w, 400, "INVALID_REQUEST")
		return
	}
	if _, err = auth.NormalizeLogin(request.Login); err != nil || !auth.ValidPassword(request.Password) {
		failure(w, 400, "INVALID_REQUEST")
		return
	}
	result, err := h.access.Login(r.Context(), request.Login, request.Password)
	request.Password = ""
	if err != nil {
		authFailure(w, err)
		return
	}
	if !result.Principal.Valid() || !auth.ValidToken(result.Token) || result.ExpiresAt.IsZero() {
		failure(w, 503, "AUTH_UNAVAILABLE")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: SessionCookieName, Value: result.Token, Path: "/", HttpOnly: true, Secure: h.secure, SameSite: http.SameSiteStrictMode, MaxAge: int(auth.SessionTTL.Seconds()), Expires: result.ExpiresAt})
	respond(w, 200, result.Principal)
}
func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	p, ok := Principal(r.Context())
	if !ok {
		failure(w, 503, "AUTH_UNAVAILABLE")
		return
	}
	respond(w, 200, p)
}
func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if h.access == nil {
		failure(w, 503, "AUTH_UNAVAILABLE")
		return
	}
	if err := h.access.Logout(r.Context(), h.cookie(r)); err != nil {
		authFailure(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: SessionCookieName, Value: "", Path: "/", HttpOnly: true, Secure: h.secure, SameSite: http.SameSiteStrictMode, MaxAge: -1, Expires: time.Unix(1, 0)})
	w.WriteHeader(http.StatusNoContent)
}
func authFailure(w http.ResponseWriter, err error) {
	if errors.Is(err, auth.ErrUnauthorized) {
		failure(w, 401, "AUTH_REQUIRED")
		return
	}
	if errors.Is(err, auth.ErrInvalid) {
		failure(w, 400, "INVALID_REQUEST")
		return
	}
	failure(w, 503, "AUTH_UNAVAILABLE")
}
func failure(w http.ResponseWriter, status int, code string) {
	respond(w, status, struct {
		Code string `json:"code"`
	}{code})
}
func respond(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
