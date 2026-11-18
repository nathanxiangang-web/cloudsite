package producthttp

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"strconv"

	"github.com/nathanxiangang-web/panta/internal/acquisition"
	"github.com/nathanxiangang-web/panta/internal/product"
)

type PrivateFileReader = product.PrivateFileReader

type cloudSpaceOptions struct {
	service    *product.CloudSpaceService
	target     product.CloudSpaceTarget
	admissions product.Admissions
	files      PrivateFileReader
}

func WithCloudSpace(service *product.CloudSpaceService, target product.CloudSpaceTarget, admissions ...product.Admissions) WorkspaceOption {
	return func(options *workspaceOptions) {
		cloudSpace := &cloudSpaceOptions{service: service, target: target}
		if len(admissions) > 0 {
			cloudSpace.admissions = admissions[0]
		}
		options.cloudSpace = cloudSpace
	}
}

func WithCloudSpaceFiles(reader PrivateFileReader) WorkspaceOption {
	return func(options *workspaceOptions) {
		if options.cloudSpace == nil {
			options.cloudSpace = &cloudSpaceOptions{}
		}
		options.cloudSpace.files = reader
	}
}

func CombineWorkspaceOptions(options ...WorkspaceOption) WorkspaceOption {
	return func(settings *workspaceOptions) {
		for _, option := range options {
			option(settings)
		}
	}
}

func registerCloudSpace(h *Handler, options *cloudSpaceOptions) error {
	if options == nil || options.service == nil || !options.target.Valid() {
		return product.ErrInvalid
	}
	h.mux.Handle("GET /api/v2/cloud-downloads/space", h.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := strictQuery(r); err != nil {
			admissionFailure(w, product.ErrInvalid)
			return
		}
		principal, _ := Principal(r.Context())
		result, err := options.service.GetSpace(r.Context(), principal, options.target)
		if err != nil {
			admissionFailure(w, err)
			return
		}
		respond(w, http.StatusOK, result)
	})))
	if options.files != nil {
		h.mux.Handle("GET /api/v2/cloud-downloads/files", h.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cursor, limit, err := parsePrivateFilesQuery(r)
			if err != nil {
				admissionFailure(w, err)
				return
			}
			principal, _ := Principal(r.Context())
			page, err := options.files.PrivateFiles(r.Context(), principal, options.target.BindingID, cursor, limit)
			if err != nil {
				admissionFailure(w, err)
				return
			}
			respond(w, http.StatusOK, page)
		})))
	}
	h.mux.Handle("POST /api/v2/cloud-downloads/space/provision", h.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key, err := decodeCloudSpaceProvision(w, r)
		if err != nil {
			admissionFailure(w, err)
			return
		}
		principal, _ := Principal(r.Context())
		result, err := options.service.Provision(r.Context(), principal, options.target, key)
		if err != nil {
			admissionFailure(w, err)
			return
		}
		status := http.StatusAccepted
		if result.Status == product.UserSpaceActive {
			status = http.StatusOK
		}
		respond(w, status, result)
	})))
	if options.admissions != nil {
		h.mux.Handle("POST /api/v2/cloud-downloads", h.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			input, err := decodeCloudDownload(w, r, false)
			if err != nil {
				admissionFailure(w, err)
				return
			}
			principal, _ := Principal(r.Context())
			space, err := options.service.ActiveSpace(r.Context(), principal, options.target)
			if err != nil {
				admissionFailure(w, err)
				return
			}
			spaceID := string(space.ID)
			input.BindingID = options.target.BindingID
			input.TargetPath = space.CanonicalPrefix
			input.UserSpaceID = &spaceID
			input.TargetProviderConnectionID = &space.ProviderConnectionID
			input.TargetProviderScope = space.NativeDirID
			result, err := options.admissions.Admit(r.Context(), principal, input)
			input.Source = ""
			input.ExpectedName = nil
			if err != nil {
				admissionFailure(w, err)
				return
			}
			status := http.StatusOK
			switch result.Status {
			case acquisition.SubmissionCreated:
				status = http.StatusCreated
			case acquisition.SubmissionPendingActivation:
				status = http.StatusAccepted
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
	}
	return nil
}

func parsePrivateFilesQuery(r *http.Request) (string, int, error) {
	query, err := strictQuery(r, "cursor", "limit")
	if err != nil {
		return "", 0, product.ErrInvalid
	}
	cursor := query.Get("cursor")
	if len(cursor) > 2048 {
		return "", 0, product.ErrInvalid
	}
	limit := 50
	if query.Has("limit") {
		value, err := strconv.Atoi(query.Get("limit"))
		if err != nil {
			return "", 0, product.ErrInvalid
		}
		limit = value
	}
	if limit < 1 || limit > 50 {
		return "", 0, product.ErrInvalid
	}
	return cursor, limit, nil
}

func decodeCloudSpaceProvision(w http.ResponseWriter, r *http.Request) (string, error) {
	if _, err := strictQuery(r); err != nil {
		return "", product.ErrInvalid
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || r.Body == nil {
		return "", product.ErrInvalid
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	var input struct {
		OperationKey string `json:"operation_key"`
	}
	if err := decoder.Decode(&input); err != nil {
		return "", product.ErrInvalid
	}
	if err := decoder.Decode(new(any)); err != io.EOF || !product.ValidOperationKey(input.OperationKey) {
		return "", product.ErrInvalid
	}
	return input.OperationKey, nil
}
