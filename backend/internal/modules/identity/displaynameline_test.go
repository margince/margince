// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import "testing"

// A joiner inside a word is kept: the Devanagari conjunct in the last case needs it.
func TestADisplayNameIsStoredAsOneVisibleLine(t *testing.T) {
	t.Parallel()
	for typed, want := range map[string]string{
		"  Ada   Lovelace  ":             "Ada Lovelace",
		"Ada\nLovelace":                  "Ada Lovelace",
		"Ada\tLovelace":                  "Ada Lovelace",
		"Ada\u200b\u200b Lovelace":       "Ada Lovelace",
		"\ufeffAda Lovelace\u2060":       "Ada Lovelace",
		"Ada \u202eecalevoL":             "Ada ecalevoL",
		"Ada\u2066 Lovelace\u2069":       "Ada Lovelace",
		"Ada\u0000\u001b Lovelace":       "Ada Lovelace",
		"Nguyễn Thị Hương":               "Nguyễn Thị Hương",
		"\u0915\u094d\u200d\u0937\u093e": "\u0915\u094d\u200d\u0937\u093e",
	} {
		if got := displayNameLine(typed); got != want {
			t.Errorf("displayNameLine(%q) = %q, want %q", typed, got, want)
		}
	}
}
