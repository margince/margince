// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package salutation

import (
	"strings"
	"unicode"
)

// NamesSigner uses whole words so a short name cannot match inside somebody else's.
func NamesSigner(block, fullName, email string) bool {
	folded := strings.ToLower(block)
	if email != "" && strings.Contains(folded, strings.ToLower(email)) {
		return true
	}
	words := make(map[string]bool)
	for _, word := range strings.FieldsFunc(folded, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	}) {
		words[word] = true
	}
	// Two tokens when the name has two: a surname alone is shared by relatives
	// and colleagues, and the name gate writes from this block.
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
