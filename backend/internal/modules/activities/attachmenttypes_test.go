// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"errors"
	"strings"
	"testing"
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
		{"the declared type wins over a disagreeing extension", "application/pdf", "invoice.exe", "application/pdf"},
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
		{"markup runs script in a viewer", "text/html", "invoice.html"},
		{"an svg carries script too", "image/svg+xml", "logo.svg"},
		{"an archive hides what it holds", "application/zip", "bundle.zip"},
		{"an accepted extension does not rescue a refused type", "text/html", "invoice.pdf"},
		{"no type and an unknown extension", "", "setup.exe"},
		{"no type and no extension", "", "README"},
		{"octet-stream and an unknown extension", "application/octet-stream", "page.htm"},
		{"a malformed type", "application/pdf; =", "quote.pdf"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := resolveAttachmentType(tc.declared, tc.filename, tc.filename)
			var refusal *UnsupportedFileTypeError
			if !errors.As(err, &refusal) {
				t.Fatalf("resolveAttachmentType(%q, %q) = (%q, %v), want an UnsupportedFileTypeError", tc.declared, tc.filename, got, err)
			}
		})
	}
}

func TestTheRefusalNamesTheFileAndWhatIsAccepted(t *testing.T) {
	t.Parallel()
	_, err := resolveAttachmentType("text/html", "invoice.html", "invoice.html")
	var refusal *UnsupportedFileTypeError
	if !errors.As(err, &refusal) {
		t.Fatalf("an HTML upload → %v, want an UnsupportedFileTypeError", err)
	}
	field, code, message := refusal.FieldFault()
	if field != "file" || code != "unsupported_file_type" {
		t.Errorf("field fault = (%q, %q), want (file, unsupported_file_type)", field, code)
	}
	for _, named := range []string{"invoice.html", "PDF", "Word", "TIFF", "saved emails"} {
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
