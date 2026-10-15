package fileaccess

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
)

func document(ext string, entries map[string]string) File {
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for name, value := range entries {
		w, _ := z.Create(name)
		w.Write([]byte(value))
	}
	z.Close()
	raw := b.Bytes()
	return File{Extension: ext, Size: int64(len(raw)), Content: Content{Body: io.NopCloser(bytes.NewReader(raw))}}
}
func TestSafeTextAndOfficePreview(t *testing.T) {
	cases := []struct {
		file File
		want string
	}{
		{File{Extension: "md", Size: 8, Content: Content{Body: io.NopCloser(strings.NewReader("<script>"))}}, "<script>"},
		{document("docx", map[string]string{"word/document.xml": `<document><p><t>真实文档</t></p></document>`}), "真实文档"},
		{document("pptx", map[string]string{"ppt/slides/slide2.xml": `<slide><p><t>second</t></p></slide>`, "ppt/slides/slide1.xml": `<slide><p><t>first</t></p></slide>`}), "first\n\nsecond"},
		{document("xlsx", map[string]string{"xl/sharedStrings.xml": `<sst><si><t>Cell</t></si></sst>`, "xl/worksheets/sheet1.xml": `<worksheet><row><c r="A1" t="s"><v>0</v></c><c r="B1"><f>do not run</f><v>42</v></c></row></worksheet>`}), "A1: Cell\tB1: 42"},
	}
	for _, c := range cases {
		text, err := Preview(c.file)
		c.file.Content.Body.Close()
		if err != nil || !strings.Contains(text, c.want) {
			t.Fatal(text, err, c.want)
		}
	}
}
func TestPreviewLimitsAndMalformedArchives(t *testing.T) {
	for _, file := range []File{
		{Extension: "txt", Size: MaxText + 1, Content: Content{Body: io.NopCloser(strings.NewReader(""))}},
		{Extension: "docx", Size: MaxOffice + 1, Content: Content{Body: io.NopCloser(strings.NewReader(""))}},
		{Extension: "docx", Size: 3, Content: Content{Body: io.NopCloser(strings.NewReader("bad"))}},
		{Extension: "html", Size: 0, Content: Content{Body: io.NopCloser(strings.NewReader(""))}},
		document("docx", map[string]string{"word/document.xml": "<bad>"}),
		document("docx", map[string]string{"../escape": "payload"}),
		document("docx", map[string]string{"word/document.xml": strings.Repeat("A", (4<<20)+1)}),
	} {
		if _, err := Preview(file); err == nil {
			t.Fatal("unsafe preview accepted", file.Extension)
		}
		file.Content.Body.Close()
	}
}
