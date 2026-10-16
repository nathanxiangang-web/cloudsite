package producthttp

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"reflect"
	"unicode/utf8"

	"github.com/nathanxiangang-web/panta/internal/acquisition"
	"github.com/nathanxiangang-web/panta/internal/product"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

// CloudDownloadTarget is operator-only configuration, never an input DTO.
type CloudDownloadTarget struct {
	BindingID  storage.BindingID `json:"binding_id"`
	TargetPath string            `json:"target_path"`
	Label      string            `json:"label"`
}

func (t CloudDownloadTarget) Valid() bool {
	path, err := acquisition.NormalizeTargetPath(t.TargetPath)
	return err == nil && path == t.TargetPath && product.ValidID(string(t.BindingID)) && ValidDisplayLabel(t.Label)
}
func WithCloudDownload(target CloudDownloadTarget, admissions product.Admissions, operations product.OperationReader) WorkspaceOption {
	return func(o *workspaceOptions) {
		o.cloud = &cloudDownloadOptions{target: target, admissions: admissions, operations: operations}
	}
}

type cloudDownloadOptions struct {
	target     CloudDownloadTarget
	admissions product.Admissions
	operations product.OperationReader
}

func nilCloudPort(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Func, reflect.Chan, reflect.Slice:
		return v.IsNil()
	}
	return false
}
func registerCloudDownload(h *Handler, options *cloudDownloadOptions) error {
	if options == nil || !options.target.Valid() || nilCloudPort(options.admissions) || nilCloudPort(options.operations) {
		return product.ErrInvalid
	}
	h.mux.Handle("POST /api/v1/cloud-downloads/prepare", h.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		input, err := decodeCloudDownload(w, r, true)
		if err != nil {
			admissionFailure(w, err)
			return
		}
		prepared, err := acquisition.PrepareSource(input.Source, acquisition.MagnetBTIHNameV1)
		if err != nil {
			admissionFailure(w, product.ErrInvalid)
			return
		}
		// Explicit same-caller preview of freshly submitted input only. Never loads
		// stored sources, contacts115, creates a Manifest or reserves quota.
		respond(w, 200, struct {
			Execution string                   `json:"execution_source"`
			Policy    acquisition.SourcePolicy `json:"policy"`
			Changed   bool                     `json:"changed"`
			Removed   int                      `json:"removed_parameters"`
		}{prepared.Reference.Value, prepared.Policy, prepared.Changed, prepared.RemovedParameters})
	})))
	h.mux.Handle("POST /api/v1/cloud-downloads", h.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		input, err := decodeCloudDownload(w, r, false)
		if err != nil {
			admissionFailure(w, err)
			return
		}
		input.BindingID = options.target.BindingID
		input.TargetPath = options.target.TargetPath
		p, _ := Principal(r.Context())
		result, err := options.admissions.Admit(r.Context(), p, input)
		input.Source = ""
		input.ExpectedName = nil
		if err != nil {
			admissionFailure(w, err)
			return
		}
		status := http.StatusOK
		switch result.Status {
		case acquisition.SubmissionCreated:
			status = 201
		case acquisition.SubmissionPendingActivation:
			status = 202
		case acquisition.SubmissionReplay:
		default:
			admissionFailure(w, product.ErrAdmissionUncertain)
			return
		}
		if !product.ValidID(string(result.ManifestID)) || !product.ValidID(string(result.JobID)) {
			admissionFailure(w, product.ErrAdmissionUncertain)
			return
		}
		respond(w, status, result)
	})))
	h.mux.Handle("GET /api/v1/cloud-downloads/operations/{operation_key}", h.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := strictQuery(r); err != nil {
			admissionFailure(w, err)
			return
		}
		key := r.PathValue("operation_key")
		if !product.ValidOperationKey(key) {
			admissionFailure(w, product.ErrInvalid)
			return
		}
		p, _ := Principal(r.Context())
		result, err := options.operations.Operation(r.Context(), p, key)
		if err != nil {
			if errors.Is(err, product.ErrNotFound) {
				failure(w, 404, "OPERATION_NOT_FOUND")
			} else {
				admissionFailure(w, err)
			}
			return
		}
		if !product.ValidID(string(result.ManifestID)) || !product.ValidID(string(result.JobID)) || (result.Status != acquisition.SubmissionReplay && result.Status != acquisition.SubmissionPendingActivation) {
			admissionFailure(w, product.ErrAdmissionUncertain)
			return
		}
		respond(w, 200, result)
	})))
	h.mux.Handle("GET /api/v1/usage", h.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := strictQuery(r); err != nil {
			admissionFailure(w, err)
			return
		}
		p, _ := Principal(r.Context())
		result, err := options.admissions.Usage(r.Context(), p)
		if err != nil {
			admissionFailure(w, err)
			return
		}
		respond(w, 200, result)
	})))
	return nil
}
func decodeCloudDownload(w http.ResponseWriter, r *http.Request, preview bool) (product.AdmissionRequest, error) {
	if _, err := strictQuery(r); err != nil {
		return product.AdmissionRequest{}, product.ErrInvalid
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || r.Body == nil {
		return product.AdmissionRequest{}, product.ErrInvalid
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 512<<10))
	defer clear(raw)
	if err != nil || !utf8.Valid(raw) {
		return product.AdmissionRequest{}, product.ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return product.AdmissionRequest{}, product.ErrInvalid
	}
	input := product.AdmissionRequest{}
	seen := map[string]bool{}
	for decoder.More() {
		key, err := decoder.Token()
		name, ok := key.(string)
		if err != nil || !ok || seen[name] {
			return product.AdmissionRequest{}, product.ErrInvalid
		}
		seen[name] = true
		var encoded json.RawMessage
		if decoder.Decode(&encoded) != nil || bytes.Equal(bytes.TrimSpace(encoded), []byte("null")) {
			return product.AdmissionRequest{}, product.ErrInvalid
		}
		var value string
		if json.Unmarshal(encoded, &value) != nil {
			return product.AdmissionRequest{}, product.ErrInvalid
		}
		switch name {
		case "source":
			input.Source = value
		case "operation_key":
			if preview {
				return product.AdmissionRequest{}, product.ErrInvalid
			}
			input.OperationKey = value
		case "expected_name":
			if preview {
				return product.AdmissionRequest{}, product.ErrInvalid
			}
			input.ExpectedName = &value
		default:
			return product.AdmissionRequest{}, product.ErrInvalid
		}
	}
	token, err = decoder.Token()
	if err != nil || token != json.Delim('}') || decoder.Decode(new(any)) != io.EOF || !seen["source"] || !preview && !seen["operation_key"] || input.Source == "" || len(input.Source) > acquisition.MaxSourceInputLength {
		return product.AdmissionRequest{}, product.ErrInvalid
	}
	return input, nil
}
