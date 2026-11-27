// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// IdentifierText is the stored spelling of an address or domain. Runes that draw
// nothing go before the trim, so the trim reaches padding they hid. NFC composes
// accents. The is_identifier_text CHECK refuses any other spelling.
func IdentifierText(raw string) string {
	visible := strings.Map(func(r rune) rune {
		if drawsNothing(r) {
			return -1
		}
		return r
	}, raw)
	return norm.NFC.String(strings.TrimFunc(visible, unicode.IsSpace))
}

// drawsNothing reports a format character, a filler or a control that is not
// white space. Such a rune changes a stored value and shows no change.
func drawsNothing(r rune) bool {
	return unicode.Is(unicode.Cf, r) || fillerRunes[r] || (unicode.IsControl(r) && !unicode.IsSpace(r))
}

// hidesInside reports a rune that draws nothing between visible characters.
// Padding is dropped; inside a host it makes a look-alike of another host.
func hidesInside(raw string) bool {
	core := strings.TrimFunc(raw, func(r rune) bool { return unicode.IsSpace(r) || drawsNothing(r) })
	return strings.ContainsFunc(core, drawsNothing)
}
