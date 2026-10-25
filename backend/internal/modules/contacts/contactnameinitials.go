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
// stored full name. It does so only when capture derived that name from the
// address. At least one header was read, none named the address, and the name
// equals the guess. A header "DV" folds to the same "Dv", so the
// string alone proves nothing.
func initialsDisplayReplacement(fullName string, headerNames []string, email string) (string, bool) {
	if len(headerNames) == 0 {
		return "", false
	}
	for _, header := range headerNames {
		if _, named := headerName(header); named {
			return "", false
		}
	}
	local, tokens, ok := readableLocalPart(email)
	if !ok || !readsAsInitials(tokens) || fullName != addressGuessName(tokens) {
		return "", false
	}
	return local, true
}
