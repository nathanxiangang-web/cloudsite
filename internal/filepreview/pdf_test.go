package filepreview

import (
	"bytes"
	"context"
	"fmt"
	"testing"
)

func TestNativePDFRendersRealPNGWithBounds(t *testing.T) {
	renderer, err := New()
	if err != nil {
		t.Skip("Poppler/prlimit not installed; real staging evidence required")
	}
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	objects := []string{"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Kids [3 0 R] /Count 1 >>", "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 4 0 R >>", "<< /Length 0 >>\nstream\n\nendstream"}
	offsets := []int{0}
	for n, object := range objects {
		offsets = append(offsets, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", n+1, object)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 5\n0000000000 65535 f \n")
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&b, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size 5 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xref)
	png, pages, err := renderer.PDF(context.Background(), bytes.NewReader(b.Bytes()), 1)
	if err != nil || pages != 1 || len(png) < 8 {
		t.Fatal(pages, err)
	}
	if _, _, err = renderer.PDF(context.Background(), bytes.NewReader(b.Bytes()), 2); err == nil {
		t.Fatal("nonexistent page")
	}
	if _, _, err = renderer.PDF(context.Background(), bytes.NewReader([]byte("not a PDF")), 1); err == nil {
		t.Fatal("malformed PDF")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err = renderer.PDF(ctx, bytes.NewReader(b.Bytes()), 1); err == nil {
		t.Fatal("cancelled render")
	}
}
