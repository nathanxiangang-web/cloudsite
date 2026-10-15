package producthttp

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/fileaccess"
	"github.com/nathanxiangang-web/panta/internal/fileshare"
	"github.com/nathanxiangang-web/panta/internal/storage"
	"io"
	"mime"
	"net/http"
	"strconv"
	"time"
	"unicode/utf8"
)

func shareBody(w http.ResponseWriter, r *http.Request, allowed ...string) (map[string]string, error) {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || r.Body == nil {
		return nil, fileshare.ErrInvalid
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8192))
	if err != nil || !utf8.Valid(raw) {
		return nil, fileshare.ErrInvalid
	}
	defer clear(raw)
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, fileshare.ErrInvalid
	}
	result := map[string]string{}
	keys := map[string]bool{}
	for _, key := range allowed {
		keys[key] = true
	}
	for decoder.More() {
		token, err = decoder.Token()
		key, ok := token.(string)
		_, seen := result[key]
		if err != nil || !ok || seen || !keys[key] {
			return nil, fileshare.ErrInvalid
		}
		var value string
		if decoder.Decode(&value) != nil {
			return nil, fileshare.ErrInvalid
		}
		result[key] = value
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') || decoder.Decode(new(any)) != io.EOF {
		return nil, fileshare.ErrInvalid
	}
	return result, nil
}
func shareFailure(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, fileshare.ErrInvalid):
		failure(w, 400, "INVALID_REQUEST")
	case errors.Is(err, fileshare.ErrLimit):
		failure(w, 429, "SHARE_LIMIT")
	case errors.Is(err, fileshare.ErrDenied):
		failure(w, 404, "SHARE_UNAVAILABLE")
	case errors.Is(err, fileshare.ErrUnavailable):
		failure(w, 503, "SHARE_SERVICE_UNAVAILABLE")
	default:
		contentFailure(w, err)
	}
}
func registerSharing(h *Handler, shares *fileshare.Service) {
	gate := &loginGate{slots: make(chan struct{}, 2), now: time.Now}
	createGate := &loginGate{slots: make(chan struct{}, 2), now: time.Now}
	streams := &streamGate{users: map[auth.UserID]int{}}
	h.mux.Handle("POST /api/v1/shares", h.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !createGate.enter() {
			failure(w, 429, "SHARE_BUSY")
			return
		}
		defer func() { <-createGate.slots }()
		if _, err := strictQuery(r); err != nil {
			shareFailure(w, fileshare.ErrInvalid)
			return
		}
		body, err := shareBody(w, r, "binding_id", "resource_id", "path", "duration", "code")
		if err != nil {
			shareFailure(w, err)
			return
		}
		p, _ := Principal(r.Context())
		record, token, err := shares.Create(r.Context(), p, fileaccess.Request{BindingID: storage.BindingID(body["binding_id"]), ResourceID: body["resource_id"], Path: body["path"]}, body["duration"], body["code"])
		if err != nil {
			shareFailure(w, err)
			return
		}
		respond(w, 201, struct {
			fileshare.Record
			URL string `json:"url"`
		}{record, h.origin + "/s/#" + token})
	})))
	h.mux.Handle("GET /api/v1/shares", h.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := strictQuery(r); err != nil {
			shareFailure(w, fileshare.ErrInvalid)
			return
		}
		p, _ := Principal(r.Context())
		items, err := shares.List(r.Context(), p)
		if err != nil {
			shareFailure(w, err)
			return
		}
		respond(w, 200, struct {
			Items []fileshare.Record `json:"items"`
		}{items})
	})))
	h.mux.Handle("POST /api/v1/shares/{share_id}/revoke", h.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := strictQuery(r); err != nil {
			shareFailure(w, fileshare.ErrInvalid)
			return
		}
		if _, err := shareBody(w, r); err != nil {
			shareFailure(w, err)
			return
		}
		p, _ := Principal(r.Context())
		if err := shares.Revoke(r.Context(), p, r.PathValue("share_id")); err != nil {
			shareFailure(w, err)
			return
		}
		w.WriteHeader(204)
	})))
	h.mux.HandleFunc("POST /api/v1/shares/redeem", func(w http.ResponseWriter, r *http.Request) {
		if !gate.enter() {
			failure(w, 429, "SHARE_BUSY")
			return
		}
		defer func() { <-gate.slots }()
		if _, err := strictQuery(r); err != nil {
			shareFailure(w, fileshare.ErrInvalid)
			return
		}
		body, err := shareBody(w, r, "token", "code")
		if err != nil {
			shareFailure(w, err)
			return
		}
		grant, err := shares.Redeem(r.Context(), body["token"], body["code"])
		if err != nil {
			shareFailure(w, err)
			return
		}
		path := "/api/v1/shared/" + grant.Record.ID + "/"
		http.SetCookie(w, &http.Cookie{Name: "panta_share", Value: grant.Token, Path: path, HttpOnly: true, Secure: h.secure, SameSite: http.SameSiteStrictMode, Expires: grant.Record.ExpiresAt})
		respond(w, 200, struct {
			Name     string    `json:"name"`
			Size     int64     `json:"size_bytes"`
			Expires  time.Time `json:"expires_at"`
			Download string    `json:"download_url"`
		}{grant.Record.Name, grant.Record.Size, grant.Record.ExpiresAt, path + "content"})
	})
	h.mux.HandleFunc("GET /api/v1/shared/{share_id}/content", func(w http.ResponseWriter, r *http.Request) {
		if _, err := strictQuery(r); err != nil {
			shareFailure(w, fileshare.ErrInvalid)
			return
		}
		if !streams.enter("guest") {
			failure(w, 429, "CONTENT_BUSY")
			return
		}
		defer streams.leave("guest")
		token := ""
		count := 0
		for _, cookie := range r.Cookies() {
			if cookie.Name == "panta_share" {
				token = cookie.Value
				count++
			}
		}
		if count != 1 {
			shareFailure(w, fileshare.ErrDenied)
			return
		}
		if len(r.Header.Values("Range")) > 1 {
			failure(w, 416, "RANGE_NOT_SATISFIABLE")
			return
		}
		file, err := shares.Open(r.Context(), r.PathValue("share_id"), token, r.Header.Get("Range"), r.Method == http.MethodHead, r.UserAgent())
		if err != nil {
			if errors.Is(err, fileaccess.ErrRange) {
				w.Header().Set("Content-Range", "bytes */"+strconv.FormatInt(file.Size, 10))
				failure(w, 416, "RANGE_NOT_SATISFIABLE")
			} else {
				shareFailure(w, err)
			}
			return
		}
		writeFile(w, r, file, "attachment")
	})
}
