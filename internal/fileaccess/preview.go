package fileaccess

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

const MaxText = 2 << 20
const MaxOffice = 16 << 20

func Preview(file File) (string, error) {
	if file.Content.Body == nil {
		return "", ErrUnavailable
	}
	limit := int64(MaxText)
	office := file.Extension == "docx" || file.Extension == "xlsx" || file.Extension == "pptx"
	if office {
		limit = MaxOffice
	} else if !strings.Contains("|txt|md|markdown|csv|", "|"+file.Extension+"|") {
		return "", ErrUnsupported
	}
	if file.Size > limit {
		return "", ErrTooLarge
	}
	raw, err := io.ReadAll(io.LimitReader(file.Content.Body, limit+1))
	if err != nil {
		return "", ErrUnavailable
	}
	if int64(len(raw)) != file.Size || int64(len(raw)) > limit {
		return "", ErrTooLarge
	}
	if !office {
		if !utf8.Valid(raw) || bytes.ContainsRune(raw, 0) {
			return "", ErrUnsupported
		}
		return string(raw), nil
	}
	archive, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil || len(archive.File) > 200 {
		return "", ErrUnsupported
	}
	var total uint64
	names := map[string]bool{}
	for _, entry := range archive.File {
		if entry.UncompressedSize64 > (64<<20)-total || names[entry.Name] || strings.Contains(entry.Name, "..") || strings.Contains(entry.Name, "\\") {
			return "", ErrTooLarge
		}
		total += entry.UncompressedSize64
		names[entry.Name] = true
	}
	var selected []*zip.File
	for _, entry := range archive.File {
		n := entry.Name
		if file.Extension == "docx" && n == "word/document.xml" || file.Extension == "pptx" && strings.HasPrefix(n, "ppt/slides/slide") && path.Ext(n) == ".xml" && !strings.Contains(strings.TrimPrefix(n, "ppt/slides/"), "/") || file.Extension == "xlsx" && (n == "xl/sharedStrings.xml" || strings.HasPrefix(n, "xl/worksheets/sheet") && path.Ext(n) == ".xml" && !strings.Contains(strings.TrimPrefix(n, "xl/worksheets/"), "/")) {
			selected = append(selected, entry)
		}
	}
	if len(selected) == 0 {
		return "", ErrUnsupported
	}
	number := func(n string) int {
		i := strings.TrimSuffix(path.Base(n), ".xml")
		i = strings.TrimLeft(i, "abcdefghijklmnopqrstuvwxyz")
		v, _ := strconv.Atoi(i)
		return v
	}
	sort.Slice(selected, func(i, j int) bool {
		if selected[i].Name == "xl/sharedStrings.xml" {
			return true
		}
		if selected[j].Name == "xl/sharedStrings.xml" {
			return false
		}
		return number(selected[i].Name) < number(selected[j].Name)
	})
	var out strings.Builder
	var shared []string
	for _, entry := range selected {
		if entry.UncompressedSize64 > 4<<20 {
			return "", ErrTooLarge
		}
		reader, err := entry.Open()
		if err != nil {
			return "", ErrUnsupported
		}
		data, err := io.ReadAll(io.LimitReader(reader, (4<<20)+1))
		reader.Close()
		if err != nil || len(data) > 4<<20 {
			return "", ErrTooLarge
		}
		if file.Extension == "xlsx" {
			if entry.Name == "xl/sharedStrings.xml" {
				shared, err = sharedStrings(data)
			} else {
				out.WriteString(path.Base(entry.Name) + "\n")
				err = sheetText(data, shared, &out)
			}
		} else {
			err = documentText(data, &out)
		}
		if err != nil {
			return "", ErrUnsupported
		}
		out.WriteString("\n")
		if out.Len() > MaxText {
			return "", ErrTooLarge
		}
	}
	return strings.TrimSpace(out.String()), nil
}
func documentText(raw []byte, out *strings.Builder) error {
	d := xml.NewDecoder(bytes.NewReader(raw))
	for {
		t, err := d.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch e := t.(type) {
		case xml.StartElement:
			if e.Name.Local == "t" {
				var text string
				if err = d.DecodeElement(&text, &e); err != nil {
					return err
				}
				out.WriteString(text)
			}
			if e.Name.Local == "tab" {
				out.WriteByte('\t')
			}
		case xml.EndElement:
			if e.Name.Local == "p" || e.Name.Local == "tr" {
				out.WriteByte('\n')
			}
		}
		if out.Len() > MaxText {
			return ErrTooLarge
		}
	}
}
func sharedStrings(raw []byte) ([]string, error) {
	d := xml.NewDecoder(bytes.NewReader(raw))
	var list []string
	for {
		t, err := d.Token()
		if err == io.EOF {
			return list, nil
		}
		if err != nil {
			return nil, err
		}
		if e, ok := t.(xml.StartElement); ok && e.Name.Local == "si" {
			var value strings.Builder
			var item struct {
				Parts []string `xml:"t"`
				Runs  []struct {
					Text string `xml:"t"`
				} `xml:"r"`
			}
			if err = d.DecodeElement(&item, &e); err != nil {
				return nil, err
			}
			for _, s := range item.Parts {
				value.WriteString(s)
			}
			for _, r := range item.Runs {
				value.WriteString(r.Text)
			}
			list = append(list, value.String())
			if len(list) > 50000 {
				return nil, ErrTooLarge
			}
		}
	}
}
func sheetText(raw []byte, shared []string, out *strings.Builder) error {
	d := xml.NewDecoder(bytes.NewReader(raw))
	for {
		t, err := d.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch e := t.(type) {
		case xml.StartElement:
			if e.Name.Local == "c" {
				var cell struct {
					Type   string `xml:"t,attr"`
					Ref    string `xml:"r,attr"`
					Value  string `xml:"v"`
					Inline string `xml:"is>t"`
				}
				if err = d.DecodeElement(&cell, &e); err != nil {
					return err
				}
				value := cell.Value
				if cell.Type == "s" {
					index, e := strconv.Atoi(value)
					if e != nil || index < 0 || index >= len(shared) {
						return ErrUnsupported
					}
					value = shared[index]
				} else if cell.Type == "inlineStr" {
					value = cell.Inline
				}
				out.WriteString(cell.Ref + ": " + value + "\t")
			}
		case xml.EndElement:
			if e.Name.Local == "row" {
				out.WriteByte('\n')
			}
		}
		if out.Len() > MaxText {
			return ErrTooLarge
		}
	}
}
