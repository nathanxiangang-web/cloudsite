package fileaccess

import (
	"context"
	"testing"
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
