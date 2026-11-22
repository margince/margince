// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailcopy

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
)

// Snippet bounds a preview without splitting a character or concealing the cut.
func Snippet(text string, limit int) string {
	text = OneLine(text)
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	return textcut.Runes(text, limit) + "…"
}

// WordSnippet is Snippet for a line read whole, such as a subject: the limit
// counts the mark too, and the cut falls between words.
func WordSnippet(text string, limit int) string {
	text = OneLine(text)
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	if limit < 1 {
		return ""
	}
	wider := textcut.Runes(text, limit)
	head := textcut.Runes(wider, limit-1)
	// A word across the cut is dropped whole, unless that keeps under half the
	// limit: a long address after "Re:" would otherwise leave only "Re…".
	if next, _ := utf8.DecodeLastRuneInString(wider); !unicode.IsSpace(next) {
		space := strings.LastIndexFunc(head, unicode.IsSpace)
		if space > 0 && utf8.RuneCountInString(head[:space]) >= limit/2 {
			head = head[:space]
		}
	}
	return strings.TrimRightFunc(head, clauseEnd) + "…"
}

// clauseEnd is a space or clause mark, which would read as the sentence going on.
func clauseEnd(r rune) bool {
	return unicode.IsSpace(r) || r == ',' || r == ';' || r == ':'
}
