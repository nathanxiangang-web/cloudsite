package fileaccess

import (
	"context"
	"net"
	"net/url"
	"strings"

	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/cloudpath"
	"github.com/nathanxiangang-web/panta/internal/product"
)

// LinkSource resolves a known file without opening its byte stream.
type LinkSource interface {
	ResolveLink(context.Context, SourceRequest) (string, error)
}

// EnableDirectLinks is composition-only. One required allowlist governs every
// public/private download, media playback and shared download.
func (s *Service) EnableDirectLinks(hosts []string) error {
	if err := s.validateLinkHosts(hosts); err != nil {
		return err
	}
	s.directHosts = append([]string(nil), hosts...)
	return nil
}

func (s *Service) validateLinkHosts(hosts []string) error {
	if len(hosts) == 0 || len(hosts) > 16 {
		return ErrUnavailable
	}
	seen := map[string]bool{}
	for _, host := range hosts {
		if host == "" || len(host) > 253 || host != strings.ToLower(host) || !strings.Contains(host, ".") || net.ParseIP(host) != nil || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".internal") || hasNumericFinalLabel(host) {
			return ErrUnavailable
		}
		for _, label := range strings.Split(host, ".") {
			if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
				return ErrUnavailable
			}
			for _, c := range label {
				if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
					return ErrUnavailable
				}
			}
		}
		if seen[host] {
			return ErrUnavailable
		}
		seen[host] = true
	}
	if _, ok := s.source.(LinkSource); !ok {
		return ErrUnavailable
	}
	return nil
}

// Browsers interpret hostnames ending in an IPv4 number differently from
// net.ParseIP, including abbreviated, octal and hexadecimal IP spellings.
func hasNumericFinalLabel(host string) bool {
	label := host[strings.LastIndexByte(host, '.')+1:]
	if label == "" {
		return false
	}
	if strings.HasPrefix(label, "0x") {
		return strings.Trim(label[2:], "0123456789abcdef") == ""
	}
	return strings.Trim(label, "0123456789") == ""
}

// MediaSources exposes only the exact allowlisted HTTPS hosts to browser media CSP.
func (s *Service) MediaSources() []string {
	result := make([]string, 0, len(s.directHosts))
	for _, host := range s.directHosts {
		result = append(result, "https://"+host)
	}
	return result
}

// Deliver always authorizes the canonical file and resolves an upstream URL.
// It never opens, streams, proxies or buffers file bytes. PreviewSource is
// reachable only through PreviewService.
func (s *Service) Deliver(ctx context.Context, p auth.Principal, r Request) (File, error) {
	if r.Shared && cloudpath.IsPrivate(r.Path) {
		return File{}, product.ErrNotFound
	}
	file, source, err := s.describe(ctx, p, r)
	if err != nil {
		return file, err
	}
	if _, _, _, err = ByteRange(r.Range, file.Size); err != nil {
		return File{Size: file.Size}, err
	}
	if len(r.UserAgent) > 2048 || strings.ContainsAny(r.UserAgent, "\x00\r\n") {
		return File{}, ErrUnavailable
	}
	if len(s.directHosts) == 0 {
		return File{}, ErrUnavailable
	}
	linkSource, ok := s.source.(LinkSource)
	if !ok {
		return File{}, ErrUnavailable
	}
	source.UserAgent = r.UserAgent
	target, err := linkSource.ResolveLink(ctx, source)
	if err != nil {
		return File{}, ErrUnavailable
	}
	u, err := url.Parse(target)
	if err != nil || len(target) > 16384 || u.Scheme != "https" || u.User != nil || u.Opaque != "" || u.Host == "" || strings.ContainsAny(target, "#\x00\r\n") {
		return File{}, ErrUnavailable
	}
	allowed := false
	for _, host := range s.directHosts {
		if u.Host == host || u.Host == host+":443" {
			allowed = true
			break
		}
	}
	if !allowed {
		return File{}, ErrUnavailable
	}
	file.DirectURL = u.String()
	return file, nil
}
