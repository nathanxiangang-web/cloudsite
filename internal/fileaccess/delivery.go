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

// EnableDirectLinks is composition-only. An empty list preserves legacy proxy
// delivery. Exact HTTPS DNS hosts also define the browser media CSP sources.
func (s *Service) EnableDirectLinks(hosts []string) error {
	if err := s.validateLinkHosts(hosts); err != nil {
		return err
	}
	s.directHosts = append([]string(nil), hosts...)
	return nil
}

// EnableVideoDirectLinks restricts the playback opt-in to explicit inline video
// requests. Each redirected file still goes through the normal ownership check.
func (s *Service) EnableVideoDirectLinks(hosts []string) error {
	if err := s.validateLinkHosts(hosts); err != nil {
		return err
	}
	s.videoDirectHosts = append([]string(nil), hosts...)
	return nil
}

// EnableSharedDirectLinks opts redeemed guest downloads into exact-host HTTPS
// redirects after the sharing service has validated the current grant.
func (s *Service) EnableSharedDirectLinks(hosts []string) error {
	if err := s.validateLinkHosts(hosts); err != nil {
		return err
	}
	s.sharedDirectHosts = append([]string(nil), hosts...)
	return nil
}

func (s *Service) validateLinkHosts(hosts []string) error {
	if len(hosts) > 16 {
		return ErrUnavailable
	}
	seen := map[string]bool{}
	for _, host := range hosts {
		if host == "" || len(host) > 253 || host != strings.ToLower(host) || !strings.Contains(host, ".") || net.ParseIP(host) != nil || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".localhost") {
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
	if len(hosts) > 0 {
		if _, ok := s.source.(LinkSource); !ok {
			return ErrUnavailable
		}
	}
	return nil
}

func (s *Service) MediaSources() []string {
	result := make([]string, 0, len(s.directHosts)+len(s.videoDirectHosts))
	seen := make(map[string]bool, cap(result))
	for _, hosts := range [][]string{s.directHosts, s.videoDirectHosts} {
		for _, host := range hosts {
			if !seen[host] {
				result = append(result, "https://"+host)
				seen[host] = true
			}
		}
	}
	return result
}

func isVideo(ext string) bool {
	switch ext {
	case "mp4", "mkv", "webm", "mov", "avi", "m4v", "mpeg", "mpg", "wmv", "ts", "m2ts", "flv", "3gp", "ogv":
		return true
	default:
		return false
	}
}

// Deliver preserves canonical authorization for each redirect. Open remains the
// byte-only port used by the private document preview cache.
func (s *Service) Deliver(ctx context.Context, p auth.Principal, r Request) (File, error) {
	hosts := s.directHosts
	private := cloudpath.IsPrivate(r.Path)
	if r.Shared && private {
		return File{}, product.ErrNotFound
	}
	video := !r.Shared && len(s.videoDirectHosts) > 0 && r.Inline && isVideo(fileExtension(r.Path))
	if r.Shared && len(s.sharedDirectHosts) > 0 {
		hosts = s.sharedDirectHosts
	} else if video {
		hosts = s.videoDirectHosts
	} else if len(hosts) == 0 || private {
		return s.Open(ctx, p, r)
	}
	file, source, err := s.describe(ctx, p, r)
	if err != nil {
		return file, err
	}
	if video && !isVideo(file.Extension) {
		return File{}, ErrUnavailable
	}
	if _, _, _, err = ByteRange(r.Range, file.Size); err != nil {
		return File{Size: file.Size}, err
	}
	if len(r.UserAgent) > 2048 || strings.ContainsAny(r.UserAgent, "\x00\r\n") {
		return File{}, ErrUnavailable
	}
	source.UserAgent = r.UserAgent
	target, err := s.source.(LinkSource).ResolveLink(ctx, source)
	if err != nil {
		return File{}, ErrUnavailable
	}
	u, err := url.Parse(target)
	if err != nil || len(target) > 16384 || u.Scheme != "https" || u.User != nil || u.Opaque != "" || u.Fragment != "" || u.Host == "" || strings.ContainsAny(target, "\x00\r\n") {
		return File{}, ErrUnavailable
	}
	allowed := false
	for _, host := range hosts {
		if u.Host == host || u.Host == host+":443" {
			allowed = true
		}
	}
	if !allowed {
		return File{}, ErrUnavailable
	}
	file.DirectURL = u.String()
	return file, nil
}
