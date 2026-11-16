package producthttp

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"

	"github.com/nathanxiangang-web/panta/internal/acquisition"
	"github.com/nathanxiangang-web/panta/internal/product"
)

type cloudSpaceOptions struct {
	service    *product.CloudSpaceService
	target     product.CloudSpaceTarget
	admissions product.Admissions
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
