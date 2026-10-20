// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftfloor

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// Body joins a floor draft under its greeting, a blank line between each part,
// and skips a paragraph that is empty.
//
// German alone carries the greeting's sentence on in lower case ("Hallo Anna,"
// then "ich ..."), and only in the paragraph right under it. Every later
// paragraph, and every paragraph in another language, opens on a capital.
func Body(lang textlang.Lang, greeting string, paragraphs ...string) string {
	parts := []string{greeting}
	for _, paragraph := range paragraphs {
		if paragraph == "" {
			continue
		}
		if len(parts) > 1 || langOrDefault(lang) != textlang.German {
			paragraph = capitalized(paragraph)
		}
		parts = append(parts, paragraph)
	}
	return strings.Join(parts, "\n\n")
}

func capitalized(paragraph string) string {
	first, size := utf8.DecodeRuneInString(paragraph)
	return string(unicode.ToUpper(first)) + paragraph[size:]
}
