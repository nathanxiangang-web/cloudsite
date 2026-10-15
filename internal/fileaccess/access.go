// Package fileaccess authorizes known canonical files, not acquisition side effects.
package fileaccess

import (
	"context"
	"errors"
	"io"
	"path"
	"strings"

	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/product"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

var ErrUnavailable = errors.New("file content unavailable")
var ErrUnsupported = errors.New("file preview unsupported")
var ErrTooLarge = errors.New("file preview too large")
var ErrRange = errors.New("file range not satisfiable")

type Request struct {
	BindingID               storage.BindingID
	ResourceID, Path, Range string
	Head                    bool
}
type SourceRequest struct {
	Mount, Path, Range string
	Size               int64
	Head               bool
}
type Content struct {
	Body         io.ReadCloser
	Status       int
	Length       int64
	ContentRange string
}
type Source interface {
	Open(context.Context, SourceRequest) (Content, error)
}
type File struct {
	Name, Extension string
	Size            int64
	Content         Content
}
type Service struct {
	reads    product.Reads
	topology product.TopologyReader
	source   Source
}

func New(reads product.Reads, topology product.TopologyReader, source Source) (*Service, error) {
	if reads == nil || topology == nil || source == nil {
		return nil, product.ErrInvalid
	}
	return &Service{reads, topology, source}, nil
}
func (s *Service) Open(ctx context.Context, p auth.Principal, r Request) (File, error) {
	file, source, err := s.describe(ctx, p, r)
	if err != nil {
		return file, err
	}
	start, end, partial, err := ByteRange(r.Range, file.Size)
	if err != nil {
		return File{Size: file.Size}, err
	}
	if partial {
		source.Range = FormatRange(start, end)
	}
	source.Head = r.Head
	body, err := s.source.Open(ctx, source)
	if err != nil {
		return File{Size: file.Size}, err
	}
	file.Content = body
	return file, nil
}

// Describe validates the same canonical identity as Open without fetching bytes.
func (s *Service) Describe(ctx context.Context, p auth.Principal, r Request) (File, error) {
	file, _, err := s.describe(ctx, p, r)
	return file, err
}
func (s *Service) describe(ctx context.Context, p auth.Principal, r Request) (File, SourceRequest, error) {
	if !p.Valid() {
		return File{}, SourceRequest{}, auth.ErrUnauthorized
	}
	if !product.ValidID(string(r.BindingID)) || !product.ValidOpaque(r.ResourceID) || r.ResourceID == "" || len(r.Path) > 2048 {
		return File{}, SourceRequest{}, product.ErrInvalid
	}
	item, err := s.reads.Resolve(ctx, p, r.BindingID, r.Path)
	if err != nil {
		return File{}, SourceRequest{}, err
	}
	if string(item.BindingID) != string(r.BindingID) || item.ResourceID != r.ResourceID || item.Path == nil || *item.Path != r.Path || item.Directory == nil || *item.Directory || item.SizeBytes == nil || *item.SizeBytes < 0 || item.Presence != "PRESENT" || item.CopyAvailability != nil && *item.CopyAvailability != "PRESENT" {
		return File{}, SourceRequest{}, product.ErrNotFound
	}
	binding, err := s.topology.GetBinding(ctx, r.BindingID)
	if err != nil || binding.ID != r.BindingID {
		return File{}, SourceRequest{}, ErrUnavailable
	}
	mount, err := storage.NormalizeMountPath(binding.OpenListMountPath)
	if err != nil || mount != binding.OpenListMountPath || r.Path == mount || mount != "/" && !strings.HasPrefix(r.Path, mount+"/") {
		return File{}, SourceRequest{}, product.ErrNotFound
	}
	name := path.Base(r.Path)
	if item.Name != nil && *item.Name != "" {
		name = *item.Name
	}
	name = strings.Map(func(c rune) rune {
		if c < 32 || c == 127 || c == '/' || c == '\\' {
			return '_'
		}
		return c
	}, name)
	return File{Name: name, Extension: strings.ToLower(strings.TrimPrefix(path.Ext(name), ".")), Size: *item.SizeBytes}, SourceRequest{Mount: mount, Path: r.Path, Size: *item.SizeBytes}, nil
}
