package adminread

import (
	"context"
	"errors"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"testing"
	"time"
)

type diagnosticFake struct {
	readerFake
	detail DownloadDetailRecord
	status string
}

func (f *diagnosticFake) FilterDownloads(ctx context.Context, b []string, n int, c, u, s string) ([]DownloadRecord, error) {
	f.status = s
	return f.ListDownloads(ctx, b, n, c, u)
}
func (f *diagnosticFake) ReadDownloadDetail(ctx context.Context, b []string, id string) (DownloadDetailRecord, error) {
	f.calls++
	_, f.deadline = ctx.Deadline()
	return f.detail, f.err
}
func detailFixture() DownloadDetailRecord {
	r := download()
	return DownloadDetailRecord{DownloadDetail: DownloadDetail{Download: r.Download, ReadAt: time.Now(), Source: "CLOUDSITE_POSTGRES_MANIFEST_JOB_COPY", ErrorCode: "NONE"}, LinkValid: true}
}
func TestAdminDownloadCursorScopeAndFiltering(t *testing.T) {
	ctx := context.Background()
	f := &diagnosticFake{}
	a, b := download(), download()
	b.ManifestID = jid
	f.downloads = []DownloadRecord{a, b}
	s, _ := NewService(f, []string{bid})
	p := admin()
	page, e := s.SearchDownloads(ctx, p, DownloadFilter{Limit: 1, Status: "PENDING"})
	if e != nil || len(page.Items) != 1 || page.NextCursor == "" || page.NextCursor == mid || f.status != "PENDING" || f.limit != 2 {
		t.Fatal(page, e)
	}
	for _, filter := range []DownloadFilter{{Limit: 1, Status: "ALL", Cursor: page.NextCursor}, {Limit: 1, Status: "PENDING", Owner: aid, Cursor: page.NextCursor}, {Limit: 1, Cursor: mid}, {Limit: 1, Status: "bad"}, {Limit: 1, Cursor: "offset:2"}, {Limit: 51}, {Limit: 1, Owner: "bad"}} {
		if _, e = s.SearchDownloads(ctx, p, filter); !errors.Is(e, ErrInvalid) {
			t.Fatal("bad filter/cursor", filter, e)
		}
	}
	q := p
	q.UserID = bid
	if _, e = s.SearchDownloads(ctx, q, DownloadFilter{Limit: 1, Status: "PENDING", Cursor: page.NextCursor}); !errors.Is(e, ErrInvalid) {
		t.Fatal("actor cursor", e)
	}
	other, _ := NewService(f, []string{aid})
	if _, e = other.SearchDownloads(ctx, p, DownloadFilter{Limit: 1, Status: "PENDING", Cursor: page.NextCursor}); !errors.Is(e, ErrInvalid) {
		t.Fatal("config cursor", e)
	}
	f.downloads = []DownloadRecord{b}
	next, e := s.SearchDownloads(ctx, p, DownloadFilter{Limit: 1, Status: "PENDING", Cursor: page.NextCursor})
	if e != nil || len(next.Items) != 1 || next.NextCursor != "" || f.cursor != mid {
		t.Fatal(next, e)
	}
	for _, mutate := range []func(*DownloadRecord){func(r *DownloadRecord) { r.OwnerUserID = "bad" }, func(r *DownloadRecord) { r.BindingID = aid }, func(r *DownloadRecord) { r.ManifestState = "UNKNOWN" }, func(r *DownloadRecord) { r.LinkValid = false }} {
		v := a
		mutate(&v)
		f.downloads = []DownloadRecord{v}
		if _, e = s.SearchDownloads(ctx, p, DownloadFilter{Limit: 20}); !errors.Is(e, ErrUnavailable) {
			t.Fatal("bad row", e)
		}
	}
	for _, principal := range []auth.Principal{{}, {UserID: aid, Login: "user", Role: auth.RoleUser}} {
		before := f.calls
		if _, e = s.SearchDownloads(ctx, principal, DownloadFilter{Limit: 20}); e == nil || f.calls != before {
			t.Fatal("auth", e)
		}
	}
}
func TestAdminDownloadDetailValidationAndOptionalPorts(t *testing.T) {
	ctx := context.Background()
	f := &diagnosticFake{detail: detailFixture()}
	s, _ := NewService(f, []string{bid})
	if d, e := s.DownloadDetail(ctx, admin(), mid); e != nil || d.JobID != nil || !f.deadline {
		t.Fatal(d, e)
	}
	for _, mutate := range []func(*DownloadDetailRecord){func(r *DownloadDetailRecord) { r.ManifestID = aid }, func(r *DownloadDetailRecord) { r.BindingID = aid }, func(r *DownloadDetailRecord) { r.LinkValid = false }, func(r *DownloadDetailRecord) { r.ReadAt = time.Time{} }, func(r *DownloadDetailRecord) { r.Source = "OTHER" }, func(r *DownloadDetailRecord) { r.ErrorCode = "SECRET" }, func(r *DownloadDetailRecord) { r.ErrorRecorded = true }, func(r *DownloadDetailRecord) { r.MaxAttempts = 3 }} {
		f.detail = detailFixture()
		mutate(&f.detail)
		if _, e := s.DownloadDetail(ctx, admin(), mid); !errors.Is(e, ErrUnavailable) {
			t.Fatal("bad detail", e)
		}
	}
	f.err = ErrDownloadNotFound
	if _, e := s.DownloadDetail(ctx, admin(), mid); !errors.Is(e, ErrDownloadNotFound) {
		t.Fatal(e)
	}
	f.err = errors.New("SECRET")
	if _, e := s.DownloadDetail(ctx, admin(), mid); !errors.Is(e, ErrUnavailable) {
		t.Fatal(e)
	}
	for _, p := range []auth.Principal{{}, {UserID: aid, Login: "user", Role: auth.RoleUser}} {
		before := f.calls
		if _, e := s.DownloadDetail(ctx, p, mid); e == nil || f.calls != before {
			t.Fatal("auth", e)
		}
	}
	legacy, _ := NewService(&readerFake{}, []string{bid})
	if _, e := legacy.DownloadDetail(ctx, admin(), mid); !errors.Is(e, ErrUnavailable) {
		t.Fatal(e)
	}
}
