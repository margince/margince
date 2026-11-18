// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/filesniff"
)

func TestAnUploadIsStoredUnderTheTypeTheTableNames(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, declared, filename, want string
	}{
		{"a name and a type that agree", "application/pdf", "quote.pdf", "application/pdf"},
		{"the extension is read case-blind", "application/pdf", "report.PDF", "application/pdf"},
		{"case and parameters are dropped from the type", "Text/Plain; charset=UTF-8", "notes.txt", "text/plain"},
		{"an alias is stored as its canonical type", "text/rtf", "terms.rtf", "application/rtf"},
		{"the declared type stands when the extension is not in the table", "application/pdf", "invoice.exe", "application/pdf"},
		{"Windows' spreadsheet type on a .csv is stored as CSV", "application/vnd.ms-excel", "export.csv", "text/csv"},
		{"an extension names the type over another accepted one", "image/png", "scan.pdf", "application/pdf"},
		{"image/jpg is JPEG", "image/jpg", "photo.jpg", "image/jpeg"},
		{"image/jpg on a .jpeg is JPEG", "image/jpg", "photo.jpeg", "image/jpeg"},
		{"application/csv is CSV", "application/csv", "export.csv", "text/csv"},
		{"text/x-csv is CSV", "text/x-csv", "export", "text/csv"},
		{"octet-stream is resolved by extension", "application/octet-stream", "mail.msg", "application/vnd.ms-outlook"},
		{"an empty type is resolved by extension", "", "README.md", "text/markdown"},
		{"a name with no extension rests on its type", "text/plain", "notes", "text/plain"},
		{"a TIFF scan", "", "scan.tif", "image/tiff"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := resolveAttachmentType(tc.declared, tc.filename, tc.filename)
			if err != nil || got != tc.want {
				t.Errorf("resolveAttachmentType(%q, %q) = (%q, %v), want %q", tc.declared, tc.filename, got, err, tc.want)
			}
		})
	}
}

func TestAFileOutsideTheTableIsRefused(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, declared, filename string
	}{
		{"an svg runs script in an image viewer", "image/svg+xml", "logo.svg"},
		{"an accepted extension does not rescue a refused type", "image/svg+xml", "logo.pdf"},
		{"no type and an unknown extension", "", "setup.exe"},
		{"no type and no extension", "", "README"},
		{"octet-stream and an unknown extension", "application/octet-stream", "setup.exe"},
		{"a malformed type", "application/pdf; =", "quote.pdf"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := resolveAttachmentType(tc.declared, tc.filename, tc.filename)
			if _, ok := errors.AsType[*UnsupportedFileTypeError](err); !ok {
				t.Fatalf("resolveAttachmentType(%q, %q) = (%q, %v), want an UnsupportedFileTypeError", tc.declared, tc.filename, got, err)
			}
		})
	}
}

func TestTheRefusalNamesTheFileAndWhatIsAccepted(t *testing.T) {
	t.Parallel()
	_, err := resolveAttachmentType("image/svg+xml", "logo.svg", "logo.svg")
	var refusal *UnsupportedFileTypeError
	if !errors.As(err, &refusal) {
		t.Fatalf("an SVG upload → %v, want an UnsupportedFileTypeError", err)
	}
	field, code, message := refusal.FieldFault()
	if field != "file" || code != "unsupported_file_type" {
		t.Errorf("field fault = (%q, %q), want (file, unsupported_file_type)", field, code)
	}
	for _, named := range []string{"logo.svg", "PDF", "Word", "TIFF", "saved emails"} {
		if !strings.Contains(message, named) {
			t.Errorf("the refusal %q does not mention %q", message, named)
		}
	}
}

func TestEveryAcceptedExtensionResolvesToItsOwnRow(t *testing.T) {
	t.Parallel()
	extensions := AcceptedAttachmentExtensions()
	if len(extensions) != len(attachmentTypes) {
		t.Fatalf("%d extensions listed for %d rows", len(extensions), len(attachmentTypes))
	}
	seen := map[string]bool{}
	for i, ext := range extensions {
		if seen[ext] {
			t.Errorf("%s is listed twice", ext)
		}
		seen[ext] = true
		want := attachmentTypes[i].mediaType
		for _, declared := range []string{"", want} {
			if got, err := resolveAttachmentType(declared, "file"+ext, "file"+ext); err != nil || got != want {
				t.Errorf("a %s file declared %q resolves to (%q, %v), want %q", ext, declared, got, err, want)
			}
		}
	}
}

func TestEveryAcceptedTypeHasAByteRule(t *testing.T) {
	t.Parallel()
	for _, kind := range attachmentTypes {
		if !filesniff.Knows(kind.mediaType) {
			t.Errorf("%s (%s) has no byte rule, so a renamed file would pass as it", kind.extension, kind.mediaType)
		}
	}
}

func TestAnUploadWhoseBytesContradictItsNameIsRefused(t *testing.T) {
	t.Parallel()
	program := make([]byte, 0x100)
	copy(program, "MZ")
	program[0x3C] = 0x80
	copy(program[0x80:], "PE\x00\x00")
	for _, tc := range []struct {
		name, filename, declared string
		content                  []byte
		refused                  bool
	}{
		{"a program named pdf", "disguised.pdf", "application/pdf", program, true},
		{"a program with no declared type", "disguised.pdf", "", program, true},
		{"svg named png", "picture.png", "image/png", []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), true},
		{"a real pdf", "quote.pdf", "application/pdf", []byte("%PDF-1.7 x"), false},
		{"a docx", "terms.docx", "", []byte("PK\x03\x04rest"), false},
		{"a text note", "note.txt", "text/plain", []byte("Köln"), false},
	} {
		in := AttachmentInput{Filename: tc.filename, ContentType: tc.declared, Content: bytes.NewReader(tc.content)}
		_, err := acceptUploadedFile(in)
		_, isRefusal := errors.AsType[*UnsupportedFileTypeError](err)
		if isRefusal != tc.refused || (!tc.refused && err != nil) {
			t.Errorf("%s: acceptUploadedFile error = %v, refused = %v, want refused = %v", tc.name, err, isRefusal, tc.refused)
		}
	}
}
