package adminread

import (
	"context"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"testing"
	"time"
)

type directoryFake struct {
	readerFake
	page   UserDirectory
	detail UserDetail
}

func (f *directoryFake) SearchUsers(context.Context, UserFilter) (UserDirectory, error) {
	f.calls++
	return f.page, f.err
}
func (f *directoryFake) UserDetail(context.Context, string) (UserDetail, error) {
	f.calls++
	return f.detail, f.err
}
func TestDirectoryPermissionValidationAndUnknown(t *testing.T) {
	f := &directoryFake{page: UserDirectory{Items: []User{user(aid)}, Total: 1, ReadAt: time.Now(), Scope: "FILTERED_PRODUCT_USERS"}, detail: UserDetail{User: user(aid)}}
	s, _ := NewService(f, []string{bid})
	valid := UserFilter{Limit: 1}
	if _, e := s.SearchUsers(context.Background(), admin(), valid); e != nil {
		t.Fatal(e)
	}
	for _, bad := range []UserFilter{{Limit: 0}, {Limit: 51}, {Limit: 1, Search: "%sql"}, {Limit: 1, Search: " invalid"}, {Limit: 1, Status: "fake"}, {Limit: 1, Cursor: "bad"}} {
		if _, e := s.SearchUsers(context.Background(), admin(), bad); e != ErrInvalid {
			t.Fatal("invalid filter", bad, e)
		}
	}
	p := admin()
	p.Role = auth.RoleUser
	calls := f.calls
	if _, e := s.SearchUsers(context.Background(), p, valid); e != ErrForbidden || f.calls != calls {
		t.Fatal("USER searched")
	}
	if _, e := s.UserDetail(context.Background(), p, aid); e != ErrForbidden || f.calls != calls {
		t.Fatal("USER detail")
	}
	f.page.Total = -1
	if _, e := s.SearchUsers(context.Background(), admin(), valid); e != ErrUnavailable {
		t.Fatal("fake count accepted")
	}
	f.detail.ActiveDownloads = 1
	if _, e := s.UserDetail(context.Background(), admin(), aid); e != ErrUnavailable {
		t.Fatal("invalid counters accepted")
	}
	f.err = ErrNotFound
	if _, e := s.UserDetail(context.Background(), admin(), aid); e != ErrNotFound {
		t.Fatal("not found lost")
	}
}
