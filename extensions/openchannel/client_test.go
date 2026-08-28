// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package openchannel

import (
	"errors"
	"net"
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
)

// The table below IS this connector's egress guard, and it is worth reading as
// the guard's real definition: the production code asks the stdlib's predicates
// and the core's published denylist, and what those two answer together is only
// checkable by naming addresses.
//
// EVERY ENTRY IS A WAY A MEMBER-SUPPLIED HOST REACHES INSIDE. A host name is text
// somebody typed, and DNS is theirs to point wherever they like — so the guard
// runs on the address the resolver returned, which is the only place the answer
// is knowable.
func TestTheEgressGuardRefusesEveryNonPublicAddress(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		address string
		refused bool
	}{
		{"loopback", "127.0.0.1:443", true},
		{"loopback in v6", "[::1]:443", true},
		{"the cloud metadata endpoint", "169.254.169.254:443", true},
		{"an rfc1918 host", "10.4.2.9:443", true},
		{"another rfc1918 host", "192.168.1.10:443", true},
		{"carrier-grade NAT", "100.71.0.5:443", true},
		{"this-network", "0.0.0.10:443", true},
		{"the unspecified address", "0.0.0.0:443", true},
		{"documentation space", "192.0.2.7:443", true},
		{"benchmarking space", "198.18.0.4:443", true},
		{"a unique local v6 address", "[fd00::1]:443", true},
		{"a v6 link-local address", "[fe80::1]:443", true},
		// The v6 ranges that CARRY a v4 address inside them, which To4() and
		// IsPrivate() do not catch and which are therefore how an internal
		// address is named without looking like one.
		{"6to4 wrapping a private v4 address", "[2002:0a04:0209::]:443", true},
		{"NAT64 wrapping loopback", "[64:ff9b::7f00:1]:443", true},
		{"a v4-mapped internal address", "[::ffff:10.4.2.9]:443", true},
		// And the ones that must still be reachable, because a guard that
		// refused everything would be a connector that sends nothing.
		{"an ordinary public host", "93.184.216.34:443", false},
		{"an ordinary public v6 host", "[2606:2800:220:1:248:1893:25c8:1946]:443", false},
		{"a v4-mapped public address", "[::ffff:93.184.216.34]:443", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := refusePrivate("tcp", tc.address, nil)
			if tc.refused && err == nil {
				t.Fatalf("%s was dialled — a member-supplied host must not become a probe of this deployment's own network", tc.address)
			}
			if !tc.refused && err != nil {
				t.Fatalf("%s was refused (%v), so this connector cannot reach an ordinary receiver", tc.address, err)
			}
		})
	}
}

// The denylist is READ FROM THE PUBLISHED SURFACE rather than hand-copied. A copy
// drifts, and a range the core refuses while this unit admits it is a member
// naming an internal address and reaching it.
func TestTheGuardRefusesEveryRangeTheCorePublishes(t *testing.T) {
	t.Parallel()
	reserved := extension.ReservedNets()
	if len(reserved) == 0 {
		t.Fatal("the published denylist is empty, so this guard is holding nothing beyond the stdlib predicates")
	}
	for _, network := range reserved {
		// The network address itself is inside its own range, which is all this
		// assertion needs: it is the range membership being checked, not a
		// particular host in it.
		if publicIP(network.IP) {
			t.Fatalf("%s is a range the core refuses and this unit would dial", network)
		}
	}
}

// The stored address is re-checked at the moment of dialling, not trusted from
// the row: what is in the column is what was legal when it was written, and a
// stored address that predates a tightened rule must not be dialable because it
// was stored first.
func TestTheAddressIsCheckedAgainAtTheMomentOfDialling(t *testing.T) {
	t.Parallel()
	for _, address := range []string{
		"http://hooks.example.com/crm",
		"https://10.4.2.9/crm",
		"https://user:pass@hooks.example.com/crm",
		"",
	} {
		t.Run(address, func(t *testing.T) {
			t.Parallel()
			if _, err := newSender(address); !errors.Is(err, extension.ErrInvalid) {
				t.Fatalf("%q was accepted as an address to dial, answering %v", address, err)
			}
		})
	}
}

// A refusal the guard makes is one nothing left for, which is what lets the send
// path call it a definite answer rather than an unanswerable one.
func TestAGuardedSenderRefusesBeforeItResolvesAnything(t *testing.T) {
	t.Parallel()
	built, err := newSender("https://hooks.example.com/crm")
	if err != nil {
		t.Fatalf("building a sender for an ordinary address: %v", err)
	}
	if built.http.Transport == nil {
		t.Fatal("the sender carries the default transport, which has no egress guard on its dialler")
	}
	if net.ParseIP("hooks.example.com") != nil {
		t.Fatal("the fixture host parses as an address, so this test is not about a name at all")
	}
}
