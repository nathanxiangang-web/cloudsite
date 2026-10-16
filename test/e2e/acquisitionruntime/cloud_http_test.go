package acquisitionruntime

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nathanxiangang-web/panta/internal/acquisition"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/integrations/indexcore"
	"github.com/nathanxiangang-web/panta/internal/product"
	"github.com/nathanxiangang-web/panta/internal/producthttp"
	"github.com/nathanxiangang-web/panta/internal/resourceview"
	"github.com/nathanxiangang-web/panta/internal/storage"
	"github.com/nathanxiangang-web/panta/internal/store/postgres"
)

// Real auth/HTTP/admission/operation/task/usage stores and PostgreSQL, followed
// by the same controlled provider+Hint+Q5/Q8 runtime as the domain E2E. The POST
// acknowledgement is deliberately discarded; reconciliation uses GET only.
func consumerCloudHTTP(t *testing.T, f fixtureState, queryURL string, admission *product.AdmissionService, intent product.AdmissionRequest, owner auth.Principal) (product.AdmissionDTO, func()) {
	t.Helper()
	accountsRepo, _ := postgres.NewAuthRepository(f.pool)
	accounts, _ := auth.NewService(accountsRepo)
	topology, _ := postgres.NewStorageRepository(f.pool)
	catalog, _ := postgres.NewCatalogRepository(f.pool)
	index, _ := indexcore.NewClient(queryURL)
	physical, _ := resourceview.NewPhysicalService(index, catalog)
	tasks, _ := postgres.NewProductReadRepository(f.pool)
	reads, err := product.NewReadService(tasks, topology, index, physical, catalog, []storage.BindingID{f.bindingID})
	if err != nil {
		t.Fatal(err)
	}
	operations, _ := postgres.NewProductAdmissionRepository(f.pool)
	server := httptest.NewUnstartedServer(nil)
	origin := "http://" + server.Listener.Addr().String()
	handler, err := producthttp.NewWorkspaceHandler(accounts, origin, reads, []producthttp.DisplayBinding{{ID: f.bindingID, Label: "Controlled fixture"}}, postgres.CurrentSchemaVersion, producthttp.WithCloudDownload(producthttp.CloudDownloadTarget{BindingID: f.bindingID, TargetPath: intent.TargetPath, Label: "Controlled download"}, admission, operations))
	if err != nil {
		t.Fatal(err)
	}
	server.Config.Handler = handler
	server.Start()
	t.Cleanup(server.Close)
	var cookie *http.Cookie
	request := func(method, path string, body any) (int, []byte) {
		t.Helper()
		var encoded []byte
		if body != nil {
			encoded, _ = json.Marshal(body)
		}
		r, _ := http.NewRequest(method, origin+path, bytes.NewReader(encoded))
		if method == "POST" {
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Origin", origin)
		}
		if cookie != nil {
			r.AddCookie(cookie)
		}
		response, err := server.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		raw, _ := io.ReadAll(response.Body)
		if cookies := response.Cookies(); len(cookies) > 0 {
			cookie = cookies[0]
		}
		return response.StatusCode, raw
	}
	if code, _ := request("POST", "/api/v1/cloud-downloads", map[string]string{"source": intent.Source, "operation_key": intent.OperationKey}); code != 401 {
		t.Fatal("anonymous admission")
	}
	if code, _ := request("POST", "/api/v1/auth/login", map[string]string{"login": "admitted-runtime", "password": "synthetic-password"}); code != 200 || cookie == nil {
		t.Fatal("login failed")
	}
	prepared, _ := acquisition.PrepareSource(intent.Source, acquisition.MagnetBTIHNameV1)
	code, raw := request("POST", "/api/v1/cloud-downloads/prepare", map[string]string{"source": intent.Source})
	var preview struct {
		Execution string `json:"execution_source"`
	}
	_ = json.Unmarshal(raw, &preview)
	if code != 200 || preview.Execution != prepared.Reference.Value {
		t.Fatal("preview contract", code)
	}
	var count int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM product_operations`).Scan(&count); err != nil || count != 0 {
		t.Fatal("preview reserved operation")
	}
	// One explicit POST. Ignore its response IDs to model an acknowledgement lost
	// after the committed operation; GET must recover those same durable IDs.
	if code, _ := request("POST", "/api/v1/cloud-downloads", map[string]string{"source": intent.Source, "operation_key": intent.OperationKey}); code != 201 {
		t.Fatal("consumer admission", code)
	}
	code, raw = request("GET", "/api/v1/cloud-downloads/operations/"+intent.OperationKey, nil)
	var created product.AdmissionDTO
	if code != 200 || json.Unmarshal(raw, &created) != nil || created.Status != acquisition.SubmissionReplay {
		t.Fatal("reconcile failed", code)
	}
	if strings.Contains(string(raw), "magnet:") || strings.Contains(string(raw), "source") {
		t.Fatal("stored source disclosed")
	}
	if current, err := operations.Operation(f.ctx, owner, intent.OperationKey); err != nil || current.ManifestID != created.ManifestID || current.JobID != created.JobID {
		t.Fatal("lookup changed identity")
	}
	return created, func() {
		code, raw := request("GET", "/api/v1/tasks", nil)
		if code != 200 || !bytes.Contains(raw, []byte("READY")) || !bytes.Contains(raw, []byte("SUCCEEDED")) || !bytes.Contains(raw, []byte("result_copy_id")) || bytes.Contains(raw, []byte("magnet:")) {
			t.Fatal("consumer completion missing or unsafe", code)
		}
		code, raw = request("GET", "/api/v1/usage", nil)
		var usage product.UsageDTO
		if code != 200 || json.Unmarshal(raw, &usage) != nil || usage.Used != 1 || usage.InFlight != 0 {
			t.Fatal("usage not converged")
		}
		for i := 0; i < 3; i++ {
			if code, _ := request("GET", "/api/v1/cloud-downloads/operations/"+intent.OperationKey, nil); code != 200 {
				t.Fatal("replay GET failed")
			}
		}
		if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM product_operations`).Scan(&count); err != nil || count != 1 {
			t.Fatal("reconcile charged again")
		}
		if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM jobs`).Scan(&count); err != nil || count != 1 {
			t.Fatal("reconcile recreated Job")
		}
		if code, _ := request("POST", "/api/v1/auth/logout", nil); code != 204 {
			t.Fatal("logout failed")
		}
		if code, _ := request("GET", "/api/v1/tasks", nil); code != 401 {
			t.Fatal("logout retained private access")
		}
	}
}
