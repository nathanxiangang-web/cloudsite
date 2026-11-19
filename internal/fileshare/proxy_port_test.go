package fileshare

import (
	"context"
	"io"
	"strings"

	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/fileaccess"
)

// Test delivery fakes model only a 302-capable result; byte streams are rejected.
func (f *fakeFiles) Deliver(_ context.Context, _ auth.Principal, _ fileaccess.Request) (fileaccess.File, error) {
	if f.err != nil {
		return fileaccess.File{}, f.err
	}
	if f.bodyOnly {
		f.opened++
		return fileaccess.File{Name: "file.pdf", Size: 4, Content: fileaccess.Content{Body: io.NopCloser(strings.NewReader("test"))}}, nil
	}
	return fileaccess.File{Name: "file.pdf", Size: 4, DirectURL: "https://cdn.example.test/file?temporary=opaque"}, nil
}
