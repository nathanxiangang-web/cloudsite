package adminread

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nathanxiangang-web/panta/internal/auth"
)

type shareReaderStub struct {
	readerFake
	page  ShareDirectory
	calls int
}

func (f *shareReaderStub) SearchShares(context.Context, ShareFilter) (ShareDirectory, error) {
	f.calls++
	return f.page, nil
}

func TestShareDirectoryRejectsMalformedEvidenceAndUnauthorizedReads(t *testing.T) {
	stamp := time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC)
	item := Share{ShareID: aid, OwnerID: bid, BindingID: mid, CanonicalPath: "/file.txt", FileName: "file.txt", SizeBytes: 4, CreatedAt: stamp, ExpiresAt: stamp.Add(time.Hour), Active: true}
	valid := func() ShareDirectory {
		return ShareDirectory{Items: []Share{item}, Total: 1, ReadAt: stamp, Scope: "FILTERED_PRODUCT_SHARES"}
	}
	cases := []struct {
		name   string
		change func(*ShareDirectory)
	}{
		{"negative-total", func(p *ShareDirectory) { p.Total = -1 }},
		{"wrong-scope", func(p *ShareDirectory) { p.Scope = "OTHER" }},
		{"missing-time", func(p *ShareDirectory) { p.ReadAt = time.Time{} }},
		{"more-items-than-total", func(p *ShareDirectory) { p.Total = 0 }},
		{"wrong-cursor", func(p *ShareDirectory) { p.NextCursor = bid }},
		{"invalid-id", func(p *ShareDirectory) { p.Items[0].ShareID = "invalid" }},
		{"negative-size", func(p *ShareDirectory) { p.Items[0].SizeBytes = -1 }},
		{"bad-expiry", func(p *ShareDirectory) { p.Items[0].ExpiresAt = stamp.Add(-time.Hour) }},
		{"duplicate-items", func(p *ShareDirectory) { p.Items = append(p.Items, item); p.Total = 2 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			page := valid()
			tc.change(&page)
			f := &shareReaderStub{page: page}
			s, _ := NewService(f, []string{mid})
			if _, e := s.SearchShares(context.Background(), admin(), ShareFilter{Limit: 2}); !errors.Is(e, ErrUnavailable) {
				t.Fatal("malformed page accepted", e)
			}
		})
	}
	f := &shareReaderStub{page: valid()}
	s, _ := NewService(f, []string{mid})
	for _, p := range []auth.Principal{{}, {UserID: aid, Login: "user", Role: auth.RoleUser}} {
		if _, e := s.SearchShares(context.Background(), p, ShareFilter{Limit: 2}); e == nil {
			t.Fatal("unauthorized read accepted")
		}
	}
	if f.calls != 0 {
		t.Fatal("unauthorized request reached reader", f.calls)
	}
	if _, e := s.SearchShares(context.Background(), admin(), ShareFilter{Limit: 2}); e != nil {
		t.Fatal("valid page rejected", e)
	}
}
