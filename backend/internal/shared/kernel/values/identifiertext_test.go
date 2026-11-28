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
		"jo" + r(0x202E) + "s" + r(0x00E9) + "@x.com",
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

// A typed domain that hides a character between visible ones is refused. That
// is how one domain is made to pass for another.
func TestATypedDomainHidingACharacterInsideIsRefused(t *testing.T) {
	if got, err := ParseDomain("Exa" + r(0x200B) + "mple.com"); err == nil {
		t.Errorf("ParseDomain = %q, want a refusal", got)
	}
	if got, err := ParseDomain(r(0xFEFF) + "Example.com" + r(0x200F)); err != nil || got.String() != "example.com" {
		t.Errorf("ParseDomain with padding = %q, %v; want example.com", got, err)
	}
}

// Received text is cleaned whole, controls included, so a value read from a
// mail always meets the database check.
func TestReceivedTextDropsEveryHiddenCharacter(t *testing.T) {
	if got := IdentifierText("a\tb" + r(0x200B) + "c" + r(0x0085) + "@x.com"); got != "abc@x.com" {
		t.Errorf("IdentifierText = %q, want abc@x.com", got)
	}
}

// A number keeps the separators the frontend mirrors, so a character that draws
// nothing is refused there rather than dropped, and both sides agree.
func TestANumberWithACharacterThatDrawsNothingIsRefused(t *testing.T) {
	if got, err := ParsePhone(r(0x202A) + "+49 30 1234567" + r(0x202C)); err == nil {
		t.Errorf("ParsePhone = %q, want a refusal", got)
	}
}
