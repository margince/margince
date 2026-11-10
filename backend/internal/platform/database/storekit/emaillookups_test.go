// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"slices"
	"testing"
)

func TestEitherSpellingLooksUpBoth(t *testing.T) {
	t.Parallel()
	want := []string{"anna@bücher.example", "anna@xn--bcher-kva.example"}
	for _, address := range append(want, " ANNA@BÜCHER.EXAMPLE ") {
		got := EmailLookups(address)
		for _, spelled := range want {
			if !slices.Contains(got, spelled) {
				t.Errorf("EmailLookups(%q) = %v, missing %q", address, got, spelled)
			}
		}
	}
}

func TestAnAddressIsLookedUpAsItWasTyped(t *testing.T) {
	t.Parallel()
	if got := EmailLookups("Ada@Example.com."); !slices.Contains(got, "ada@example.com.") {
		t.Errorf("EmailLookups = %v, want the typed spelling a row may store", got)
	}
	if got := EmailLookups("", "  "); len(got) != 0 {
		t.Errorf("EmailLookups of nothing = %v, want none", got)
	}
}
