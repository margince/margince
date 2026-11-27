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

// An erasure hashed before the fold holds the digest of the decomposed spelling.
// A hash cannot be folded later, so a lookup asks for both digests.
func TestAnErasureHashedBeforeTheFoldStillSuppresses(t *testing.T) {
	t.Parallel()
	decomposed, composed := "josé@x.example", "josé@x.example"
	forms := SuppressionForms(EmailLookups(decomposed)...)
	for _, stored := range []string{identifierDigest(decomposed), identifierDigest(composed)} {
		if !slices.Contains(forms, stored) {
			t.Errorf("SuppressionForms(EmailLookups(%q)) = %v, missing the stored digest %s", decomposed, forms, stored)
		}
	}
}
