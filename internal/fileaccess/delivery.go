package fileaccess

import (
	"context"
	"net"
	"net/url"
	"strings"

	"github.com/nathanxiangang-web/panta/internal/auth"
)

// LinkSource resolves a known file without opening its byte stream.
type LinkSource interface {
	ResolveLink(context.Context, SourceRequest) (string, error)
}

// EnableDirectLinks is composition-only. An empty list preserves legacy proxy
// delivery. Exact HTTPS DNS hosts also define the browser media CSP sources.
func (s *Service) EnableDirectLinks(hosts []string) error {
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
	s.directHosts = append([]string(nil), hosts...)
	return nil
}

func (s *Service) MediaSources() []string {
	result := make([]string, 0, len(s.directHosts))
	for _, host := range s.directHosts {
		result = append(result, "https://"+host)
	}
	return result
}

// Deliver preserves canonical authorization for each redirect. Open remains the
// byte-only port used by the private document preview cache.
func (s *Service) Deliver(ctx context.Context, p auth.Principal, r Request) (File, error) {
	if len(s.directHosts) == 0 {
		return s.Open(ctx, p, r)
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
	for _, host := range s.directHosts {
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
