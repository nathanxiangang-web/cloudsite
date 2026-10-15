// Package filepreview renders PDF bytes in bounded system-tool subprocesses.
package filepreview

import (
	"bytes"
	"context"
	"github.com/nathanxiangang-web/panta/internal/fileaccess"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type Renderer struct{ slots chan struct{} }

func New() (*Renderer, error) {
	for _, name := range []string{"/usr/bin/prlimit", "/usr/bin/pdfinfo", "/usr/bin/pdftoppm"} {
		info, err := os.Stat(name)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 {
			return nil, fileaccess.ErrUnavailable
		}
	}
	return &Renderer{slots: make(chan struct{}, 2)}, nil
}

type bounded struct {
	bytes.Buffer
	max int
}

func (b *bounded) Write(p []byte) (int, error) {
	if len(p) > b.max-b.Len() {
		return 0, fileaccess.ErrTooLarge
	}
	return b.Buffer.Write(p)
}
func run(ctx context.Context, input io.Reader, limit int, tool string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, "/usr/bin/prlimit", append([]string{"--cpu=20", "--as=805306368", "--nofile=96", "--", tool}, args...)...)
	command.Stdin = input
	output := &bounded{max: limit}
	command.Stdout = output
	command.Stderr = io.Discard
	if command.Run() != nil {
		return nil, fileaccess.ErrUnsupported
	}
	return output.Bytes(), nil
}
func (r *Renderer) PDF(ctx context.Context, input io.ReadSeeker, page int) ([]byte, int, error) {
	select {
	case r.slots <- struct{}{}:
		defer func() { <-r.slots }()
	case <-ctx.Done():
		return nil, 0, ctx.Err()
	}
	ctx, cancel := context.WithTimeout(ctx, 35*time.Second)
	defer cancel()
	if _, err := input.Seek(0, io.SeekStart); err != nil {
		return nil, 0, fileaccess.ErrUnavailable
	}
	info, err := run(ctx, input, 64<<10, "/usr/bin/pdfinfo", "-")
	if err != nil {
		return nil, 0, err
	}
	pages := 0
	for _, line := range strings.Split(string(info), "\n") {
		if strings.HasPrefix(line, "Pages:") {
			pages, _ = strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "Pages:")))
		}
	}
	if page < 1 || page > pages || pages > 2000 {
		return nil, 0, fileaccess.ErrUnsupported
	}
	if _, err := input.Seek(0, io.SeekStart); err != nil {
		return nil, 0, fileaccess.ErrUnavailable
	}
	png, err := run(ctx, input, 8<<20, "/usr/bin/pdftoppm", "-q", "-f", strconv.Itoa(page), "-l", strconv.Itoa(page), "-singlefile", "-scale-to", "1400", "-png", "-")
	if err != nil {
		return nil, 0, err
	}
	if len(png) < 8 || !bytes.Equal(png[:8], []byte{137, 80, 78, 71, 13, 10, 26, 10}) {
		return nil, 0, fileaccess.ErrUnsupported
	}
	return png, pages, nil
}
