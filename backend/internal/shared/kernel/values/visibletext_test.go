// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import "testing"

// The invisible characters are named by code point, so the source stays legible.
func r(code rune) string { return string(code) }

func TestHasVisibleText(t *testing.T) {
	cases := map[string]bool{
		"":                                      false,
		"   ":                                   false,
		"\t\n":                                  false,
		r(0x00A0):                               false,
		r(0x200B):                               false,
		" " + r(0x200D) + r(0x2060) + r(0xFEFF): false,
		"\a":                                    false,
		r(0x034F):                               false,
		r(0x3164):                               false,
		r(0xFFA0) + r(0x115F) + r(0x1160):       false,
		r(0x2800):                               false,
		r(0xE000):                               false,
		r(0x0378):                               false,
		"a":                                     true,
		"e" + r(0x0301):                         true,
		r(0x200B) + "A" + r(0x200B):             true,
		"Đội bán hàng":                          true,
		r(0x2800) + "A":                         true,
	}
	for in, want := range cases {
		if got := HasVisibleText(in); got != want {
			t.Errorf("HasVisibleText(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestWithoutZeroWidthKeepsTheJoinersARealWordNeeds(t *testing.T) {
	if got := WithoutZeroWidth(r(0x200B) + "Alpha" + r(0x2060) + r(0xFEFF) + r(0x3164)); got != "Alpha" {
		t.Fatalf("zero-width runes survived: %q", got)
	}
	persian := "می" + r(0x200C) + "خواهم"
	if got := WithoutZeroWidth(persian); got != persian {
		t.Fatalf("a zero-width non-joiner was dropped: %q", got)
	}
}
