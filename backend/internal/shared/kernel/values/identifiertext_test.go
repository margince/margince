// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import "testing"

// Every spelling a reader takes for one address parses to one value.
func TestAnAddressTypedTwoWaysIsOneAddress(t *testing.T) {
	composed := "jos" + r(0x00E9) + "@x.com"
	for _, typed := range []string{
		composed,
		"jos" + "e" + r(0x0301) + "@x.com",
		r(0x200B) + composed,
		composed + r(0xFEFF),
		r(0x180E) + composed,
		composed + r(0x200E),
		r(0x00A0) + composed + " ",
	} {
		got, err := ParseEmail(typed)
		if err != nil || got.String() != composed {
			t.Errorf("ParseEmail(%q) = %q, %v; want %q", typed, got, err, composed)
		}
	}
}

// A hidden rune between letters spells a look-alike, so a typed address that
// holds one is refused.
func TestAnAddressWithAHiddenRuneInsideIsRefused(t *testing.T) {
	for _, typed := range []string{
		"jo" + r(0x200B) + "se@x.com",
		"jo" + r(0x202E) + "se@x.com",
		"jose@exa" + r(0x2060) + "mple.com",
	} {
		if got, err := ParseEmail(typed); err == nil {
			t.Errorf("ParseEmail(%q) = %q, want a refusal", typed, got)
		}
	}
}

func TestADomainDropsPaddingAndRefusesAHiddenRuneInside(t *testing.T) {
	if got, err := ParseDomain(r(0xFEFF) + "Example.com" + r(0x200F)); err != nil || got.String() != "example.com" {
		t.Errorf("ParseDomain = %q, %v; want example.com", got, err)
	}
	if got, err := ParseDomain("exa" + r(0x200B) + "mple.com"); err == nil {
		t.Errorf("ParseDomain kept %q, want a refusal: a hidden rune inside makes a look-alike host", got)
	}
}
