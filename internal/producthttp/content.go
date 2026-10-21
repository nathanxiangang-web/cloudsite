package producthttp

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/nathanxiangang-web/panta/internal/adminread"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/fileaccess"
	"github.com/nathanxiangang-web/panta/internal/fileshare"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

type WorkspaceOption func(*workspaceOptions)
type workspaceOptions struct {
	adminReads    adminread.Reads
	sourcePreview *SourcePreviewConfig
	cloud         *cloudDownloadOptions
	registration  bool
	libraryIDs    []storage.BindingID
	wholeRoot     bool
	shares        *fileshare.Service
	files         *fileaccess.Service
	previews      *fileaccess.PreviewService
}

func WithRegistration(enabled bool) WorkspaceOption {
	return func(o *workspaceOptions) { o.registration = enabled }
}

func WithLibraryScope(ids []storage.BindingID, wholeRoot bool) WorkspaceOption {
	return func(o *workspaceOptions) {
		o.libraryIDs = append([]storage.BindingID(nil), ids...)
		o.wholeRoot = wholeRoot
	}
}

func WithSharing(shares *fileshare.Service) WorkspaceOption {
	return func(o *workspaceOptions) { o.shares = shares }
}

func WithFileAccess(service *fileaccess.Service, previews ...*fileaccess.PreviewService) WorkspaceOption {
	return func(o *workspaceOptions) {
		o.files = service
		if len(previews) > 0 {
			o.previews = previews[0]
		}
	}
}

type streamGate struct {
	mu    sync.Mutex
	total int
	users map[auth.UserID]int
}

func (g *streamGate) enter(user auth.UserID) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.total >= 16 || g.users[user] >= 4 {
		return false
	}
	g.total++
	g.users[user]++
	return true
}
func (g *streamGate) leave(user auth.UserID) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.total--
	g.users[user]--
	if g.users[user] == 0 {
		delete(g.users, user)
	}
}

func registerContent(h *Handler, service *fileaccess.Service, previews *fileaccess.PreviewService) {
	gate := &streamGate{users: map[auth.UserID]int{}}
	for _, mode := range []string{"content", "preview"} {
		handler := h.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			allowed := []string{"binding_id", "resource_id", "path", "disposition"}
			if mode == "preview" {
				allowed = append(allowed, "page")
			}
			query, err := strictQuery(r, allowed...)
			if err != nil {
				readFailure(w, err)
				return
			}
			disposition := query.Get("disposition")
			if disposition != "" && disposition != "attachment" && disposition != "inline" {
				failure(w, 400, "INVALID_REQUEST")
				return
			}
			p, _ := Principal(r.Context())
			if !gate.enter(p.UserID) {
				failure(w, 429, "CONTENT_BUSY")
				return
			}
			defer gate.leave(p.UserID)
			requestedRange := r.Header.Get("Range")
			if len(r.Header.Values("Range")) > 1 {
				failure(w, 416, "RANGE_NOT_SATISFIABLE")
				return
			}
			if mode == "preview" && requestedRange != "" {
				failure(w, 400, "INVALID_REQUEST")
				return
			}
			input := fileaccess.Request{BindingID: storage.BindingID(query.Get("binding_id")), ResourceID: query.Get("resource_id"), Path: query.Get("path"), Range: requestedRange, Head: r.Method == http.MethodHead, UserAgent: r.UserAgent()}
			if mode == "preview" {
				if previews == nil {
					failure(w, 503, "CONTENT_UNAVAILABLE")
					return
				}
				if r.Method == http.MethodHead {
					failure(w, 405, "METHOD_NOT_ALLOWED")
					return
				}
				page := 1
				if query.Has("page") {
					page, err = strconv.Atoi(query.Get("page"))
					if err != nil || page < 1 || page > 2000 {
						failure(w, 400, "INVALID_REQUEST")
						return
					}
				}
				_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(90 * time.Second))
				result, err := previews.Preview(r.Context(), p, input, page)
				if err != nil {
					contentFailure(w, err)
					return
				}
				respond(w, 200, result)
				return
			}
			file, err := service.Deliver(r.Context(), p, input)
			if err != nil {
				if errors.Is(err, fileaccess.ErrRange) {
					w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", file.Size))
					failure(w, 416, "RANGE_NOT_SATISFIABLE")
				} else {
					contentFailure(w, err)
				}
				return
			}
			writeFile(w, r, file, disposition)
		}))
		h.mux.Handle("GET /api/v1/files/"+mode, handler)
	}
}
func writeFile(w http.ResponseWriter, r *http.Request, file fileaccess.File, disposition string) {
	if file.DirectURL != "" {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Location", file.DirectURL)
		w.WriteHeader(http.StatusFound)
		return
	}
	defer file.Content.Body.Close()
	media := inlineType(file.Extension)
	if disposition != "inline" || media == "" {
		disposition = "attachment"
		media = "application/octet-stream"
	}
	w.Header().Set("Content-Type", media)
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": file.Name}))
	w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'self'; base-uri 'none'; form-action 'none'")
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Length", fmt.Sprint(file.Content.Length))
	if file.Content.ContentRange != "" {
		w.Header().Set("Content-Range", file.Content.ContentRange)
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(30 * time.Minute))
	w.WriteHeader(file.Content.Status)
	if r.Method != http.MethodHead {
		_, _ = io.CopyN(w, file.Content.Body, file.Content.Length)
	}
}
func contentFailure(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, fileaccess.ErrUnsupported):
		failure(w, 415, "PREVIEW_UNSUPPORTED")
	case errors.Is(err, fileaccess.ErrTooLarge):
		failure(w, 413, "PREVIEW_TOO_LARGE")
	case errors.Is(err, fileaccess.ErrUnavailable):
		failure(w, 503, "CONTENT_UNAVAILABLE")
	default:
		readFailure(w, err)
	}
}
func inlineType(extension string) string {
	return map[string]string{"pdf": "application/pdf", "mp4": "video/mp4", "webm": "video/webm", "m4v": "video/mp4", "mov": "video/quicktime", "mp3": "audio/mpeg", "m4a": "audio/mp4", "ogg": "audio/ogg", "wav": "audio/wav", "jpg": "image/jpeg", "jpeg": "image/jpeg", "png": "image/png", "gif": "image/gif", "webp": "image/webp"}[extension]
}
