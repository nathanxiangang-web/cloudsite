// Package productserver composes explicit product actions. The readonly action
// never constructs an acquisition graph; the separate cloud action opts in.
package productserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/platform/bootstrap"
	"github.com/nathanxiangang-web/panta/internal/product"
	"github.com/nathanxiangang-web/panta/internal/producthttp"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

const ServeAction = "serve-product-readonly"
const ProvisionAction = "provision-product-user"

var ErrConfig = errors.New("invalid protected product configuration")
var ErrStartup = errors.New("product process unavailable")

type Config struct {
	AdminProbeBootstrapFile string                           `json:"admin_probe_bootstrap_file,omitempty"`
	CloudDownloadPreview    *producthttp.SourcePreviewConfig `json:"cloud_download_preview,omitempty"`
	CloudDownload           *CloudDownloadConfig             `json:"cloud_download,omitempty"`
	RegistrationEnabled     bool                             `json:"registration_enabled,omitempty"`
	HTTPSProxy              bool                             `json:"https_proxy,omitempty"`
	LibraryBindingIDs       []storage.BindingID              `json:"library_binding_ids,omitempty"`
	WholeRootLibrary        bool                             `json:"whole_root_library,omitempty"`
	SharingEnabled          bool                             `json:"sharing_enabled,omitempty"`
	FileAccessConfigFile    string                           `json:"file_access_config_file,omitempty"`
	DatabaseURLFile         string                           `json:"database_url_file"`
	ListenAddress           string                           `json:"listen_address"`
	Origin                  string                           `json:"origin"`
	IndexCoreBaseURL        string                           `json:"indexcore_base_url"`
	Bindings                []producthttp.DisplayBinding     `json:"bindings"`
}
type provisionRequest struct {
	Login    string    `json:"login"`
	Password string    `json:"password"`
	Role     auth.Role `json:"role"`
}

func load(ctx context.Context, path string, destination any, limit int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	raw, err := bootstrap.ReadProtectedFile(path, limit)
	if err != nil {
		return ErrConfig
	}
	defer clear(raw)
	if !utf8.Valid(raw) {
		return ErrConfig
	}
	// Reject duplicate object members at all nesting levels, not just unknown
	// struct fields. Protected input cannot silently overwrite role/config.
	tokens := json.NewDecoder(bytes.NewReader(raw))
	if err = checkObject(tokens, 0); err != nil {
		return ErrConfig
	}
	if _, err = tokens.Token(); err != io.EOF {
		return ErrConfig
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(destination) != nil || decoder.Decode(new(any)) != io.EOF {
		return ErrConfig
	}
	return ctx.Err()
}
func checkObject(decoder *json.Decoder, depth int) error {
	if depth > 8 {
		return ErrConfig
	}
	token, err := decoder.Token()
	if err != nil {
		return ErrConfig
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]bool{}
		for decoder.More() {
			key, err := decoder.Token()
			name, ok := key.(string)
			if err != nil || !ok || seen[name] {
				return ErrConfig
			}
			seen[name] = true
			if err = checkObject(decoder, depth+1); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim('}') {
			return ErrConfig
		}
	case '[':
		for decoder.More() {
			if err = checkObject(decoder, depth+1); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim(']') {
			return ErrConfig
		}
	default:
		return ErrConfig
	}
	return nil
}
func LoadConfig(ctx context.Context, path string) (Config, error) {
	var cfg Config
	if err := load(ctx, path, &cfg, 32<<10); err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
func (cfg Config) Validate() error {
	if cfg.CloudDownloadPreview != nil && (!cfg.CloudDownloadPreview.Valid() || cfg.CloudDownload != nil) {
		return ErrConfig
	}
	if cfg.CloudDownload != nil && !cfg.CloudDownload.Valid() {
		return ErrConfig
	}
	host, port, err := net.SplitHostPort(cfg.ListenAddress)
	ip := net.ParseIP(host)
	p, portErr := strconv.Atoi(port)
	if err != nil || portErr != nil || p < 1 || p > 65535 || ip == nil || !ip.IsLoopback() {
		return ErrConfig
	}
	origin, err := url.Parse(cfg.Origin)
	if err != nil || origin.Host == "" || origin.Path != "" || origin.User != nil || origin.RawQuery != "" || origin.Fragment != "" || origin.Opaque != "" {
		return ErrConfig
	}
	if cfg.HTTPSProxy {
		if origin.Scheme != "https" || origin.Hostname() == "" {
			return ErrConfig
		}
	} else if origin.Scheme != "http" || origin.Host != cfg.ListenAddress {
		return ErrConfig
	}
	endpoint, err := url.Parse(cfg.IndexCoreBaseURL)
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" || endpoint.Opaque != "" {
		return ErrConfig
	}
	if strings.TrimSpace(cfg.DatabaseURLFile) != cfg.DatabaseURLFile || cfg.DatabaseURLFile == "" || len(cfg.Bindings) < 1 || len(cfg.Bindings) > 64 {
		return ErrConfig
	}
	seen := map[string]bool{}
	for _, binding := range cfg.Bindings {
		if !product.ValidID(string(binding.ID)) || seen[string(binding.ID)] || !producthttp.ValidDisplayLabel(binding.Label) {
			return ErrConfig
		}
		seen[string(binding.ID)] = true
	}
	if cfg.CloudDownload != nil && !seen[string(cfg.CloudDownload.Target.BindingID)] {
		return ErrConfig
	}
	return nil
}
func loadProvision(ctx context.Context, path string) (provisionRequest, error) {
	var input provisionRequest
	if err := load(ctx, path, &input, 2048); err != nil {
		return provisionRequest{}, err
	}
	if _, err := auth.NormalizeLogin(input.Login); err != nil || !auth.ValidPassword(input.Password) || !input.Role.Valid() {
		return provisionRequest{}, ErrConfig
	}
	return input, nil
}
