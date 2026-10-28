package producthttp

import (
	"context"
	"embed"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/nathanxiangang-web/panta/internal/adminread"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/product"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

//go:embed web/index.html web/app.css web/resource-display.js web/admin-lists.js web/admin-observe.js web/admin-operations.js web/admin-shares.js web/admin-overview.js web/admin-download-detail.js web/admin-pending-cancel.js web/admin-audit.js web/admin-resource-editor.js web/app.js web/library.html web/library.css web/file-catalog.js web/library.js web/library-sharing.js web/share.html web/share.js web/downloads.html web/downloads.css web/downloads.js web/download-model.js web/file-browser.html web/file-surface.js
var workspaceFiles embed.FS

type DisplayBinding struct {
	ID    storage.BindingID `json:"binding_id"`
	Label string            `json:"label"`
}

func ValidDisplayLabel(label string) bool {
	return utf8.ValidString(label) && strings.TrimSpace(label) != "" && utf8.RuneCountInString(label) <= 100 && !strings.ContainsAny(label, "\x00\r\n")
}

// Canonical browsing coordinates only, never the native provider save-directory ID.
type cloudFileScope struct {
	BindingID storage.BindingID `json:"binding_id"`
	Path      string            `json:"path"`
}

type workspaceInfo struct {
	AdminCopyPresentationEnabled    bool             `json:"admin_copy_presentation_enabled"`
	AdminAuditEnabled               bool             `json:"admin_audit_enabled"`
	AdminPendingCancellationEnabled bool             `json:"admin_pending_cancellation_enabled"`
	CloudDownloadFiles              *cloudFileScope  `json:"cloud_download_files,omitempty"`
	CloudDownloadPreviewEnabled     bool             `json:"cloud_download_preview_enabled"`
	CloudDownloadUnavailableReason  string           `json:"cloud_download_unavailable_reason,omitempty"`
	CloudDownloadEnabled            bool             `json:"cloud_download_enabled"`
	CloudDownloadLabel              string           `json:"cloud_download_label,omitempty"`
	WholeRootLibrary                bool             `json:"whole_root_library"`
	SharingEnabled                  bool             `json:"sharing_enabled"`
	ContentAccess                   bool             `json:"content_access_enabled"`
	Mode                            string           `json:"mode"`
	SchemaVersion                   int64            `json:"schema_version"`
	Bindings                        []DisplayBinding `json:"bindings"`
}
type loginGate struct {
	mu     sync.Mutex
	window time.Time
	used   int
	slots  chan struct{}
	now    func() time.Time
}

func (gate *loginGate) enter() bool {
	gate.mu.Lock()
	defer gate.mu.Unlock()
	now := gate.now()
	if gate.window.IsZero() || now.Sub(gate.window) >= time.Minute {
		gate.window = now
		gate.used = 0
	}
	if gate.used >= 8 {
		return false
	}
	select {
	case gate.slots <- struct{}{}:
		gate.used++
		return true
	default:
		return false
	}
}

type WorkspaceHandler struct {
	mediaSources string
	http         *Handler
	gate         loginGate
	mu           sync.Mutex
	stopping     bool
	active       sync.WaitGroup
}

func NewWorkspaceHandler(access auth.Access, origin string, reads product.Reads, bindings []DisplayBinding, version int64, options ...WorkspaceOption) (*WorkspaceHandler, error) {
	if version < 1 || len(bindings) < 1 || len(bindings) > 64 {
		return nil, product.ErrInvalid
	}
	seen := map[storage.BindingID]bool{}
	for _, binding := range bindings {
		if !product.ValidID(string(binding.ID)) || !ValidDisplayLabel(binding.Label) || seen[binding.ID] {
			return nil, product.ErrInvalid
		}
		seen[binding.ID] = true
	}
	h, err := NewReadHandler(access, origin, reads)
	if err != nil {
		return nil, err
	}
	settings := workspaceOptions{}
	for _, option := range options {
		if option != nil {
			option(&settings)
		}
	}
	registerAdminReads(h, settings.adminReads)
	registerAdminAudit(h, settings.audit)
	registerAdminObservation(h, settings.observation)
	registerAdminOperations(h, settings.operations)
	if settings.files != nil {
		registerContent(h, settings.files, settings.previews)
	}
	if err := registerAccounts(h, access, settings.registration); err != nil {
		return nil, err
	}
	if settings.shares != nil {
		registerSharing(h, settings.shares)
	}
	_, diagnosticsReady := settings.adminReads.(adminread.DownloadDiagnosticReads)
	cancellationReady := false
	if capability, ok := settings.operations.(interface{ PendingCancellationEnabled() bool }); ok {
		cancellationReady = version >= 19 && diagnosticsReady && capability.PendingCancellationEnabled()
	}
	copyPresentationReady := false
	if capability, ok := settings.operations.(interface{ CopyPresentationEnabled() bool }); ok && settings.operations != nil {
		copyPresentationReady = version >= 20 && capability.CopyPresentationEnabled()
	}
	info := workspaceInfo{AdminCopyPresentationEnabled: copyPresentationReady, AdminAuditEnabled: version >= 16 && auditReady(settings.audit), AdminPendingCancellationEnabled: cancellationReady, SharingEnabled: settings.shares != nil, ContentAccess: settings.files != nil, Mode: "READ_ONLY", SchemaVersion: version, Bindings: append([]DisplayBinding(nil), bindings...)}
	if settings.sourcePreview != nil {
		if settings.cloud != nil || !settings.sourcePreview.Valid() {
			return nil, product.ErrInvalid
		}
		registerSourcePreview(h)
		info.CloudDownloadPreviewEnabled = true
		info.CloudDownloadLabel = settings.sourcePreview.Label
		info.CloudDownloadUnavailableReason = settings.sourcePreview.UnavailableReason
	}
	if settings.cloud != nil {
		if !seen[settings.cloud.target.BindingID] {
			return nil, product.ErrInvalid
		}
		if err := registerCloudDownload(h, settings.cloud); err != nil {
			return nil, err
		}
		info.CloudDownloadFiles = &cloudFileScope{BindingID: settings.cloud.target.BindingID, Path: settings.cloud.target.TargetPath}
		info.CloudDownloadEnabled = true
		info.CloudDownloadPreviewEnabled = true
		info.CloudDownloadLabel = settings.cloud.target.Label
		info.Mode = "ACQUISITION"
	}
	consumer := info
	consumer.AdminCopyPresentationEnabled = false
	consumer.AdminAuditEnabled = false
	consumer.AdminPendingCancellationEnabled = false
	consumer.WholeRootLibrary = settings.wholeRoot
	if len(settings.libraryIDs) > 0 {
		consumer.Bindings = nil
		selected := map[storage.BindingID]bool{}
		for _, id := range settings.libraryIDs {
			if !seen[id] || selected[id] {
				return nil, product.ErrInvalid
			}
			selected[id] = true
			for _, binding := range bindings {
				if binding.ID == id {
					consumer.Bindings = append(consumer.Bindings, binding)
				}
			}
		}
	}
	h.mux.Handle("GET /api/v1/workspace", h.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := strictQuery(r); err != nil {
			readFailure(w, err)
			return
		}
		respond(w, 200, consumer)
	})))
	h.mux.Handle("GET /api/v1/admin/workspace", h.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, _ := Principal(r.Context())
		if p.Role != auth.RoleAdmin {
			failure(w, http.StatusForbidden, "ADMIN_REQUIRED")
			return
		}
		if _, err := strictQuery(r); err != nil {
			readFailure(w, err)
			return
		}
		respond(w, 200, info)
	})))
	for _, asset := range []struct{ route, file, mime string }{
		{"GET /downloads/{$}", "web/downloads.html", "text/html; charset=utf-8"},
		{"GET /assets/downloads.js", "web/downloads.js", "text/javascript; charset=utf-8"},
		{"GET /assets/download-model.js", "web/download-model.js", "text/javascript; charset=utf-8"},
		{"GET /assets/downloads.css", "web/downloads.css", "text/css; charset=utf-8"},
		{"GET /{$}", "web/library.html", "text/html; charset=utf-8"},
		{"GET /admin/{$}", "web/index.html", "text/html; charset=utf-8"},
		{"GET /assets/library.css", "web/library.css", "text/css; charset=utf-8"},
		{"GET /assets/library.js", "web/library.js", "text/javascript; charset=utf-8"},
		{"GET /assets/library-sharing.js", "web/library-sharing.js", "text/javascript; charset=utf-8"},
		{"GET /s/{$}", "web/share.html", "text/html; charset=utf-8"},
		{"GET /assets/share.js", "web/share.js", "text/javascript; charset=utf-8"},
		{"GET /assets/file-surface.js", "web/file-surface.js", "text/javascript; charset=utf-8"},
		{"GET /assets/file-catalog.js", "web/file-catalog.js", "text/javascript; charset=utf-8"},
		{"GET /assets/app.css", "web/app.css", "text/css; charset=utf-8"},
		{"GET /assets/resource-display.js", "web/resource-display.js", "text/javascript; charset=utf-8"},
		{"GET /assets/app.js", "web/app.js", "text/javascript; charset=utf-8"},
		{"GET /assets/admin-lists.js", "web/admin-lists.js", "text/javascript; charset=utf-8"},
		{"GET /assets/admin-observe.js", "web/admin-observe.js", "text/javascript; charset=utf-8"},
		{"GET /assets/admin-operations.js", "web/admin-operations.js", "text/javascript; charset=utf-8"},
		{"GET /assets/admin-shares.js", "web/admin-shares.js", "text/javascript; charset=utf-8"},
		{"GET /assets/admin-overview.js", "web/admin-overview.js", "text/javascript; charset=utf-8"},
		{"GET /assets/admin-download-detail.js", "web/admin-download-detail.js", "text/javascript; charset=utf-8"},
		{"GET /assets/admin-pending-cancel.js", "web/admin-pending-cancel.js", "text/javascript; charset=utf-8"},
		{"GET /assets/admin-resource-editor.js", "web/admin-resource-editor.js", "text/javascript; charset=utf-8"},
		{"GET /assets/admin-audit.js", "web/admin-audit.js", "text/javascript; charset=utf-8"},
	} {
		content, err := workspaceFiles.ReadFile(asset.file)
		if err != nil {
			return nil, product.ErrUnavailable
		}
		if strings.Contains(string(content), "<!--FILE_BROWSER-->") {
			fragment, err := workspaceFiles.ReadFile("web/file-browser.html")
			if err != nil {
				return nil, product.ErrUnavailable
			}
			content = []byte(strings.ReplaceAll(string(content), "<!--FILE_BROWSER-->", string(fragment)))
		}
		h.mux.HandleFunc(asset.route, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", asset.mime)
			_, _ = w.Write(content)
		})
	}
	mediaSources := ""
	if settings.files != nil {
		mediaSources = strings.Join(settings.files.MediaSources(), " ")
	}
	return &WorkspaceHandler{http: h, mediaSources: mediaSources, gate: loginGate{slots: make(chan struct{}, 2), now: time.Now}}, nil
}
func (h *WorkspaceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self' data:; media-src 'self' "+h.mediaSources+"; base-uri 'none'; form-action 'self'; frame-ancestors 'none'; object-src 'none'")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	h.mu.Lock()
	if h.stopping {
		h.mu.Unlock()
		failure(w, 503, "SERVICE_STOPPING")
		return
	}
	h.active.Add(1)
	h.mu.Unlock()
	defer h.active.Done()
	lifetime := 15 * time.Second
	if (r.Method == "GET" || r.Method == "HEAD") && (r.URL.Path == "/api/v1/files/content" || strings.HasPrefix(r.URL.Path, "/api/v1/shared/")) {
		lifetime = 30 * time.Minute
	}
	if r.Method == "GET" && r.URL.Path == "/api/v1/files/preview" {
		lifetime = 75 * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), lifetime)
	defer cancel()
	r = r.WithContext(ctx)
	if r.Method == "POST" && (r.URL.Path == "/api/v1/auth/login" || r.URL.Path == "/api/v1/auth/register") && h.http.ValidMutationOrigin(r) {
		if !h.gate.enter() {
			w.Header().Set("Retry-After", "60")
			failure(w, 429, "AUTH_BUSY")
			return
		}
		defer func() { <-h.gate.slots }()
	}
	h.http.ServeHTTP(w, r)
}

// Wait seals request admission before joining handlers; a racing HTTP request
// cannot add backend work after the pool-owning process begins teardown.
func (h *WorkspaceHandler) Wait() { h.mu.Lock(); h.stopping = true; h.mu.Unlock(); h.active.Wait() }
