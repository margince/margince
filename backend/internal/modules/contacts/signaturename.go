// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Mapping the name parts a signature reading reports onto first_name and
// last_name. The model says which words are the family name and which the given
// names, and the order it saw them in. This file is the only place that turns
// those parts into the two columns. No caller decides by word position.

import (
	"strings"
	"unicode"

	"github.com/margince/margince/backend/internal/platform/evidencefold"
)

// The orders a signature reading may report.
const (
	SignatureOrderGivenFirst  = "given_first"
	SignatureOrderFamilyFirst = "family_first"
	SignatureOrderUnclear     = "unclear"
)

// SignatureNameParts is the name as the signature reading reports it.
type SignatureNameParts struct {
	Family   string
	Given    string
	Middle   string
	Order    string
	Evidence string
}

// SignatureName is the mapped result. Display is the words in the order the
// signature prints them.
type SignatureName struct {
	First   string
	Last    string
	Display string
}

// MapSignatureName answers the columns and display for the reported parts, and
// false when nothing may be written. Both halves are written together or not at
// all. They are written only when the parts read back as one contiguous run of
// the evidence, in the printed order the reading reported.
func MapSignatureName(p SignatureNameParts) (SignatureName, bool) {
	family, given, middle := strings.Join(strings.Fields(p.Family), " "), strings.Join(strings.Fields(p.Given), " "), strings.Join(strings.Fields(p.Middle), " ")
	if family == "" || given == "" {
		return SignatureName{}, false
	}
	var printed []string
	switch p.Order {
	case SignatureOrderFamilyFirst:
		printed = []string{family, middle, given}
	case SignatureOrderGivenFirst:
		printed = []string{given, middle, family}
	default:
		return SignatureName{}, false
	}
	var words []string
	for _, part := range printed {
		words = append(words, strings.Fields(part)...)
	}
	for _, word := range words {
		if !isWordLike(word) {
			return SignatureName{}, false
		}
	}
	display := strings.Join(words, " ")
	if isMixedScriptSpoof(display) || !containsRun(nameWords(evidencefold.Normalize(p.Evidence)), nameWords(evidencefold.Normalize(display))) {
		return SignatureName{}, false
	}
	return SignatureName{First: given, Last: family, Display: display}, true
}

// nameWords splits folded text into the words a name is made of, dropping the
// punctuation that surrounds a name in a signature line.
func nameWords(folded string) []string {
	return strings.FieldsFunc(folded, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsMark(r) && r != '\'' && r != '-'
	})
}

// containsRun reports whether run appears in words as consecutive entries.
func containsRun(words, run []string) bool {
	if len(run) == 0 {
		return false
	}
	for start := 0; start+len(run) <= len(words); start++ {
		matched := true
		for i, want := range run {
			if words[start+i] != want {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

// SplitStoredName splits a full name for a caller that holds nothing else. One
// word is a first name. Two words split by position, the documented convention
// for a display string. Three words split only where splitFirstLast finds a
// particle surname, because nothing else in the string says which word is the
// given name. Four or more are not split.
func SplitStoredName(full string) (first, last string, ok bool) {
	tokens := strings.Fields(full)
	switch len(tokens) {
	case 0:
		return "", "", false
	case 1:
		return tokens[0], "", true
	case 2:
		return tokens[0], tokens[1], true
	default:
		return splitFirstLast(tokens)
	}
}
