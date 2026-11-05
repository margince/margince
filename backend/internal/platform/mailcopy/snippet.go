// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailcopy

import (
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
