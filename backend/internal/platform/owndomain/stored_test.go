// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package owndomain

import "testing"

// Every spelling of one address stores one folded domain, the value CoversSQL
// compares. Case, unicode against punycode, fullwidth letters, a decomposed
// umlaut and a trailing dot all fold away.
func TestEverySpellingOfADomainStoresOneFoldedValue(t *testing.T) {
	const want = "xn--bcher-kva.example"
	for _, address := range []string{
		"anna@bücher.example",
		"ANNA@BÜCHER.EXAMPLE",
		"anna@xn--bcher-kva.example",
		"anna@XN--BCHER-KVA.EXAMPLE",
		"anna@ｂücher.example",
		"anna@bücher.example",
		"anna@bücher.example.",
	} {
		got := StoredOfAddress(address)
		if got == nil || *got != want {
			t.Errorf("StoredOfAddress(%q) = %v, want %s", address, deref(got), want)
		}
	}
	for _, address := range []string{"", "no-at-sign", "trailing@"} {
		if got := StoredOfAddress(address); got != nil {
			t.Errorf("StoredOfAddress(%q) = %q, want nothing stored", address, *got)
		}
	}
}

// An own-domain candidate nobody can own stores nothing, so the SQL side drops
// it as New does.
func TestAPublicSuffixStoresNoOwnDomain(t *testing.T) {
	for _, domain := range []string{"co.uk", "com", "", "  "} {
		if got := StoredOwn(domain); got != nil {
			t.Errorf("StoredOwn(%q) = %q, want nothing stored", domain, *got)
		}
	}
	if got := StoredOwn("Bücher.Example"); got == nil || *got != "xn--bcher-kva.example" {
		t.Errorf("StoredOwn(Bücher.Example) = %v, want xn--bcher-kva.example", deref(got))
	}
}

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}
