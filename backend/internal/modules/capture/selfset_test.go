// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// What counts as "that is also me", at the boundaries where a set membership
// test usually goes wrong.

import "testing"

func TestASelfSetRecognisesTheSeatsOwnAddressesAndNothingElse(t *testing.T) {
	// Declared with capitals and a domain, because that is what a person types.
	self := NewSelfSet([]string{"Lars@Private.Example"}, []string{"other.example"})

	for _, tc := range []struct {
		address string
		want    bool
		why     string
	}{
		{"lars@private.example", true, "the address as stored"},
		{"LARS@PRIVATE.EXAMPLE", true, "a message header need not agree with the declaration about case"},
		{"  lars@private.example  ", true, "a header value carries the whitespace around it"},
		{"lars+news@private.example", false, "a plus-address is a DIFFERENT address: treating it as the same " +
			"would silence mail to an address the seat never declared"},
		{"anyone@other.example", true, "a declared domain covers the addresses on it"},
		{"anyone@mail.other.example", true, "and its subdomains, the way the workspace's own domains do"},
		{"anyone@notother.example", false, "a domain that merely ENDS with a claimed one is somebody else — " +
			"the suffix test includes the separating dot"},
		{"anyone@other.example.attacker.test", false, "and a claimed domain does not cover a domain that " +
			"contains it as a prefix"},
		{"", false, "no address is not the seat"},
		{"not-an-address", false, "a value with no domain resolves to no domain, which is covered by nothing"},
	} {
		if got := self.Covers(tc.address); got != tc.want {
			t.Errorf("Covers(%q) = %v, want %v — %s", tc.address, got, tc.want, tc.why)
		}
	}
}

// A seat that declared nothing changes no decision. Every gate reading this set
// must be left exactly where it was, which is what makes the feature additive.
// A unicode domain and its punycode form are ONE address. Domain claims
// already normalize through IDNA, and an address claim that did not would
// disagree with a domain claim about the same mailbox.
func TestASelfSetFoldsAnAddressesDomainTheWayADomainClaimIsFolded(t *testing.T) {
	unicode := NewSelfSet([]string{"owner@bücher.example"}, nil)
	if !unicode.Covers("owner@xn--bcher-kva.example") {
		t.Error("a unicode address claim does not cover its punycode form — a header carries whichever " +
			"form the sender's client wrote")
	}
	punycode := NewSelfSet([]string{"owner@xn--bcher-kva.example"}, nil)
	if !punycode.Covers("owner@bücher.example") {
		t.Error("a punycode address claim does not cover its unicode form")
	}
}

func TestAnEmptySelfSetCoversNothing(t *testing.T) {
	empty := NewSelfSet(nil, nil)
	if !empty.Empty() {
		t.Error("a seat that declared nothing does not report empty")
	}
	if empty.Covers("anyone@example.test") {
		t.Error("an empty set covers an address")
	}
	addresses := []string{"a@example.test", "b@example.test"}
	if got := empty.WithoutSelf(addresses); len(got) != len(addresses) {
		t.Errorf("an empty set removed %d address(es) — it must leave every gate where it was",
			len(addresses)-len(got))
	}
}

func TestWithoutSelfKeepsTheOrderAndDropsOnlyTheSeatsOwn(t *testing.T) {
	self := NewSelfSet([]string{"me@private.example"}, nil)
	got := self.WithoutSelf([]string{"first@acme.test", "me@private.example", "second@acme.test"})
	if len(got) != 2 || got[0] != "first@acme.test" || got[1] != "second@acme.test" {
		t.Errorf("WithoutSelf = %v, want the two external addresses in header order — "+
			"the ladder picks the FIRST of them, so order is a decision and not a detail", got)
	}
}
