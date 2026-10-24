// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package evidencefold folds text to the form an evidence snippet is compared
// in. The no-guess gates in compose and the signature name mapper in contacts
// both compare through it. A snippet one accepts, the other reads the same way.
package evidencefold

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// Normalize reduces text to its comparable core. The text is NFC and
// casefolded, typographic quotes and dashes become ASCII, and soft hyphens are
// dropped. Every whitespace run, NBSP included, collapses to one space.
func Normalize(s string) string {
	s = norm.NFC.String(s)
	var b strings.Builder
	b.Grow(len(s))
	space := false
	for _, r := range s {
		switch {
		case unicode.IsSpace(r):
			space = true
			continue
		case r == '­': // soft hyphen: a rendering hint, not content
			continue
		case values.IsInvisibleRune(r):
			// A bidi override or a zero-width space between two letters must not
			// stop a value matching the text that prints it.
			continue
		}
		if space {
			if b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = false
		}
		switch r {
		case '‘', '’', '‚', '′': // ’ ‘ ‚ ′
			b.WriteByte('\'')
		case '“', '”', '„', '«', '»': // “ ” „ « »
			b.WriteByte('"')
		case '–', '—', '−': // en dash, em dash, minus sign
			b.WriteByte('-')
		default:
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}
