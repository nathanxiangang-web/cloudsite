package fileaccess

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"io"
)

type PreviewCache interface {
	Open(context.Context, string, int64, func(context.Context) (io.ReadCloser, error)) (io.ReadSeekCloser, error)
}
type PDFRenderer interface {
	PDF(context.Context, io.ReadSeeker, int) ([]byte, int, error)
}
type PreviewResult struct {
	Text   string `json:"text,omitempty"`
	Format string `json:"format"`
	Image  []byte `json:"image_base64,omitempty"`
	Page   int    `json:"page,omitempty"`
	Pages  int    `json:"pages,omitempty"`
}
type PreviewService struct {
	files *Service
	cache PreviewCache
	pdf   PDFRenderer
}

func NewPreviewService(files *Service, cache PreviewCache, pdf PDFRenderer) (*PreviewService, error) {
	if files == nil || cache == nil || pdf == nil {
		return nil, ErrUnavailable
	}
	return &PreviewService{files, cache, pdf}, nil
}
func (s *PreviewService) Preview(ctx context.Context, p auth.Principal, r Request, page int) (PreviewResult, error) {
	if page < 1 || page > 2000 {
		return PreviewResult{}, ErrUnsupported
	}
	file, source, err := s.files.describe(ctx, p, r)
	if err != nil {
		return PreviewResult{}, err
	}
	if file.Extension != "pdf" && file.Extension != "docx" && file.Extension != "xlsx" && file.Extension != "pptx" && file.Extension != "txt" && file.Extension != "md" && file.Extension != "markdown" && file.Extension != "csv" {
		return PreviewResult{}, ErrUnsupported
	}
	if file.Size > 128<<20 || file.Extension != "pdf" && file.Size > MaxOffice {
		return PreviewResult{}, ErrTooLarge
	}
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%d", r.BindingID, r.ResourceID, r.Path, file.Name, file.Size)))
	cached, err := s.cache.Open(ctx, hex.EncodeToString(hash[:]), file.Size, func(ctx context.Context) (io.ReadCloser, error) {
		content, err := s.files.source.Open(ctx, source)
		if err != nil {
			return nil, err
		}
		return content.Body, nil
	})
	if err != nil {
		return PreviewResult{}, ErrUnavailable
	}
	defer cached.Close()
	if file.Extension == "pdf" {
		png, pages, err := s.pdf.PDF(ctx, cached, page)
		if err != nil {
			return PreviewResult{}, err
		}
		return PreviewResult{Format: "pdf", Image: png, Page: page, Pages: pages}, nil
	}
	if page != 1 {
		return PreviewResult{}, ErrUnsupported
	}
	file.Content.Body = cached
	text, err := Preview(file)
	if err != nil {
		return PreviewResult{}, err
	}
	return PreviewResult{Format: file.Extension, Text: text}, nil
}
