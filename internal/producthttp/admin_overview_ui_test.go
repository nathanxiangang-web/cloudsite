package producthttp

import (
	"strings"
	"testing"
)

// The shell/asset is public static code, never a data fallback or command port.
func TestAdminOverviewUIAssetAndConsumerIsolation(t *testing.T) {
	h, err := NewWorkspaceHandler(&fakeAccess{p: goodPrincipal()}, "http://127.0.0.1:8180", &readAccessFake{}, []DisplayBinding{workspaceBinding}, 18)
	if err != nil {
		t.Fatal(err)
	}
	asset := request(h, "GET", "/assets/admin-overview.js", "", "", "")
	if asset.Code != 200 || !strings.Contains(asset.Header().Get("Content-Type"), "text/javascript") || asset.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("overview asset not served safely", asset.Code, asset.Header())
	}
	for _, want := range []string{"/api/v1/admin/overview", "CLOUDSITE_POSTGRES_MANIFESTS_AND_JOBS", "PRODUCT_ACQUISITIONS_IN_CONFIGURED_BINDINGS"} {
		if !strings.Contains(asset.Body.String(), want) {
			t.Fatal("missing read contract", want)
		}
	}
	shell := request(h, "GET", "/admin/", "", "", "")
	for _, want := range []string{"/assets/admin-overview.js", "overview-manifests-total", "overview-jobs-total", "overview-users-total", "overview-read-at"} {
		if !strings.Contains(shell.Body.String(), want) {
			t.Fatal("homepage not wired", want)
		}
	}
	front := request(h, "GET", "/", "", "", "")
	if strings.Contains(front.Body.String(), "/assets/admin-overview.js") || strings.Contains(front.Body.String(), "管理后台") {
		t.Fatal("consumer/admin navigation isolation regressed")
	}
}
