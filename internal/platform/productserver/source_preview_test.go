package productserver

import (
	"github.com/nathanxiangang-web/panta/internal/producthttp"
	"testing"
)

func TestReadOnlySourcePreviewConfiguration(t *testing.T) {
	cfg := validConfig()
	cfg.CloudDownloadPreview = &producthttp.SourcePreviewConfig{Label: "Cloud downloads", UnavailableReason: "LOGIN_REQUIRED"}
	if cfg.Validate() != nil {
		t.Fatal("readonly preview rejected")
	}
	cfg.CloudDownloadPreview.UnavailableReason = "READY"
	if cfg.Validate() == nil {
		t.Fatal("unbounded health declaration")
	}
	cfg.CloudDownloadPreview.UnavailableReason = "LOGIN_REQUIRED"
	cfg.CloudDownload = &CloudDownloadConfig{Target: producthttp.CloudDownloadTarget{BindingID: cfg.Bindings[0].ID, TargetPath: "/", Label: "Cloud"}, BootstrapFile: "/protected/bootstrap.json"}
	if cfg.Validate() == nil {
		t.Fatal("readonly declaration mixed with graph")
	}
}
