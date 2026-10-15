package fileaccess

import (
	"context"
	"errors"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/filecache"
	"github.com/nathanxiangang-web/panta/internal/product"
	"github.com/nathanxiangang-web/panta/internal/storage"
	"io"
	"os"
	"strings"
	"testing"
)

const bindingID = storage.BindingID("97000000-0000-4000-8000-000000000001")

type readsFake struct {
	product.Reads
	item  product.Resource
	err   error
	calls int
}

func (f *readsFake) Resolve(context.Context, auth.Principal, storage.BindingID, string) (product.Resource, error) {
	f.calls++
	return f.item, f.err
}

type topologyFake struct{ mount string }

func (f topologyFake) GetBinding(context.Context, storage.BindingID) (storage.Binding, error) {
	return storage.Binding{ID: bindingID, OpenListMountPath: f.mount}, nil
}

type sourceFake struct {
	calls   int
	request SourceRequest
}

func (f *sourceFake) Open(_ context.Context, r SourceRequest) (Content, error) {
	f.calls++
	f.request = r
	return Content{Body: io.NopCloser(strings.NewReader("data")), Status: 200, Length: 4}, nil
}
func resource() product.Resource {
	p := "/docs/file.md"
	dir := false
	size := int64(4)
	return product.Resource{BindingID: bindingID, ResourceID: "resource-one", Path: &p, Name: nil, Directory: &dir, SizeBytes: &size, Presence: "PRESENT"}
}
func principal() auth.Principal {
	return auth.Principal{UserID: "90000000-0000-4000-8000-000000000001", Login: "user", Role: auth.RoleUser}
}
func TestKnownCanonicalFileAuthorizationBeforeContent(t *testing.T) {
	reader := &readsFake{item: resource()}
	source := &sourceFake{}
	s, _ := New(reader, topologyFake{"/docs"}, source)
	request := Request{BindingID: bindingID, ResourceID: "resource-one", Path: "/docs/file.md", Range: "bytes=1-", Head: true}
	file, err := s.Open(context.Background(), principal(), request)
	if err != nil || file.Name != "file.md" || source.calls != 1 || source.request.Range != "bytes=1-3" || !source.request.Head {
		t.Fatal(err, source.request)
	}
	file.Content.Body.Close()
	for _, mutate := range []func(*product.Resource){func(r *product.Resource) { r.ResourceID = "wrong" }, func(r *product.Resource) { r.Directory = nil }, func(r *product.Resource) { d := true; r.Directory = &d }, func(r *product.Resource) { r.Presence = "REMOVED" }, func(r *product.Resource) { r.SizeBytes = nil }, func(r *product.Resource) { r.BindingID = "other" }, func(r *product.Resource) { v := "REMOVED"; r.CopyAvailability = &v }} {
		reader.item = resource()
		mutate(&reader.item)
		before := source.calls
		if _, err = s.Open(context.Background(), principal(), request); err == nil || source.calls != before {
			t.Fatal("invalid resource reached content")
		}
	}
	reader.item = resource()
	reader.err = product.ErrAmbiguous
	if _, err = s.Open(context.Background(), principal(), request); !errors.Is(err, product.ErrAmbiguous) {
		t.Fatal(err)
	}
	reader.err = nil
	s, _ = New(reader, topologyFake{"/outside"}, source)
	if _, err = s.Open(context.Background(), principal(), request); err == nil {
		t.Fatal("mount escape")
	}
	if _, err = s.Open(context.Background(), auth.Principal{}, request); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatal("missing auth")
	}
}
func TestSingleByteRanges(t *testing.T) {
	for _, test := range []struct {
		value      string
		start, end int64
		bad        bool
	}{{"", 0, 9, false}, {"bytes=0-2", 0, 2, false}, {"bytes=5-", 5, 9, false}, {"bytes=-3", 7, 9, false}, {"bytes=-99", 0, 9, false}, {"bytes=8-99", 8, 9, false}, {"bytes=10-", 0, 0, true}, {"bytes=3-1", 0, 0, true}, {"bytes=0-1,3-4", 0, 0, true}, {"bytes=-0", 0, 0, true}, {"bytes=+1-2", 0, 0, true}, {"bytes= 1-2", 0, 0, true}, {"bytes=999999999999999999999999-", 0, 0, true}} {
		start, end, partial, err := ByteRange(test.value, 10)
		if test.bad {
			if err == nil {
				t.Fatal(test.value)
			}
			continue
		}
		if err != nil || start != test.start || end != test.end || partial != (test.value != "") {
			t.Fatal(test.value, start, end, err)
		}
	}
	if _, _, _, err := ByteRange("bytes=0-", 0); err == nil {
		t.Fatal("empty range")
	}
}

type unusedPDF struct{}

func (unusedPDF) PDF(context.Context, io.ReadSeeker, int) ([]byte, int, error) {
	return nil, 0, ErrUnsupported
}
func TestCachedPreviewRevalidatesCanonicalIdentityEveryTime(t *testing.T) {
	root := t.TempDir()
	os.Chmod(root, 0700)
	cache, err := filecache.New(root)
	if err != nil {
		t.Fatal(err)
	}
	reader := &readsFake{item: resource()}
	source := &sourceFake{}
	files, _ := New(reader, topologyFake{"/docs"}, source)
	previews, _ := NewPreviewService(files, cache, unusedPDF{})
	r := Request{BindingID: bindingID, ResourceID: "resource-one", Path: "/docs/file.md"}
	for n := 0; n < 2; n++ {
		result, err := previews.Preview(context.Background(), principal(), r, 1)
		if err != nil || result.Text != "data" {
			t.Fatal(result, err)
		}
	}
	if source.calls != 1 || reader.calls != 2 {
		t.Fatal("cache miss or auth shortcut", source.calls, reader.calls)
	}
	reader.item.Presence = "REMOVED"
	if _, err = previews.Preview(context.Background(), principal(), r, 1); err == nil {
		t.Fatal("cache bypassed canonical removal")
	}
	if source.calls != 1 {
		t.Fatal("unnecessary source access")
	}
}
