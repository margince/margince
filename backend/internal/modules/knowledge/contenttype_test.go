// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package knowledge

import (
	"strings"
	"testing"
)

func TestARefusedFileIsNamedByItsKindInPlainWords(t *testing.T) {
	t.Parallel()
	const advice = " Save it as plain text, Markdown, CSV or JSON first."
	for _, c := range []struct {
		name, contentType, filename, want string
	}{
		{"word by its stated type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", "contract.docx", "Word files are not supported."},
		{"word with no stated type", "", "contract.doc", "Word files are not supported."},
		{"word with no extension", "application/msword", "contract", "Word files are not supported."},
		{"excel", "", "budget.xlsx", "Excel files are not supported."},
		{"powerpoint", "", "pitch.pptx", "PowerPoint files are not supported."},
		{"any other ending is named by it", "application/pdf", "handbook.pdf", "PDF files are not supported."},
		{"an executable", "application/x-msdownload", "setup.exe", "EXE files are not supported."},
		{"no ending and an unknown type", "application/zip", "bundle", "Files of this kind are not supported."},
		{"an ending that is not a word", "", "notes.tar%20gz", "Files of this kind are not supported."},
		{"an ending too long to be one", "", "notes." + strings.Repeat("x", 40), "Files of this kind are not supported."},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			media, ok := acceptedType(c.contentType, c.filename)
			if ok {
				t.Fatalf("%s (%s) was accepted as %s", c.filename, c.contentType, media)
			}
			got := (&UnsupportedTypeError{Kind: refusedKind(media, c.filename)}).Error()
			if got != c.want+advice {
				t.Fatalf("refusal = %q, want %q", got, c.want+advice)
			}
		})
	}
}

// A browser types a file by its own registry, so a readable ending is filed as
// what it is whatever type rode along.
func TestAReadableEndingIsFiledAsItsOwnTypeWhateverTheBrowserSent(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ contentType, filename, want string }{
		{"application/vnd.ms-excel", "data.csv", "text/csv"},
		{"text/x-markdown", "notes.md", "text/markdown"},
		{"", "notes.MD", "text/markdown"},
		{"application/octet-stream", "faq.json", "application/json"},
		{"text/plain; charset=utf-8", "readme", "text/plain"},
	} {
		if media, ok := acceptedType(c.contentType, c.filename); !ok || media != c.want {
			t.Errorf("%s sent as %q = %q (accepted %v), want %s", c.filename, c.contentType, media, ok, c.want)
		}
	}
}

func TestAnEmptyFileIsRefusedWithAReasonTheUploaderCanActOn(t *testing.T) {
	t.Parallel()
	field, code, message := (&EmptyDocumentError{}).FieldFault()
	if field != "file" || code != "empty_file" {
		t.Fatalf("fault = %s/%s, want file/empty_file", field, code)
	}
	if message != "The file is empty, so there is nothing to add. Choose a file that has content." {
		t.Fatalf("message = %q", message)
	}
}
