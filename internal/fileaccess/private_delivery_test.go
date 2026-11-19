package fileaccess

import (
	"context"
	"errors"
	"testing"

	"github.com/nathanxiangang-web/panta/internal/product"
)

func TestPrivateAndPublicDownloadsAlwaysRedirectWithoutOpeningBytes(t *testing.T) {
	reader := &readsFake{item: resource()}
	privatePath := "/云下载/_users/a/result.bin"
	reader.item.Path = &privatePath
	source := &linkFake{target: "https://cdn.example.test/private"}
	service, _ := New(reader, topologyFake{"/"}, source)
	if err := service.EnableDirectLinks([]string{"cdn.example.test"}); err != nil {
		t.Fatal(err)
	}
	private, err := service.Deliver(context.Background(), principal(), Request{BindingID: bindingID, ResourceID: "resource-one", Path: privatePath})
	if err != nil || private.DirectURL != source.target || private.Content.Body != nil || source.links != 1 || source.calls != 0 {
		t.Fatalf("private download did not redirect: file=%#v links=%d opens=%d err=%v", private, source.links, source.calls, err)
	}
	reader.item = resource()
	public, err := service.Deliver(context.Background(), principal(), Request{BindingID: bindingID, ResourceID: "resource-one", Path: "/docs/file.md"})
	if err != nil || public.DirectURL == "" || public.Content.Body != nil || source.links != 2 || source.calls != 0 {
		t.Fatalf("public download did not redirect: file=%#v links=%d opens=%d err=%v", public, source.links, source.calls, err)
	}
}

func TestPrivateVideoAndAttachmentRedirectAfterCanonicalAuthorization(t *testing.T) {
	path := "/云下载/_users/a/result.mp4"
	reader := &readsFake{item: resource()}
	reader.item.Path = &path
	source := &linkFake{target: "https://cdn.example.test/video?signature=temporary"}
	service, _ := New(reader, topologyFake{"/"}, source)
	if err := service.EnableDirectLinks([]string{"cdn.example.test"}); err != nil {
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
	source.target = "https://cdn.example.test/video?signature=temporary"
	request.Inline = false
	file, err = service.Deliver(context.Background(), principal(), request)
	if err != nil || file.DirectURL != source.target || source.calls != 0 || source.links != 2 || file.Content.Body != nil {
		t.Fatalf("attachment delivery must also redirect, error = %v", err)
	}
}
