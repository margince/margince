// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

//go:build !integration

package gates

// The egress SSRF denylist exists twice, and this is where the two are held
// equal. internal/platform/netguard owns the guard the core dials through;
// pkg/extension PUBLISHES the same ranges, because an extension unit is its own
// module and may import only the published pkg/** surface.
//
// It used to be published nowhere, so a unit that dials a member-supplied host
// hand-copied the literals and this test read that unit's file off disk. That
// worked only while the unit lived in this repository — and the drift it guarded
// against was not a style difference but the way around the core's guard: a
// range the core refuses and the copy admits is an internal address a member can
// name. Publishing the list removes the class of bug instead of re-testing it,
// and what remains to guard is these two lists inside this one module.
//
// It compares the parsed networks rather than file text: the two are formatted
// and grouped differently on purpose.

import (
	"net"
	"testing"

	"github.com/margince/margince/backend/internal/platform/netguard"
	"github.com/margince/margince/backend/pkg/extension"
)

// The published list and the guard's own list are the same list. A unit that
// reads it from the surface cannot drift from the guard; a unit that hand-copied
// it could, and that drift was the way around the guard for a member-supplied
// host.
func TestPublishedReservedNetsMatchTheGuard(t *testing.T) {
	t.Parallel()
	published := extension.ReservedNets()
	internal := netguard.ReservedNetsForTest()
	if len(published) != len(internal) {
		t.Fatalf("published %d nets, guard has %d", len(published), len(internal))
	}
	for i := range published {
		if published[i].String() != internal[i].String() {
			t.Errorf("net %d: published %s, guard %s", i, published[i], internal[i])
		}
	}
}

// The published slice must not be the guard's own backing array. It is returned
// across a module boundary to callers the core does not control, and a caller
// that reslices or overwrites an element would be editing the guard itself.
func TestReservedNetsDoesNotHandOutTheGuardsOwnSlice(t *testing.T) {
	t.Parallel()
	first := extension.ReservedNets()
	if len(first) == 0 {
		t.Fatal("ReservedNets returned nothing — the denylist is the guard")
	}
	first[0] = nil
	if extension.ReservedNets()[0] == nil {
		t.Error("a caller mutating the returned slice changed what the next caller sees")
	}
}

// The two lists agreeing is not the same as the two DECISIONS agreeing.
//
// Each side composes its list with the stdlib predicates — loopback, private,
// link-local, multicast, unspecified — and that half is written out twice, in
// two files, in two modules. A list held equal while one side dropped
// IsPrivate would pass the test above and admit every RFC 1918 address, which
// is the first thing a member-supplied host resolves to.
//
// So this asks both sides about the same addresses instead of comparing their
// source. The corpus is deliberately one address per REASON a decision can be
// made — each stdlib predicate, each shape of published range, and the
// routable controls that stop a side answering "no" to everything.
func TestThePublishedEgressDecisionMatchesTheGuards(t *testing.T) {
	t.Parallel()
	corpus := []string{
		// The stdlib predicates, one address each.
		"127.0.0.1", "10.0.0.5", "172.16.0.1", "192.168.1.1",
		"169.254.169.254", "224.0.0.1", "0.0.0.0",
		"::1", "fe80::1", "ff02::1", "::",
		// The published ranges: what the stdlib predicates miss.
		"100.64.0.1", "192.0.2.1", "198.18.0.1", "255.255.255.255",
		"64:ff9b::7f00:1", "2002:7f00:1::", "::ffff:0:127.0.0.1",
		// And the controls. A guard that refused these would pass every
		// negative case above while refusing the whole internet.
		"93.184.216.34", "8.8.8.8", "2606:2800:220:1::",
	}
	agreed, refused, admitted := 0, 0, 0
	for _, address := range corpus {
		ip := net.ParseIP(address)
		if ip == nil {
			t.Fatalf("%q is not an address, so this corpus asks about nothing", address)
		}
		// The published side is asked through the hook, which is what a unit
		// actually dials through — the predicate behind it is unexported, and
		// testing an unexported function would prove less than testing the
		// thing units reach.
		refusedByUnit := extension.RefuseNonPublic("tcp", net.JoinHostPort(address, "443"), nil) != nil
		refusedByCore := !netguard.PublicIP(ip)
		if refusedByUnit != refusedByCore {
			t.Errorf("%s: the published surface %s it and the core guard %s — a unit and the core disagree "+
				"about whether this address is dialable, which is the drift publishing the list was meant to end",
				address, refusedOrNot(refusedByUnit), refusedOrNot(refusedByCore))
			continue
		}
		agreed++
		if refusedByUnit {
			refused++
		} else {
			admitted++
		}
	}
	// A corpus that stopped parsing, or a hook that answered nil for
	// everything, would agree with a guard that did the same.
	if agreed != len(corpus) {
		t.Fatalf("%d of %d addresses agreed", agreed, len(corpus))
	}
	// Two sides that both answer the same thing to everything agree perfectly
	// and decide nothing, so the agreement above is only worth something if
	// both answers were actually given.
	if refused == 0 || admitted == 0 {
		t.Fatalf("the corpus produced %d refusals and %d admissions — agreement between two inert guards "+
			"is not agreement about anything", refused, admitted)
	}
}

func refusedOrNot(refused bool) string {
	if refused {
		return "refuses"
	}
	return "admits"
}
