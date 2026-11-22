// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailidentity

import (
	"slices"
	"testing"
)

// One mailbox, however a header spells its domain, is one set of spellings:
// the folded ASCII form first and its Unicode display form beside it.
func TestEverySpellingOfOneMailboxIsTheSameSet(t *testing.T) {
	want := []string{"anna@xn--bcher-kva.example", "anna@bücher.example"}
	for _, in := range []string{"Anna@Bücher.example", "anna@xn--bcher-kva.example", " ANNA@BÜCHER.EXAMPLE. "} {
		if got := AddressSpellings(in); !slices.Equal(got, want) {
			t.Errorf("AddressSpellings(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestAnASCIIMailboxHasOneSpelling(t *testing.T) {
	if got := AddressSpellings("Rep@Seller.Example"); !slices.Equal(got, []string{"rep@seller.example"}) {
		t.Errorf("got %v, want the one folded address", got)
	}
}

func TestAnAddressWithNoDomainIsItself(t *testing.T) {
	if got := AddressSpellings(" Rep "); !slices.Equal(got, []string{"rep"}) {
		t.Errorf("got %v", got)
	}
	if got := AddressSpellings("  "); got != nil {
		t.Errorf("a blank address has no spelling, got %v", got)
	}
}

// Only case, surrounding space and the spelling of the domain are folded. The
// dot and plus rules differ per provider, and a wrong guess joins two people.
func TestFoldAddressKeepsPlusTagsDotsAndSubdomainsApart(t *testing.T) {
	const base = "jane.doe@gmail.com"
	for _, other := range []string{
		"jane.doe+news@gmail.com",
		"janedoe@gmail.com",
		"j.a.n.e.doe@gmail.com",
		"jane.doe@mail.gmail.com",
		"jane.doe@googlemail.com",
	} {
		if FoldAddress(other) == FoldAddress(base) {
			t.Errorf("FoldAddress folded %q into %q, want a different address", other, base)
		}
		if SeatAddressKey(other, "") == SeatAddressKey(base, "") {
			t.Errorf("SeatAddressKey folded %q into %q, want a different address", other, base)
		}
	}
	if got := FoldAddress(" Jane.Doe+News@GMAIL.com "); got != "jane.doe+news@gmail.com" {
		t.Errorf("FoldAddress kept %q, want only case and space folded", got)
	}
}
