package fileaccess

import (
	"context"
	"errors"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/product"
	"strings"
	"testing"
)

type linkFake struct {
	sourceFake
	links   int
	target  string
	linkErr error
	ua      string
}

func (s *linkFake) ResolveLink(_ context.Context, r SourceRequest) (string, error) {
	s.links++
	s.ua = r.UserAgent
	return s.target, s.linkErr
}

func TestDirectDeliveryKeepsCanonicalAuthorizationAndNeverOpensBytes(t *testing.T) {
	reader := &readsFake{item: resource()}
	source := &linkFake{target: "https://cdn.example.test/file?opaque=temporary"}
	s, _ := New(reader, topologyFake{"/docs"}, source)
	if err := s.EnableDirectLinks([]string{"cdn.example.test"}); err != nil {
		t.Fatal(err)
	}
	request := Request{BindingID: bindingID, ResourceID: "resource-one", Path: "/docs/file.md", Range: "bytes=1-2", UserAgent: "Browser/1"}
	for _, head := range []bool{false, true} {
		request.Head = head
		file, err := s.Deliver(context.Background(), principal(), request)
		if err != nil || file.DirectURL != source.target || file.Content.Body != nil || source.calls != 0 || source.ua != "Browser/1" {
			t.Fatal("not byte-free direct delivery", err)
		}
	}
	for _, mutate := range []func(*product.Resource){func(r *product.Resource) { r.ResourceID = "wrong" }, func(r *product.Resource) { r.Presence = "REMOVED" }, func(r *product.Resource) { r.Directory = nil }, func(r *product.Resource) { r.SizeBytes = nil }, func(r *product.Resource) { r.BindingID = "wrong" }} {
		reader.item = resource()
		mutate(&reader.item)
		before := source.links
		if _, err := s.Deliver(context.Background(), principal(), request); err == nil || source.links != before {
			t.Fatal("unauthorized direct-link lookup")
		}
	}
	reader.item = resource()
	reader.err = product.ErrAmbiguous
	if _, err := s.Deliver(context.Background(), principal(), request); !errors.Is(err, product.ErrAmbiguous) {
		t.Fatal(err)
	}
	reader.err = nil
	before := source.links
	if _, err := s.Deliver(context.Background(), auth.Principal{}, request); err == nil || source.links != before {
		t.Fatal("anonymous URL disclosure")
	}
	request.Range = "bytes=5-6"
	if _, err := s.Deliver(context.Background(), principal(), request); !errors.Is(err, ErrRange) || source.links != before {
		t.Fatal("invalid range resolved link")
	}
	request.Range = ""
	request.UserAgent = "bad\r\nheader"
	if _, err := s.Deliver(context.Background(), principal(), request); err == nil || source.links != before {
		t.Fatal("header injection")
	}
	request.UserAgent = "Browser/1"
	for _, target := range []string{"http://cdn.example.test/a", "https://evil.example/a", "https://cdn.example.test.evil/a", "https://user@cdn.example.test/a", "https://cdn.example.test/a#x", "https://127.0.0.1/a", "https://cdn.example.test:444/a", "https://cdn.example.test/a\r\nX: yes", ""} {
		source.target = target
		if _, err := s.Deliver(context.Background(), principal(), request); err == nil || source.calls != 0 {
			t.Fatal("unsafe direct target accepted")
		}
	}
	source.target = "https://cdn.example.test:443/a"
	if _, err := s.Deliver(context.Background(), principal(), request); err != nil {
		t.Fatal("explicit HTTPS port", err)
	}
	source.linkErr = errors.New("secret-link-must-not-leak")
	if _, err := s.Deliver(context.Background(), principal(), request); err != ErrUnavailable || source.calls != 0 {
		t.Fatal("unsafe fallback or error leak")
	}
	source.linkErr = nil
	file, err := s.Open(context.Background(), principal(), request)
	if err != nil || file.DirectURL != "" || source.calls != 1 {
		t.Fatal("preview stream port changed", err)
	}
	file.Content.Body.Close()
	if len(s.MediaSources()) != 1 || s.MediaSources()[0] != "https://cdn.example.test" {
		t.Fatal("unbounded media CSP")
	}
}

func TestDirectHostConfigurationFailsClosedAndDefaultsToProxy(t *testing.T) {
	source := &linkFake{}
	s, _ := New(&readsFake{item: resource()}, topologyFake{"/docs"}, source)
	for _, hosts := range [][]string{{"*"}, {"https://cdn.example.test"}, {"cdn.example.test; script-src *"}, {"127.0.0.1"}, {"localhost"}, {"cdn.example.test", "cdn.example.test"}, {"cdn.local"}, {"cdn.localhost"}, {"CDN.example.test"}, {"-a.example.test"}, {strings.Repeat("a", 64) + ".test"}} {
		if s.EnableDirectLinks(hosts) == nil {
			t.Fatal("invalid host configuration")
		}
	}
	if s.EnableDirectLinks(nil) != nil {
		t.Fatal("legacy proxy rejected")
	}
	file, err := s.Deliver(context.Background(), principal(), Request{BindingID: bindingID, ResourceID: "resource-one", Path: "/docs/file.md"})
	if err != nil || file.DirectURL != "" || source.calls != 1 || source.links != 0 {
		t.Fatal("default changed", err)
	}
	file.Content.Body.Close()
	plain, _ := New(&readsFake{item: resource()}, topologyFake{"/docs"}, &sourceFake{})
	if plain.EnableDirectLinks([]string{"cdn.example.test"}) == nil {
		t.Fatal("missing source port")
	}
}
