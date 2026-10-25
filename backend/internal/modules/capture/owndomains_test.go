// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/owndomain"
)

// CAP-AC1.3 — every participant internal is the zero-rows condition; one
// external participant makes the whole message external. The second half is
// what keeps the introduction working: a colleague writing to a prospect with
// the prospect copied is correspondence, not chatter.
func TestAllInternalNeedsEveryPartyAndOneOutsiderIsEnoughToKeepTheMessage(t *testing.T) {
	own := NewInternalDomains([]string{"acme.com"})

	if !own.AllInternal([]string{"rep@acme.com", "boss@mail.acme.com"}) {
		t.Error("two colleagues, one on a subdomain: want all-internal")
	}
	if own.AllInternal([]string{"rep@acme.com", "client@customer.example"}) {
		t.Error("a copied prospect makes the message external — it must be kept")
	}
	if own.AllInternal([]string{"rep@acme.com", "unparseable"}) {
		t.Error("an address with no readable domain is not internal")
	}
}

// CAP-AC1.3d — an empty own-domain set makes nothing internal, so everything
// captures. An installation that has registered no domain is making no claim
// about its contacts's mail, and inventing one would be wrong in most workspaces.
func TestAnEmptyOwnDomainSetMakesNothingInternal(t *testing.T) {
	own := NewInternalDomains(nil)
	if own.AllInternal([]string{"rep@acme.com", "boss@acme.com"}) {
		t.Error("with no registered domain nothing is internal, so the message is kept")
	}
}

// A message that names nobody is not internal either. A connector reporting no
// addresses is saying it could not read the parties, which is different from
// saying there were none — and the direction to fail in is toward keeping mail.
func TestAMessageNamingNobodyIsNotInternal(t *testing.T) {
	own := NewInternalDomains([]string{"acme.com"})
	if own.AllInternal(nil) {
		t.Error("no addresses is not evidence of an internal message")
	}
	if own.AllInternal([]string{"", "   "}) {
		t.Error("blank addresses are not evidence of an internal message")
	}
}

// The external set is what a captured message may create records for, in header
// order and deduplicated. It is deliberately not the message's author.
func TestExternalNamesTheOutsidersInHeaderOrder(t *testing.T) {
	own := NewInternalDomains([]string{"acme.com"})
	got := own.External([]string{
		"rep@acme.com",
		"client@customer.example",
		"boss@mail.acme.com",
		"CLIENT@customer.example", // same party, different case
		"second@other.example",
	})
	want := []string{"client@customer.example", "second@other.example"}
	if !slices.Equal(got, want) {
		t.Errorf("External = %v, want %v", got, want)
	}
}

// An account label arrives in either shape a provider chooses. Taking the text
// after the last "@" of the display form yields "acme.com>", which matches no
// registered domain — so the gate would quietly never fire for that mailbox.
func TestAnAccountLabelYieldsItsDomainInEitherShape(t *testing.T) {
	for _, label := range []string{
		"rep@acme.com",
		"Rep <rep@acme.com>",
		"  Rep Example <rep@acme.com>  ",
		`"Rep, Example" <rep@acme.com>`,
	} {
		if got := owndomain.OfAddress(bareAddress(label)); got != "acme.com" {
			t.Errorf("label %q gave domain %q, want acme.com", label, got)
		}
	}
	// An unparseable label is judged as an address, which is what it was before.
	if got := owndomain.OfAddress(bareAddress("not an address")); got != "" {
		t.Errorf("unparseable label gave domain %q, want none", got)
	}
}

// The administrator's own domain is stored in the form owndomain.Fold gives,
// so the SQL own-domain checks can compare it without folding.
func TestAnOwnDomainIsStoredFolded(t *testing.T) {
	for _, typed := range []string{"Bücher.Example", "@bücher.example.", "XN--BCHER-KVA.EXAMPLE", "ｂücher.example"} {
		stored, err := ValidOwnDomain(typed)
		if err != nil {
			t.Fatalf("ValidOwnDomain(%q): %v", typed, err)
		}
		if want := owndomain.Fold(strings.TrimPrefix(typed, "@")); stored != want {
			t.Errorf("ValidOwnDomain(%q) stores %q, owndomain.Fold gives %q", typed, stored, want)
		}
	}
}
