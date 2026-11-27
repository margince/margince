// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// IdentifierText spells an address or domain the way a key compares it, typed
// by a user or read from a mail. Characters that draw nothing are dropped before
// the trim, so the trim reaches padding they hid. Accents are composed (NFC), so
// one address typed two ways is one value. The is_identifier_text CHECK refuses
// any other spelling.
func IdentifierText(raw string) string {
	return norm.NFC.String(strings.TrimFunc(strings.Map(dropHidden, raw), unicode.IsSpace))
}

// typedIdentifier is IdentifierText for a domain a user typed: padding is
// dropped, and a hidden character between visible ones is reported. A host that
// hides one inside is how one domain is made to pass for another. An address
// is cleaned whole instead, as a mailbox is reached by its cleaned spelling.
func typedIdentifier(raw string) (string, bool) {
	inner := strings.TrimFunc(raw, func(r rune) bool { return unicode.IsSpace(r) || hiddenRune(r) })
	if strings.ContainsFunc(inner, hiddenRune) {
		return "", false
	}
	return norm.NFC.String(inner), true
}

// hiddenRune is a character that draws nothing: a format character, a filler
// or any control, matching drop_hidden_characters in the database.
func hiddenRune(r rune) bool {
	return unicode.Is(unicode.Cf, r) || fillerRunes[r] || unicode.IsControl(r)
}

func dropHidden(r rune) rune {
	if hiddenRune(r) {
		return -1
	}
	return r
}
