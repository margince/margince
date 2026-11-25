// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// IdentifierText spells an address or domain the way a key compares it, typed
// by a user or read from a mail. Characters that draw nothing are dropped before the trim, so the trim
// reaches padding they hid. Accents are composed (NFC), so one address typed two
// ways is one value. The is_identifier_text CHECK refuses any other spelling.
func IdentifierText(raw string) string {
	visible := strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Cf, r) || fillerRunes[r] || (unicode.IsControl(r) && !unicode.IsSpace(r)) {
			return -1
		}
		return r
	}, raw)
	return norm.NFC.String(strings.TrimFunc(visible, unicode.IsSpace))
}

// HasDirectionOverride reports a bidirectional embedding, override or isolate.
// One in a name makes it display in an order other than the one stored.
func HasDirectionOverride(s string) bool {
	return strings.ContainsFunc(s, func(r rune) bool {
		return (r >= 0x202A && r <= 0x202E) || (r >= 0x2066 && r <= 0x2069)
	})
}
