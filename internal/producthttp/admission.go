package producthttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"reflect"
	"unicode/utf8"

	"github.com/nathanxiangang-web/panta/internal/acquisition"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/product"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

// NewAdmissionHandler is an explicit writable adapter. It does not change the
// accepted readonly router, open a listener or start an acquisition Worker.
func NewAdmissionHandler(access auth.Access, origin string, admissions product.Admissions) (*Handler, error) {
	if admissions == nil || reflect.ValueOf(admissions).Kind() == reflect.Pointer && reflect.ValueOf(admissions).IsNil() {
		return nil, product.ErrUnavailable
	}
	h, err := NewAuthHandler(access, origin)
	if err != nil {
		return nil, err
	}
	h.mux.Handle("POST /api/v1/acquisitions", h.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := strictQuery(r); err != nil {
			admissionFailure(w, product.ErrInvalid)
			return
		}
		input, err := decodeAdmission(w, r)
		if err != nil {
			admissionFailure(w, err)
			return
		}
		p, _ := Principal(r.Context())
		result, err := admissions.Admit(r.Context(), p, input)
		// No raw body or original error is logged or returned.
		input.Source = ""
		input.ExpectedName = nil
		if err != nil {
			admissionFailure(w, err)
			return
		}
		status := 200
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
	h.mux.Handle("GET /api/v1/usage", h.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := strictQuery(r); err != nil {
			admissionFailure(w, product.ErrInvalid)
			return
		}
		p, _ := Principal(r.Context())
		result, err := admissions.Usage(r.Context(), p)
		if err != nil {
			admissionFailure(w, err)
			return
		}
		respond(w, 200, result)
	})))
	return h, nil
}
func decodeAdmission(w http.ResponseWriter, r *http.Request) (product.AdmissionRequest, error) {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || r.Body == nil {
		return product.AdmissionRequest{}, product.ErrInvalid
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 512<<10))
	defer clear(raw)
	if err != nil || !utf8.Valid(raw) {
		return product.AdmissionRequest{}, product.ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return product.AdmissionRequest{}, product.ErrInvalid
	}
	input := product.AdmissionRequest{}
	seen := map[string]bool{}
	for d.More() {
		key, err := d.Token()
		name, ok := key.(string)
		if err != nil || !ok || seen[name] {
			return product.AdmissionRequest{}, product.ErrInvalid
		}
		seen[name] = true
		var value json.RawMessage
		if err = d.Decode(&value); err != nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return product.AdmissionRequest{}, product.ErrInvalid
		}
		var text string
		if json.Unmarshal(value, &text) != nil {
			return product.AdmissionRequest{}, product.ErrInvalid
		}
		switch name {
		case "operation_key":
			input.OperationKey = text
		case "source":
			input.Source = text
		case "binding_id":
			input.BindingID = storage.BindingID(text)
		case "target_path":
			input.TargetPath = text
		case "expected_name":
			input.ExpectedName = &text
		default:
			return product.AdmissionRequest{}, product.ErrInvalid
		}
	}
	token, err = d.Token()
	if err != nil || token != json.Delim('}') || d.Decode(new(any)) != io.EOF || !seen["operation_key"] || !seen["source"] || !seen["binding_id"] || !seen["target_path"] {
		return product.AdmissionRequest{}, product.ErrInvalid
	}
	if err = product.ValidateAdmissionRequest(input); err != nil {
		return product.AdmissionRequest{}, err
	}
	return input, nil
}
func admissionFailure(w http.ResponseWriter, err error) {
	status, code := 503, "ADMISSION_UNAVAILABLE"
	switch {
	case errors.Is(err, auth.ErrUnauthorized):
		status, code = 401, "AUTH_REQUIRED"
	case errors.Is(err, product.ErrInvalid):
		status, code = 400, "INVALID_REQUEST"
	case errors.Is(err, product.ErrNotFound):
		status, code = 404, "BINDING_NOT_AVAILABLE"
	case errors.Is(err, product.ErrOperationConflict):
		status, code = 409, "OPERATION_CONFLICT"
	case errors.Is(err, product.ErrOperationLimit):
		status, code = 429, "OPERATION_LIMIT"
	case errors.Is(err, product.ErrConcurrencyLimit):
		status, code = 429, "CONCURRENCY_LIMIT"
	case errors.Is(err, product.ErrAdmissionUncertain):
		code = "ADMISSION_UNCERTAIN"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		code = "ADMISSION_UNAVAILABLE"
	}
	failure(w, status, code)
}
