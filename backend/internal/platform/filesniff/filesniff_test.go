// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package filesniff_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/filesniff"
)

const (
	pdfType  = "application/pdf"
	docxType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
)

func peProgram() []byte {
	b := make([]byte, 0x100)
	b[0], b[1] = 'M', 'Z'
	b[0x3C] = 0x80
	copy(b[0x80:], "PE\x00\x00")
	return b
}

func TestContradictsHoldsNameAndBytesTogether(t *testing.T) {
	t.Parallel()
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	ole := []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1, 0, 0}
	for _, tc := range []struct {
		name, mediaType string
		content         []byte
		want            bool
	}{
		{"a real pdf", pdfType, []byte("%PDF-1.7\n..."), false},
		{"a pdf with a leading banner", pdfType, append([]byte("\n\n"), "%PDF-1.4"...), false},
		{"a program named pdf", pdfType, peProgram(), true},
		{"a program named png", "image/png", peProgram(), true},
		{"a program named txt", "text/plain", peProgram(), true},
		{"an ELF named pdf", pdfType, []byte("\x7fELF\x02\x01\x01"), true},
		{"a Mach-O named docx", docxType, []byte{0xCF, 0xFA, 0xED, 0xFE, 0, 0}, true},
		{"text named pdf", pdfType, []byte("just words"), true},
		{"svg named png", "image/png", []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`), true},
		{"html named jpeg", "image/jpeg", []byte("<html><script>x</script></html>"), true},
		{"a real png", "image/png", png, false},
		{"a real jpeg", "image/jpeg", []byte{0xFF, 0xD8, 0xFF, 0xE0, 0}, false},
		{"a png named jpeg", "image/jpeg", png, true},
		{"a real tiff", "image/tiff", []byte("II*\x00\x08"), false},
		{"a real heic", "image/heic", []byte("\x00\x00\x00\x18ftypheic"), false},
		{"a docx is a zip", docxType, []byte("PK\x03\x04\x14\x00"), false},
		{"text named docx", docxType, []byte("hello"), true},
		{"a zip", "application/zip", []byte("PK\x03\x04"), false},
		{"an old doc is OLE", "application/msword", ole, false},
		{"a csv saved as xls", "application/vnd.ms-excel", []byte("a;b\n1;2\n"), false},
		{"a program named xls", "application/vnd.ms-excel", peProgram(), true},
		{"an msg is OLE", "application/vnd.ms-outlook", ole, false},
		{"text named msg", "application/vnd.ms-outlook", []byte("hello"), true},
		{"rtf", "application/rtf", []byte(`{\rtf1\ansi`), false},
		{"a csv", "text/csv", []byte("name,city\nZoë,Köln\n"), false},
		{"utf-16 text", "text/plain", []byte("\xff\xfeh\x00i\x00"), false},
		{"a binary named csv", "text/csv", []byte("a\x00\x01\x02b"), true},
		{"an mz-looking text", "text/plain", []byte("MZ Industries quarterly numbers, with enough text to pass sixty-four bytes."), false},
		{"an empty file", pdfType, nil, false},
		{"a type with no rule", "application/x-unknown", []byte("anything"), false},
	} {
		got, err := filesniff.Contradicts(tc.mediaType, bytes.NewReader(tc.content))
		if err != nil || got != tc.want {
			t.Errorf("%s: Contradicts(%s) = (%v, %v), want %v", tc.name, tc.mediaType, got, err, tc.want)
		}
	}
}

func TestContradictsRewindsTheReader(t *testing.T) {
	t.Parallel()
	r := bytes.NewReader([]byte(strings.Repeat("a", 3000)))
	if _, err := filesniff.Contradicts("text/plain", r); err != nil {
		t.Fatal(err)
	}
	if r.Len() != 3000 {
		t.Errorf("%d bytes left after the check, want all 3000", r.Len())
	}
}
