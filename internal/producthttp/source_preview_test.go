package producthttp

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSourcePreviewCutsAMPTail(t *testing.T) {
	h, err := NewWorkspaceHandler(&fakeAccess{p: goodPrincipal()}, "http://127.0.0.1:8180", &readAccessFake{}, []DisplayBinding{workspaceBinding}, 15, WithCloudSourcePreview(SourcePreviewConfig{Label: "Cloud downloads", UnavailableReason: "LOGIN_REQUIRED"}))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]string{"source": "magnet:?xt=urn:btih:0123456789ABCDEF0123456789ABCDEF01234567&amp;dn=discarded&amp;tr=udp://tracker.invalid"})
	w := request(h, "POST", "/api/v1/cloud-downloads/prepare", string(raw), testToken, "http://127.0.0.1:8180")
	var result struct {
		ExecutionSource string `json:"execution_source"`
		Policy          string `json:"policy"`
		Changed         bool   `json:"changed"`
		Removed         int    `json:"removed_parameters"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || result.ExecutionSource != "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567" || result.Policy != "magnet-btih-name-v1" || !result.Changed || result.Removed != 2 {
		t.Fatal("AMP preview differs from execution contract", w.Code)
	}
	if strings.Contains(w.Body.String(), "discarded") || strings.Contains(w.Body.String(), "tracker.invalid") {
		t.Fatal("discarded tail exposed in preview")
	}
}

func TestReadOnlySourcePreviewNeverRegistersAdmission(t *testing.T) {
	access := &fakeAccess{p: goodPrincipal()}
	h, err := NewWorkspaceHandler(access, "http://127.0.0.1:8180", &readAccessFake{}, []DisplayBinding{workspaceBinding}, 15, WithCloudSourcePreview(SourcePreviewConfig{Label: "Cloud downloads", UnavailableReason: "LOGIN_REQUIRED"}))
	if err != nil {
		t.Fatal(err)
	}
	raw := `{"source":"magnet:?xt=urn:btih:0123456789ABCDEF0123456789ABCDEF01234567&dn=example.iso&tr=https%3A%2F%2Ftracker.invalid"}`
	w := request(h, "POST", "/api/v1/cloud-downloads/prepare", raw, testToken, "http://127.0.0.1:8180")
	if w.Code != 200 || strings.Contains(w.Body.String(), "tracker.invalid") || !strings.Contains(w.Body.String(), "example.iso") {
		t.Fatal("pure preparation", w.Code)
	}
	if w = request(h, "POST", "/api/v1/cloud-downloads/prepare", raw, "", "http://127.0.0.1:8180"); w.Code != 401 {
		t.Fatal("anonymous preview")
	}
	if w = request(h, "POST", "/api/v1/cloud-downloads/prepare", raw, testToken, "https://other.invalid"); w.Code != 403 {
		t.Fatal("cross-origin preview")
	}
	for _, path := range []string{"/api/v1/cloud-downloads", "/api/v1/acquisitions"} {
		if w = request(h, "POST", path, raw, testToken, "http://127.0.0.1:8180"); w.Code != 404 {
			t.Fatal("admission registered", path, w.Code)
		}
	}
	for _, path := range []string{"/api/v1/usage", "/api/v1/cloud-downloads/operations/example"} {
		if w = request(h, "GET", path, "", testToken, ""); w.Code != 404 {
			t.Fatal("mutation service exposed", path, w.Code)
		}
	}
	w = request(h, "GET", "/api/v1/workspace", "", testToken, "")
	var info workspaceInfo
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &info) != nil || info.Mode != "READ_ONLY" || info.CloudDownloadEnabled || !info.CloudDownloadPreviewEnabled || info.CloudDownloadUnavailableReason != "LOGIN_REQUIRED" {
		t.Fatal("false runtime capability")
	}
	if strings.Contains(w.Body.String(), "source") || strings.Contains(w.Body.String(), "credential") {
		t.Fatal("source/config disclosure")
	}
	for _, reason := range []string{"", "READY", "private credential"} {
		if _, err = NewWorkspaceHandler(access, "http://127.0.0.1:8180", &readAccessFake{}, []DisplayBinding{workspaceBinding}, 15, WithCloudSourcePreview(SourcePreviewConfig{Label: "Cloud", UnavailableReason: reason})); err == nil {
			t.Fatal("open reason set")
		}
	}
	if _, err = NewWorkspaceHandler(access, "http://127.0.0.1:8180", &readAccessFake{}, []DisplayBinding{workspaceBinding}, 15, WithCloudSourcePreview(SourcePreviewConfig{Label: "Cloud", UnavailableReason: "NOT_CONFIGURED"}), WithCloudDownload(cloudTarget(), &admissionFake{}, &operationFake{})); err == nil {
		t.Fatal("mixed graph/readonly declaration")
	}
}
