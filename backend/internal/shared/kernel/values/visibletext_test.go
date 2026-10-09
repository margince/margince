// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import "testing"

// The invisible characters are named by code point, so the source stays legible.
const (
	zeroWidthSpace  = string(rune(0x200B))
	zeroWidthJoiner = string(rune(0x200D))
	wordJoiner      = string(rune(0x2060))
	byteOrderMark   = string(rune(0xFEFF))
	noBreakSpace    = string(rune(0x00A0))
)

func TestHasVisibleText(t *testing.T) {
	cases := map[string]bool{
		"":             false,
		"   ":          false,
		"\t\n":         false,
		noBreakSpace:   false,
		zeroWidthSpace: false,
		" " + zeroWidthJoiner + wordJoiner + byteOrderMark: false,
		"\a":                                  false,
		"a":                                   true,
		zeroWidthSpace + "A" + zeroWidthSpace: true,
		"Đội bán hàng":                        true,
	}
	for in, want := range cases {
		if got := HasVisibleText(in); got != want {
			t.Errorf("HasVisibleText(%q) = %v, want %v", in, got, want)
		}
	}
}
