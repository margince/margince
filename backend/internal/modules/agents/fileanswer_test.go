// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"encoding/json"
	"testing"
)

// A text file reads back as text as well, so a model need not decode base64 by
// hand. Bytes that are not UTF-8, or not text, answer as base64 alone.
func TestATextFileAnswersItsTextBesideItsBytes(t *testing.T) {
	for name, tc := range map[string]struct {
		contentType string
		content     []byte
		want        string
	}{
		"UTF-8 text":       {"text/plain; charset=utf-8", []byte("Schlüssel liegt beim Pförtner"), "Schlüssel liegt beim Pförtner"},
		"text, not UTF-8":  {"text/plain", []byte{'P', 0xf6, 'r'}, ""},
		"a file, not text": {"application/pdf", []byte("%PDF-1.7"), ""},
	} {
		raw, err := FileAnswer("handover.txt", tc.contentType, tc.content)
		if err != nil {
			t.Fatal(err)
		}
		var got struct {
			ContentText *string `json:"content_text"`
			SizeBytes   int     `json:"size_bytes"`
		}
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatal(err)
		}
		if (tc.want == "") != (got.ContentText == nil) || (got.ContentText != nil && *got.ContentText != tc.want) {
			t.Errorf("%s answered %s, want content_text %q", name, raw, tc.want)
		}
		if got.SizeBytes != len(tc.content) {
			t.Errorf("%s answered size %d, want %d", name, got.SizeBytes, len(tc.content))
		}
	}
}
