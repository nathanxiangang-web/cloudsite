package fileaccess

import (
	"context"
	"errors"
	"testing"

	"github.com/nathanxiangang-web/panta/internal/product"
)

func TestPrivateDeliveryNeverRedirectsToExternalURL(t *testing.T) {
	reader := &readsFake{item: resource()}
	privatePath := "/云下载/_users/a/result.bin"
	reader.item.Path = &privatePath
	source := &linkFake{target: "https://cdn.example.test/private"}
	service, _ := New(reader, topologyFake{"/"}, source)
	if err := service.EnableDirectLinks([]string{"cdn.example.test"}); err != nil {
		t.Fatal(err)
	}
	file, err := service.Deliver(context.Background(), principal(), Request{BindingID: bindingID, ResourceID: "resource-one", Path: privatePath})
	if err != nil {
		t.Fatal(err)
	}
	if file.DirectURL != "" || source.links != 0 || source.calls != 1 {
		t.Fatalf("private delivery leaked redirect: url=%q links=%d opens=%d", file.DirectURL, source.links, source.calls)
	}
	if file.Content.Body != nil {
		_ = file.Content.Body.Close()
	}
	reader.item = resource()
	public, err := service.Deliver(context.Background(), principal(), Request{BindingID: bindingID, ResourceID: "resource-one", Path: "/docs/file.md"})
	if err != nil || public.DirectURL == "" || source.links != 1 {
		t.Fatalf("public direct delivery changed: file=%#v links=%d error=%v", public, source.links, err)
	}
}

func TestPrivateVideoRedirectRequiresInlineAndCanonicalAccess(t *testing.T) {
	path := "/云下载/_users/a/result.mp4"
	reader := &readsFake{item: resource()}
	reader.item.Path = &path
	source := &linkFake{target: "https://cdn.example.test/video?signature=temporary"}
	service, _ := New(reader, topologyFake{"/"}, source)
	if err := service.EnableVideoDirectLinks([]string{"cdn.example.test"}); err != nil {
		t.Fatal(err)
	}
	request := Request{BindingID: bindingID, ResourceID: "resource-one", Path: path, Inline: true, Range: "bytes=0-0", UserAgent: "Browser/1"}
	file, err := service.Deliver(context.Background(), principal(), request)
	if err != nil || file.DirectURL != source.target || source.calls != 0 || source.links != 1 || source.ua != request.UserAgent {
		t.Fatalf("inline video must redirect without opening bytes, error = %v", err)
	}
	reader.err = product.ErrNotFound
	if _, err := service.Deliver(context.Background(), principal(), request); !errors.Is(err, product.ErrNotFound) || source.links != 1 || source.calls != 0 {
		t.Fatalf("private owner check must precede link resolution, error = %v", err)
	}
	reader.err = nil
	source.target = "https://other.example.test/video"
	if _, err := service.Deliver(context.Background(), principal(), request); !errors.Is(err, ErrUnavailable) || source.calls != 0 {
		t.Fatalf("invalid video link must fail without using proxy traffic, error = %v", err)
	}
	request.Inline = false
	file, err = service.Deliver(context.Background(), principal(), request)
	if err != nil || file.DirectURL != "" || source.calls != 1 || source.links != 2 {
		t.Fatalf("attachment delivery unexpectedly changed, error = %v", err)
	}
	defer file.Content.Body.Close()
}
