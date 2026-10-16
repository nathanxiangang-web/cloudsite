package producthttp

import (
	"github.com/nathanxiangang-web/panta/internal/acquisition"
	"github.com/nathanxiangang-web/panta/internal/product"
	"net/http"
)

// SourcePreviewConfig is an operator-declared unavailable state, not a live
// health claim and not permission to construct an acquisition graph.
type SourcePreviewConfig struct {
	Label             string `json:"label"`
	UnavailableReason string `json:"unavailable_reason"`
}

func (c SourcePreviewConfig) Valid() bool {
	return ValidDisplayLabel(c.Label) && (c.UnavailableReason == "LOGIN_REQUIRED" || c.UnavailableReason == "NOT_CONFIGURED")
}
func WithCloudSourcePreview(config SourcePreviewConfig) WorkspaceOption {
	return func(o *workspaceOptions) { o.sourcePreview = &config }
}

// Same caller, fresh source only. No repository, quota, admission or runtime port
// is accepted by this pure preparation adapter.
func registerSourcePreview(h *Handler) {
	h.mux.Handle("POST /api/v1/cloud-downloads/prepare", h.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		input, err := decodeCloudDownload(w, r, true)
		if err != nil {
			admissionFailure(w, err)
			return
		}
		prepared, err := acquisition.PrepareSource(input.Source, acquisition.MagnetBTIHNameV1)
		input.Source = ""
		if err != nil {
			admissionFailure(w, product.ErrInvalid)
			return
		}
		respond(w, 200, struct {
			Execution string                   `json:"execution_source"`
			Policy    acquisition.SourcePolicy `json:"policy"`
			Changed   bool                     `json:"changed"`
			Removed   int                      `json:"removed_parameters"`
		}{prepared.Reference.Value, prepared.Policy, prepared.Changed, prepared.RemovedParameters})
	})))
}
