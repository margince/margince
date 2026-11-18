// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package salutation

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
)

// NamesSigner reports whether a signature block is the contact's own. It
// matches whole words, so a short name cannot match inside somebody else's.
// The address alone qualifies a block only when the block prints no other
// address: a second address belongs to the person who signed it. Words inside
// an address name nobody, so "bob@" does not sign Alice's block as Bob.
func NamesSigner(block, fullName, email string) bool {
	folded := strings.ToLower(block)
	if namesAll(wordsOf(printedAddress.ReplaceAllString(folded, " ")), fullName) {
		return true
	}
	email = correspondence.Fold(email)
	return email != "" && strings.Contains(folded, email) && !holdsOtherAddress(folded, email)
}

// NameIsContacts reports whether a name read off a signature is this contact's.
// It must carry their name or the name their address spells, as NamesSigner
// matches it. A name that does neither is another participant's.
func NameIsContacts(read, fullName, email string) bool {
	words := wordsOf(strings.ToLower(read))
	if namesAll(words, fullName) {
		return true
	}
	local, _, _ := strings.Cut(correspondence.Fold(email), "@")
	parts := strings.FieldsFunc(correspondence.LocalPartBase(local), notWordRune)
	if namesAll(words, strings.Join(parts, " ")) {
		return true
	}
	// "mgreven@" names Marcus Greven. A short word inside a longer local part
	// is noise: "ann" sits inside "joanne".
	for word := range words {
		if len(parts) == 1 && len([]rune(word)) >= 4 && strings.Contains(parts[0], word) {
			return true
		}
	}
	return false
}

// namesAll asks for two tokens when the name has two. A surname alone is shared
// by relatives and colleagues, and the name gate writes from what matched.
func namesAll(words map[string]bool, fullName string) bool {
	var tokens, matched int
	for token := range strings.FieldsSeq(strings.ToLower(fullName)) {
		token = strings.Trim(token, ".,;:()<>\"'")
		if token == "" {
			continue
		}
		tokens++
		if words[token] {
			matched++
		}
	}
	return matched > 0 && matched >= min(tokens, 2)
}

func wordsOf(folded string) map[string]bool {
	words := make(map[string]bool)
	for _, word := range strings.FieldsFunc(folded, notWordRune) {
		words[word] = true
	}
	return words
}

func notWordRune(r rune) bool {
	return !unicode.IsLetter(r) && !unicode.IsNumber(r)
}

var printedAddress = regexp.MustCompile(`[\p{L}\p{N}._%+-]+@[\p{L}\p{N}-]+(?:\.[\p{L}\p{N}-]+)+`)

func holdsOtherAddress(folded, email string) bool {
	for _, address := range printedAddress.FindAllString(folded, -1) {
		if address != email {
			return true
		}
	}
	return false
}
