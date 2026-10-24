// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"strings"
	"unicode"
)

// addressGuessName is the display capture derived from an address alone: its
// tokens title-cased and joined by a space. The repair compares a stored name
// against it to tell capture's guess from a name somebody typed.
func addressGuessName(tokens []string) string {
	cased := make([]string, 0, len(tokens))
	for _, token := range tokens {
		cased = append(cased, titleCaseToken(token))
	}
	return strings.Join(cased, " ")
}

// readsAsInitials reports whether every token has at most two letters. A
// two-letter local part is almost always initials, and abstaining costs a null
// column while guessing writes a wrong name that every greeting repeats.
func readsAsInitials(tokens []string) bool {
	for _, token := range tokens {
		letters := 0
		for _, r := range token {
			if unicode.IsLetter(r) {
				letters++
			}
		}
		if letters > initialsMaxLetters {
			return false
		}
	}
	return len(tokens) > 0
}

const initialsMaxLetters = 2

// maxInitialsDisplayLen bounds the displays the repair reads: three tokens of
// initials plus the separators between them.
const maxInitialsDisplayLen = maxNameTokens*initialsMaxLetters + maxNameTokens - 1

// initialsDisplayReplacement answers the local part that should replace a
// stored full name, only when that name equals the guess capture made from the
// address alone. A header name that reads "Dv" never equals the
// guess for a different address, and a typed name never equals it at all.
func initialsDisplayReplacement(fullName, email string) (string, bool) {
	local, tokens, ok := readableLocalPart(email)
	if !ok || !readsAsInitials(tokens) || fullName != addressGuessName(tokens) {
		return "", false
	}
	return local, true
}
