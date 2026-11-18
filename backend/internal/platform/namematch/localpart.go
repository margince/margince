// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package namematch

import (
	"strings"
	"unicode"
)

// MaxNameTokens bounds what reads as "first last". Three admits a middle name
// or a particle surname ("Ludwig van Beethoven"). Past that the string carries
// a department, a company or a title, and a split would invent a surname.
const MaxNameTokens = 3

// LocalPartTokens splits a local part into name-shaped words. Dots and
// underscores separate words, digits at either end are dropped, and anything
// else that is not a word is refused: `trung314578` reads as `trung`.
//
// The hyphen stays inside a word, because "anne-marie" and "first-last" look
// alike. complete is false when a field did not read as a word, and the caller
// must then abstain, or it would rename the contact.
func LocalPartTokens(local string) ([]string, bool) {
	fields := strings.FieldsFunc(local, func(r rune) bool {
		return r == '.' || r == '_'
	})
	tokens := make([]string, 0, len(fields))
	for _, field := range fields {
		word := strings.TrimFunc(field, unicode.IsDigit)
		if word == "" || !IsWordLike(word) {
			return nil, false
		}
		tokens = append(tokens, word)
	}
	if len(tokens) > MaxNameTokens {
		return nil, false
	}
	return tokens, true
}

// IsWordLike reports whether a token could be part of a name: letters, plus the
// apostrophes and hyphens real names carry. A token with a digit in the MIDDLE
// (`user2name`) is a handle, not a name.
func IsWordLike(token string) bool {
	letters := 0
	for _, r := range token {
		switch {
		case unicode.IsLetter(r):
			letters++
		case unicode.IsMark(r):
			// A combining diacritic belongs to the letter before it: decomposed
			// "é" is e + U+0301, and refusing the mark would drop the field and
			// rename the contact.
		case r == '\'' || r == '’' || r == '-':
		default:
			return false
		}
	}
	// Punctuation alone is not a word. Without this, "Alice -" reads as a
	// contact whose surname is a hyphen.
	return letters > 0
}
