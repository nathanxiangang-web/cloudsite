package fileshare

import (
	"context"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/fileaccess"
)

// Existing proxy-mode tests exercise the same explicit delivery port.
func (f *fakeFiles) Deliver(ctx context.Context, p auth.Principal, r fileaccess.Request) (fileaccess.File, error) {
	return f.Open(ctx, p, r)
}
