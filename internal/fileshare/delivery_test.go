package fileshare

import (
	"context"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/fileaccess"
	"testing"
)

type guestDelivery struct {
	fakeFiles
	deliveries int
	ua         string
	wrong      bool
}

func (f *guestDelivery) Deliver(_ context.Context, _ auth.Principal, r fileaccess.Request) (fileaccess.File, error) {
	f.deliveries++
	f.ua = r.UserAgent
	name := "file.pdf"
	if f.wrong {
		name = "other.pdf"
	}
	return fileaccess.File{Name: name, Size: 4, DirectURL: "https://cdn.example.test/file?temporary=opaque"}, f.err
}
func TestGuestDeliveryChecksGrantRevokeAndImmutableTargetBeforeRedirect(t *testing.T) {
	ctx := context.Background()
	store := &fakeStore{}
	files := &guestDelivery{}
	s, _ := New(store, files)
	record, link, err := s.Create(ctx, owner(), fileaccess.Request{}, "1h", "aB39")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Open(ctx, record.ID, link, "", false, "Browser/4"); err == nil || files.deliveries != 0 {
		t.Fatal("link secret bypassed grant")
	}
	grant, err := s.Redeem(ctx, link, "aB39")
	if err != nil {
		t.Fatal(err)
	}
	file, err := s.Open(ctx, record.ID, grant.Token, "bytes=0-1", false, "Browser/4")
	if err != nil || file.DirectURL == "" || file.Content.Body != nil || files.opened != 0 || files.deliveries != 1 || files.ua != "Browser/4" {
		t.Fatal("guest not redirected", err)
	}
	files.wrong = true
	if _, err := s.Open(ctx, record.ID, grant.Token, "", false); err == nil {
		t.Fatal("immutable target mismatch accepted")
	}
	files.wrong = false
	files.err = fileaccess.ErrUnavailable
	if _, err := s.Open(ctx, record.ID, grant.Token, "", false); err == nil || files.opened != 0 {
		t.Fatal("failed direct lookup silently proxied")
	}
	files.err = nil
	before := files.deliveries
	s.Revoke(ctx, owner(), record.ID)
	if _, err := s.Open(ctx, record.ID, grant.Token, "", false); err == nil || files.deliveries != before {
		t.Fatal("revoked grant resolved URL")
	}
}
