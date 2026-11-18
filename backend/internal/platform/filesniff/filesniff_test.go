// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package filesniff_test

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/filesniff"
)

const (
	pdfType  = "application/pdf"
	docxType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	xlsxType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	odtType  = "application/vnd.oasis.opendocument.text"
	odsType  = "application/vnd.oasis.opendocument.spreadsheet"
)

func zipOf(t *testing.T, entries ...[2]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		w, err := zw.Create(e[0])
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(e[1])); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func peProgram() []byte {
	b := make([]byte, 0x100)
	b[0], b[1] = 'M', 'Z'
	b[0x3C] = 0x80
	copy(b[0x80:], "PE\x00\x00")
	return b
}

func utf16le(s string, bom bool) []byte {
	var out []byte
	if bom {
		out = []byte{0xFF, 0xFE}
	}
	for _, r := range s {
		out = append(out, byte(r), byte(r>>8))
	}
	return out
}

func TestContradictsHoldsNameAndBytesTogether(t *testing.T) {
	t.Parallel()
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	ole := []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1, 0, 0}
	docx := zipOf(t, [2]string{"[Content_Types].xml", "<Types/>"}, [2]string{"word/document.xml", "<w/>"})
	xlsx := zipOf(t, [2]string{"[Content_Types].xml", "<Types/>"}, [2]string{"xl/workbook.xml", "<w/>"})
	odt := zipOf(t, [2]string{"mimetype", odtType}, [2]string{"content.xml", "<c/>"})
	jar := zipOf(t, [2]string{"META-INF/MANIFEST.MF", "Main-Class: A"}, [2]string{"A.class", "x"})
	apk := zipOf(t, [2]string{"AndroidManifest.xml", "x"}, [2]string{"classes.dex", "x"})
	jarAsDocx := zipOf(t, [2]string{"[Content_Types].xml", "<Types/>"}, [2]string{"word/document.xml", "<w/>"}, [2]string{"A.class", "x"})
	heicBox := func(major, compatible string) []byte {
		return []byte("\x00\x00\x00\x18ftyp" + major + "\x00\x00\x00\x00" + compatible + "xxxx")
	}
	for _, tc := range []struct {
		name, mediaType, charset string
		content                  []byte
		want                     bool
	}{
		{"a real pdf", pdfType, "", []byte("%PDF-1.7\n..."), false},
		{"a pdf after a BOM and a newline", pdfType, "", append([]byte("\xef\xbb\xbf\n"), "%PDF-1.4"...), false},
		{"a pdf marker buried in text", pdfType, "", []byte("hello\n%PDF-1.4"), true},
		{"a program named pdf", pdfType, "", peProgram(), true},
		{"a program named png", "image/png", "", peProgram(), true},
		{"a program named txt", "text/plain", "", peProgram(), true},
		{"MZ with no PE header is still a program", "text/plain", "", []byte("MZ Industries quarterly numbers"), true},
		{"an ELF named pdf", pdfType, "", []byte("\x7fELF\x02\x01\x01"), true},
		{"a Mach-O named docx", docxType, "", []byte{0xCF, 0xFA, 0xED, 0xFE, 0, 0}, true},
		{"text named pdf", pdfType, "", []byte("just words"), true},
		{"svg named png", "image/png", "", []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`), true},
		{"html named jpeg", "image/jpeg", "", []byte("<html><script>x</script></html>"), true},
		{"a real png", "image/png", "", png, false},
		{"a real jpeg", "image/jpeg", "", []byte{0xFF, 0xD8, 0xFF, 0xE0, 0}, false},
		{"a png named jpeg", "image/jpeg", "", png, true},
		{"a real tiff", "image/tiff", "", []byte("II*\x00\x08"), false},
		{"a heic by major brand", "image/heic", "", heicBox("heic", "mif1"), false},
		{"a heif by compatible brand", "image/heif", "", heicBox("isom", "mif1"), false},
		{"an mp4", "image/heic", "", heicBox("isom", "mp42"), true},
		{"text named heic", "image/heic", "", []byte("hello world, not an image"), true},
		{"a docx", docxType, "", docx, false},
		{"an xlsx named docx", docxType, "", xlsx, true},
		{"a jar named docx", docxType, "", jar, true},
		{"an apk named docx", docxType, "", apk, true},
		{"a docx that also carries a class", docxType, "", jarAsDocx, true},
		{"a bare zip named docx", docxType, "", zipOf(t, [2]string{"a.txt", "x"}), true},
		{"a truncated zip named docx", docxType, "", []byte("PK\x03\x04rest"), true},
		{"an odt", odtType, "", odt, false},
		{"an odt named ods", odsType, "", odt, true},
		{"a zip without mimetype named odt", odtType, "", zipOf(t, [2]string{"a.txt", "x"}), true},
		{"a plain zip", "application/zip", "", zipOf(t, [2]string{"a.txt", "x"}), false},
		{"a jar named zip", "application/zip", "", jar, true},
		{"an apk named zip", "application/zip", "", apk, true},
		{"an old doc is OLE", "application/msword", "", ole, false},
		{"rtf named doc", "application/msword", "", []byte(`{\rtf1\ansi`), false},
		{"text named doc", "application/msword", "", []byte("hello"), true},
		{"a zip named doc", "application/msword", "", zipOf(t, [2]string{"a.txt", "x"}), true},
		{"text named ppt", "application/vnd.ms-powerpoint", "", []byte("hello"), true},
		{"a csv saved as xls", "application/vnd.ms-excel", "", []byte("a;b\n1;2\n"), false},
		{"html saved as xls", "application/vnd.ms-excel", "", []byte("<html><table></table></html>"), false},
		{"an xlsx saved as xls", "application/vnd.ms-excel", "", xlsx, false},
		{"a jar named xls", "application/vnd.ms-excel", "", jar, true},
		{"a program named xls", "application/vnd.ms-excel", "", peProgram(), true},
		{"an msg is OLE", "application/vnd.ms-outlook", "", ole, false},
		{"text named msg", "application/vnd.ms-outlook", "", []byte("hello"), true},
		{"rtf", "application/rtf", "", []byte(`{\rtf1\ansi`), false},
		{"a csv", "text/csv", "", []byte("name,city\nZoë,Köln\n"), false},
		{"utf-16 text with a BOM", "text/plain", "", utf16le("hi there", true), false},
		{"utf-16 csv without a BOM and its charset", "text/csv", "utf-16le", utf16le("name,city\r\nZoë,Köln\r\n", false), false},
		{"utf-16 csv without a BOM and no charset", "text/csv", "", utf16le("name,city\r\n", false), true},
		{"binary claiming utf-16", "text/csv", "utf-16le", []byte("\x00\x01\x02\x03\x04\x05\x06\x07\x08\x09"), true},
		{"a binary named csv", "text/csv", "", []byte("a\x00\x01\x02b"), true},
		{"an empty file", pdfType, "", nil, false},
		{"a type with no rule", "application/x-unknown", "", []byte("anything"), false},
	} {
		got, err := filesniff.Contradicts(tc.mediaType, tc.charset, bytes.NewReader(tc.content))
		if err != nil || got != tc.want {
			t.Errorf("%s: Contradicts(%s) = (%v, %v), want %v", tc.name, tc.mediaType, got, err, tc.want)
		}
	}
}

func TestContradictsRewindsTheReader(t *testing.T) {
	t.Parallel()
	for name, content := range map[string][]byte{
		"text": []byte(strings.Repeat("a", 3000)),
		"docx": zipOf(t, [2]string{"[Content_Types].xml", "<Types/>"}, [2]string{"word/document.xml", "<w/>"}),
	} {
		r := bytes.NewReader(content)
		mediaType := "text/plain"
		if name == "docx" {
			mediaType = docxType
		}
		if _, err := filesniff.Contradicts(mediaType, "", r); err != nil {
			t.Fatal(err)
		}
		if r.Len() != len(content) {
			t.Errorf("%s: %d bytes left after the check, want all %d", name, r.Len(), len(content))
		}
	}
}

func TestDeclaredCharset(t *testing.T) {
	t.Parallel()
	if got := filesniff.DeclaredCharset("text/csv; charset=UTF-16LE"); got != "UTF-16LE" {
		t.Errorf("DeclaredCharset = %q, want UTF-16LE", got)
	}
	if got := filesniff.DeclaredCharset("not a type;;"); got != "" {
		t.Errorf("DeclaredCharset of garbage = %q, want empty", got)
	}
}
