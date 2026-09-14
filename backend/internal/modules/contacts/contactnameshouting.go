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

// isShouted reports whether the name was typed in capitals. That needs POSITIVE
// evidence, not merely the absence of lowercase: a caseless script has neither
// case, and answering "shouted" for 長谷川一郎 would let unshout rewrite it and
// withoutSlashedUnit delete it as a unit code.
//
// So two things must hold. No cased letter may be lowercase, and at least one
// token must carry two or more UPPERCASE letters — enough for the capitals to be
// a choice rather than an initial. "J SMITH" shouts; "J" alone does not, and
// 長谷川一郎 does not.
func isShouted(tokens []string) bool {
	shouting := false
	for _, token := range tokens {
		uppers := 0
		for _, r := range token {
			if unicode.IsLower(r) {
				return false
			}
			if unicode.IsUpper(r) {
				uppers++
			}
		}
		if uppers >= 2 {
			shouting = true
		}
	}
	return shouting
}

// withoutSlashedUnit cuts an office or country code written straight onto the
// name with no spaces — "Hang Tran/VNM", "Marco Reus/DE". The spaced " / " form
// is an affiliationSeparator already; this one cannot join that list, because a
// bare "/" would also cut a name that legitimately contains one.
//
// Being SHORT is not enough to be a unit code: "Jane Smith/Lee" is a surname
// after a slash, and deleting it would lose half the name with full confidence.
// The tail must also be written as a code rather than as a word — in capitals,
// like every ISO country and office code this appears as.
//
// The cut is taken at the LAST slash, so a name that legitimately contains one
// keeps it while still shedding a trailing code: "Anna/Maria Weber/DE" becomes
// "Anna/Maria Weber", and "Anna/Maria Weber" is left entirely alone.
func withoutSlashedUnit(name string) string {
	cut := strings.LastIndex(name, "/")
	if cut < 0 {
		return name
	}
	head, tail := strings.TrimSpace(name[:cut]), strings.TrimSpace(name[cut+1:])
	if head == "" || tail == "" {
		return name
	}
	if len([]rune(tail)) > maxUnitCodeRunes || !isShouted(strings.Fields(tail)) {
		return name
	}
	return head
}

// maxUnitCodeRunes bounds the tail withoutSlashedUnit will read as a unit code
// rather than a name. Three letters covers the ISO country and office codes this
// appears as; four admits the occasional team abbreviation.
const maxUnitCodeRunes = 4
