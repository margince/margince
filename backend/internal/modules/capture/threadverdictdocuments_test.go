// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import "testing"

func TestWhichFilesAThreadVerdictMustRead(t *testing.T) {
	for _, c := range []struct {
		filename, contentType string
		want                  bool
	}{
		{"Cost+ VER 2.xlsx", "application/zip", true},
		{"data.bin", "application/zip", true},
		{"scan", "application/pdf", true},
		{"INV 5582.PDF", "application/octet-stream", true},
		{"payroll.csv", "text/plain; charset=utf-8", true},
		{"notes", "text/plain", true},
		{"invite.ics", "text/plain", false},
		{"logo.png", "image/png", true},
		{"scan.bin", "image/jpeg; charset=binary", true},
		{"image.JPG", "application/octet-stream", true},
		{"passport.png", "image/png", true},
		{"test_result.jpg", "image/jpeg", true},
		{"smime.p7s", "application/octet-stream", false},
	} {
		if got := IsThreadDocument(c.filename, c.contentType); got != c.want {
			t.Errorf("IsThreadDocument(%q, %q) = %v, want %v", c.filename, c.contentType, got, c.want)
		}
	}
}

func TestAMessageWithMoreDocumentsThanTheCapCountsTheRest(t *testing.T) {
	var files []ThreadDocument
	for _, name := range []string{"a.pdf", "invite.ics", "b.pdf", "c.pdf", "d.pdf"} {
		files = append(files, ThreadDocument{Filename: name})
	}
	kept, omitted := threadDocuments(files)
	if len(kept) != MaxThreadDocuments || omitted != 1 {
		t.Errorf("kept %d omitted %d, want %d and 1: the invitation is no document", len(kept), omitted, MaxThreadDocuments)
	}
}
