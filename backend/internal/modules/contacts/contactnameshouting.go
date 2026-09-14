// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Display-name spellings that belong to the sending CLIENT rather than to the
// contact.
//
// A phone address book exports "QUANG NGUYEN"; a corporate directory appends an
// office code as "Hang Tran/VNM". Neither is how the human writes their own
// name, and both arrive in the same header field as the spellings that ARE
// theirs — "Ronan McDonald", "van Dijk". So each reading here has to say why the
// string is a convention and not a choice, and abstain when it cannot tell.

import (
	"strings"
	"unicode"
)

// unshout rewrites a display name typed entirely in capitals — "QUANG NGUYEN",
// the spelling a phone address book and several mail clients produce. Shouting
// is a property of the WHOLE string, so the decision is made once over every
// token: a name with any lowercase in it has chosen its own spelling, and one
// all-caps token inside it is an acronym or an initialism ("Ronan McDonald",
// "Anna MBA Weber") that a per-token rule would quietly fold.
//
// Caseless scripts are untouched for free — 寺田文哉 has no uppercase to find, so
// isShouted reports false and the tokens come back as they arrived.
func unshout(tokens []string) []string {
	if !isShouted(tokens) {
		return tokens
	}
	folded := make([]string, 0, len(tokens))
	for _, token := range tokens {
		folded = append(folded, titleCaseToken(strings.ToLower(token)))
	}
	return folded
}

// isShouted reports whether every cased letter is uppercase AND some token is
// long enough for that to be a choice. A single letter says nothing — "J SMITH"
// shouts, "J" alone is an initial — so at least one token of two or more letters
// must be present before the name is called shouted.
func isShouted(tokens []string) bool {
	longEnough := false
	for _, token := range tokens {
		letters := 0
		for _, r := range token {
			if unicode.IsLower(r) {
				return false
			}
			if unicode.IsLetter(r) {
				letters++
			}
		}
		if letters >= 2 {
			longEnough = true
		}
	}
	return longEnough
}

// withoutSlashedUnit cuts an office or country code written straight onto the
// name with no spaces — "Hang Tran/VNM", "Marco Reus/DE". The spaced " / " form
// is an affiliationSeparator already; this one cannot join that list, because a
// bare "/" would also cut a name that legitimately contains one.
//
// So the tail has to look like a unit code rather than a name: short, or shouted.
// A longer mixed-case tail is left alone — "Anna/Maria Weber" keeps both halves,
// since nothing here can tell that slash from a compound given name.
func withoutSlashedUnit(name string) string {
	before, after, found := strings.Cut(name, "/")
	if !found {
		return name
	}
	head, tail := strings.TrimSpace(before), strings.TrimSpace(after)
	if head == "" || tail == "" {
		return name
	}
	if len([]rune(tail)) > maxUnitCodeRunes && !isShouted(strings.Fields(tail)) {
		return name
	}
	return head
}

// maxUnitCodeRunes bounds the tail withoutSlashedUnit will read as a unit code
// rather than a name. Three letters covers the ISO country and office codes this
// appears as; four admits the occasional team abbreviation.
const maxUnitCodeRunes = 4
