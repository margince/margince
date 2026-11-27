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

func TestADomainDropsPaddingAndANumberRefusesWhatDrawsNothing(t *testing.T) {
	if got, err := ParseDomain(r(0xFEFF) + "Example.com" + r(0x200F)); err != nil || got.String() != "example.com" {
		t.Errorf("ParseDomain = %q, %v; want example.com", got, err)
	}
	if got, err := ParseDomain("exa" + r(0x200B) + "mple.com"); err == nil {
		t.Errorf("ParseDomain kept %q, want a refusal: a hidden rune inside makes a look-alike host", got)
	}
	if got, err := ParsePhone(r(0x202A) + "+49 30 1234567" + r(0x202C)); err == nil {
		t.Errorf("ParsePhone kept %q, want a refusal: a number holds digits and the separators the browser lists", got)
	}
}
