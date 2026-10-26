// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"slices"
	"testing"
)

// The stranger check looks a sender up in contact_email and counterparty_email,
// which may hold either spelling of a domain. A punycode sender must also be
// looked up in its Unicode form, or a contact stored in Unicode reads as a
// stranger. The local helper used to give only the typed and IDNA forms.
func TestAStrangerLookupCarriesBothSpellingsOfTheDomain(t *testing.T) {
	for _, sender := range []string{"Anna@xn--bcher-kva.example", "Anna@Bücher.example"} {
		got := addressSpellings(sender)
		for _, want := range []string{"anna@xn--bcher-kva.example", "anna@bücher.example"} {
			if !slices.Contains(got, want) {
				t.Errorf("addressSpellings(%q) = %v, lacks %q", sender, got, want)
			}
		}
		if !slices.Equal(storedSpellings(sender), got) {
			t.Errorf("storedSpellings(%q) = %v, want the same set as addressSpellings", sender, storedSpellings(sender))
		}
	}
}
