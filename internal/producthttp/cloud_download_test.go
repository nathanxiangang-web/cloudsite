package producthttp

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/nathanxiangang-web/panta/internal/acquisition"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/product"
	"strings"
	"testing"
)

type operationFake struct {
	calls int
	owner auth.Principal
	key   string
	err   error
}

func (f *operationFake) Operation(_ context.Context, p auth.Principal, key string) (product.AdmissionDTO, error) {
	f.calls++
	f.owner = p
	f.key = key
	return product.AdmissionDTO{ManifestID: "98000000-0000-4000-8000-000000000003", JobID: "98000000-0000-4000-8000-000000000004", Status: acquisition.SubmissionReplay}, f.err
}
func cloudTarget() CloudDownloadTarget {
	return CloudDownloadTarget{BindingID: workspaceBinding.ID, TargetPath: "/operator-target", Label: "网盘下载目录"}
}
func TestCloudDownloadExplicitCapabilityAndPrivateFixedTarget(t *testing.T) {
	access := &fakeAccess{p: goodPrincipal()}
	backend := &admissionFake{}
	operations := &operationFake{}
	readonly, err := NewWorkspaceHandler(access, "http://127.0.0.1:8180", &readAccessFake{}, []DisplayBinding{workspaceBinding}, 15)
	if err != nil {
		t.Fatal(err)
	}
	body := `{"operation_key":"web-once","source":"https://example.invalid/PRIVATE?sig=exact"}`
	if w := request(readonly, "POST", "/api/v1/cloud-downloads", body, testToken, "http://127.0.0.1:8180"); w.Code != 404 {
		t.Fatal("readonly silently enabled admission")
	}
	writable, err := NewWorkspaceHandler(access, "http://127.0.0.1:8180", &readAccessFake{}, []DisplayBinding{workspaceBinding}, 15, WithCloudDownload(cloudTarget(), backend, operations))
	if err != nil {
		t.Fatal(err)
	}
	info := request(writable, "GET", "/api/v1/workspace", "", testToken, "")
	if info.Code != 200 || !strings.Contains(info.Body.String(), `"cloud_download_enabled":true`) || strings.Contains(info.Body.String(), "operator-target") {
		t.Fatal("capability or private target")
	}
	submitted := request(writable, "POST", "/api/v1/cloud-downloads", body, testToken, "http://127.0.0.1:8180")
	if submitted.Code != 201 || backend.calls != 1 || backend.owner != access.p || backend.input.BindingID != workspaceBinding.ID || backend.input.TargetPath != "/operator-target" || backend.input.Source != "https://example.invalid/PRIVATE?sig=exact" {
		t.Fatal("fixed target/identity/source", submitted.Code)
	}
	if strings.Contains(submitted.Body.String(), "PRIVATE") || strings.Contains(submitted.Body.String(), "source") {
		t.Fatal("source leak")
	}
	for _, asset := range []string{"/downloads/", "/assets/downloads.js", "/assets/download-model.js", "/assets/downloads.css"} {
		if w := request(writable, "GET", asset, "", "", ""); w.Code != 200 {
			t.Fatal(asset, w.Code)
		}
	}
	if w := request(writable, "POST", "/api/v1/acquisitions", body, testToken, "http://127.0.0.1:8180"); w.Code != 404 {
		t.Fatal("operator admission route exposed")
	}
}
func TestCloudDownloadPreviewIsOwnInputOnlyAndNeverAdmits(t *testing.T) {
	backend := &admissionFake{}
	operations := &operationFake{}
	h, _ := NewWorkspaceHandler(&fakeAccess{p: goodPrincipal()}, "http://127.0.0.1:8180", &readAccessFake{}, []DisplayBinding{workspaceBinding}, 15, WithCloudDownload(cloudTarget(), backend, operations))
	source := "magnet:?xt=urn:btih:0123456789ABCDEF0123456789ABCDEF01234567&dn=demo&tr=drop"
	data, _ := json.Marshal(map[string]string{"source": source})
	w := request(h, "POST", "/api/v1/cloud-downloads/prepare", string(data), testToken, "http://127.0.0.1:8180")
	var preview struct {
		Execution string `json:"execution_source"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &preview)
	if w.Code != 200 || preview.Execution != "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567&dn=demo" || backend.calls != 0 || operations.calls != 0 {
		t.Fatal("preview reserved or wrong policy")
	}
	for _, tc := range []struct {
		raw, token, origin string
		code               int
	}{
		{string(data), "", "http://127.0.0.1:8180", 401}, {string(data), testToken, "https://evil.invalid", 403},
		{`{"source":"magnet:?xt=urn:btih:bad"}`, testToken, "http://127.0.0.1:8180", 400},
		{`{"source":"https://a.invalid","operation_key":"x"}`, testToken, "http://127.0.0.1:8180", 400},
		{`{"source":"https://a.invalid","source":"https://b.invalid"}`, testToken, "http://127.0.0.1:8180", 400},
	} {
		if got := request(h, "POST", "/api/v1/cloud-downloads/prepare", tc.raw, tc.token, tc.origin); got.Code != tc.code {
			t.Fatal(got.Code, tc.code)
		}
	}
	if backend.calls != 0 || operations.calls != 0 {
		t.Fatal("preview mutation")
	}
}
func TestCloudDownloadStrictSubmissionAndSourceFreeLookup(t *testing.T) {
	backend := &admissionFake{}
	operations := &operationFake{}
	access := &fakeAccess{p: goodPrincipal()}
	h, _ := NewWorkspaceHandler(access, "http://127.0.0.1:8180", &readAccessFake{}, []DisplayBinding{workspaceBinding}, 15, WithCloudDownload(cloudTarget(), backend, operations))
	valid := `{"operation_key":"x","source":"https://example.invalid/PRIVATE"}`
	for _, raw := range []string{`{}`, `null`, valid + `{}`, strings.Repeat(" ", 512<<10) + valid, strings.Replace(valid, `"source":"https://example.invalid/PRIVATE"`, `"source":null`, 1), strings.Replace(valid, `"operation_key":"x"`, `"operation_key":2`, 1)} {
		if w := request(h, "POST", "/api/v1/cloud-downloads", raw, testToken, "http://127.0.0.1:8180"); w.Code != 400 {
			t.Fatal("invalid JSON", w.Code)
		}
	}
	for _, field := range []string{"binding_id", "target_path", "provider_scope", "owner", "user_id", "role", "source_policy", "max_attempts", "limit", "cookie"} {
		raw := strings.TrimSuffix(valid, "}") + `,"` + field + `":"forbidden"}`
		if w := request(h, "POST", "/api/v1/cloud-downloads", raw, testToken, "http://127.0.0.1:8180"); w.Code != 400 {
			t.Fatal("client target/policy injection", field)
		}
	}
	if backend.calls != 0 {
		t.Fatal("invalid input admitted")
	}
	for _, tc := range []struct {
		path, token string
		code        int
	}{{"/api/v1/cloud-downloads/operations/web-once", testToken, 200}, {"/api/v1/cloud-downloads/operations/web-once", "", 401}, {"/api/v1/cloud-downloads/operations/web-once?raw=1", testToken, 400}, {"/api/v1/cloud-downloads/operations/!invalid", testToken, 400}} {
		w := request(h, "GET", tc.path, "", tc.token, "")
		if w.Code != tc.code || strings.Contains(w.Body.String(), "source") {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if operations.calls != 1 || operations.owner != access.p || operations.key != "web-once" || backend.calls != 0 {
		t.Fatal("lookup mutated/adopted")
	}
	operations.err = product.ErrNotFound
	if w := request(h, "GET", "/api/v1/cloud-downloads/operations/web-once", "", testToken, ""); w.Code != 404 || !strings.Contains(w.Body.String(), "OPERATION_NOT_FOUND") {
		t.Fatal("missing operation classification")
	}
	operations.err = errors.New("PRIVATE cookie")
	if w := request(h, "GET", "/api/v1/cloud-downloads/operations/web-once", "", testToken, ""); w.Code != 503 || strings.Contains(w.Body.String(), "PRIVATE") {
		t.Fatal("unsafe lookup failure")
	}
	var nilOperations *operationFake
	if _, err := NewWorkspaceHandler(access, "http://127.0.0.1:8180", &readAccessFake{}, []DisplayBinding{workspaceBinding}, 15, WithCloudDownload(cloudTarget(), backend, nilOperations)); err == nil {
		t.Fatal("typed nil enabled cloud")
	}
	bad := cloudTarget()
	bad.TargetPath = "/a//b"
	if bad.Valid() {
		t.Fatal("ambiguous operator path")
	}
}
