// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"slices"
	"testing"
)

func TestBothSpellingsOfADomainAreOneIdentityKey(t *testing.T) {
	t.Parallel()
	for _, address := range []string{
		"anna@bücher.example",
		" ANNA@BÜCHER.EXAMPLE ",
		"anna@xn--bcher-kva.example",
		"Anna@XN--BCHER-KVA.example",
	} {
		if got := IdentityEmail(address); got != "anna@xn--bcher-kva.example" {
			t.Errorf("IdentityEmail(%q) = %q, want the ASCII spelling", address, got)
		}
	}
}

func TestAnAddressWithNoDomainIsOnlyCaseFolded(t *testing.T) {
	t.Parallel()
	if got := IdentityEmail(" Anna "); got != "anna" {
		t.Errorf("IdentityEmail = %q, want %q", got, "anna")
	}
	if got := EmailSpellings(""); got != nil {
		t.Errorf("EmailSpellings of nothing = %v, want none", got)
	}
}

func TestEitherSpellingListsBoth(t *testing.T) {
	t.Parallel()
	want := []string{"anna@bücher.example", "anna@xn--bcher-kva.example"}
	for _, address := range want {
		got := EmailSpellings(address)
		for _, spelled := range want {
			if !slices.Contains(got, spelled) {
				t.Errorf("EmailSpellings(%q) = %v, missing %q", address, got, spelled)
			}
		}
	}
	if got := EmailSpellings("ada@example.com"); !slices.Equal(got, []string{"ada@example.com"}) {
		t.Errorf("an ASCII address has spellings %v, want only itself", got)
	}
}
