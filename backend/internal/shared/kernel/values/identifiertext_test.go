// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import "testing"

// Every spelling a reader sees as one address parses to one value.
func TestAnAddressTypedTwoWaysIsOneAddress(t *testing.T) {
	composed := "jos" + r(0x00E9) + "@x.com"
	for _, typed := range []string{
		composed,
		"jos" + "e" + r(0x0301) + "@x.com",
		r(0x200B) + composed,
		"jo" + r(0x200B) + "s" + r(0x00E9) + "@x.com",
		composed + r(0xFEFF),
		r(0x180E) + composed,
		"jo" + r(0x202E) + "s" + r(0x00E9) + "@x.com",
		composed + r(0x200E),
		r(0x00A0) + composed + " ",
	} {
		got, err := ParseEmail(typed)
		if err != nil || got.String() != composed {
			t.Errorf("ParseEmail(%q) = %q, %v; want %q", typed, got, err, composed)
		}
	}
}

func TestADomainDropsWhatDrawsNothing(t *testing.T) {
	if got, err := ParseDomain(r(0xFEFF) + "Exa" + r(0x200B) + "mple.com" + r(0x200F)); err != nil || got.String() != "example.com" {
		t.Errorf("ParseDomain = %q, %v; want example.com", got, err)
	}
}

// A number keeps the separators the frontend mirrors, so a character that draws
// nothing is refused there rather than dropped, and both sides agree.
func TestANumberWithACharacterThatDrawsNothingIsRefused(t *testing.T) {
	if got, err := ParsePhone(r(0x202A) + "+49 30 1234567" + r(0x202C)); err == nil {
		t.Errorf("ParsePhone = %q, want a refusal", got)
	}
}

func TestADirectionOverrideIsSeenAndAnAccentIsNot(t *testing.T) {
	for in, want := range map[string]bool{
		"Ada Lovelace":                 false,
		"Jos" + "e" + r(0x0301):        false,
		"Đội bán hàng":                 false,
		"Ada" + r(0x200D) + "x":        false,
		"Ada " + r(0x202E) + "gnp.exe": true,
		r(0x2066) + "Ada" + r(0x2069):  true,
		"Ada" + r(0x202A):              true,
	} {
		if got := HasDirectionOverride(in); got != want {
			t.Errorf("HasDirectionOverride(%q) = %v, want %v", in, got, want)
		}
	}
}
