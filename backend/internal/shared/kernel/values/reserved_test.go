// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import "testing"

func TestIsReservedHostMatchesTheRFC2606NamesAndEverythingUnderThem(t *testing.T) {
	reserved := []string{
		"example.com", "example.net", "example.org",
		"mail.example.com", "a.b.example.net", "eu.example.org",
		"test", "acme.test", "deep.acme.test",
		"example", "acme.example",
		"invalid", "nowhere.invalid",
		"localhost", "app.localhost",
		"EXAMPLE.COM", "Acme.Test",
		"example.com.", "acme.test.",
		" example.org ",
	}
	for _, host := range reserved {
		if !IsReservedHost(host) {
			t.Errorf("IsReservedHost(%q) = false, want true", host)
		}
	}
}

func TestIsReservedHostRefusesLookAlikes(t *testing.T) {
	notReserved := []string{
		"", ".",
		"notexample.com", "example.co", "example.com.au", "examples.com",
		"example.community", "myexample.org", "example-test.com",
		"testing", "contest", "attest.io", "test.com", "localhost.com",
		"invalid.de", "example.io", "acme.com",
	}
	for _, host := range notReserved {
		if IsReservedHost(host) {
			t.Errorf("IsReservedHost(%q) = true, want false", host)
		}
	}
}

func TestIsReservedAddressReadsOnlyTheDomain(t *testing.T) {
	cases := map[string]bool{
		"buyer@example.com":      true,
		"Buyer@Mail.Example.COM": true,
		"ada@acme.test":          true,
		"buyer@notexample.com":   false,
		"buyer@example.co":       false,
		"buyer@gmail.com":        false,
		"not an address":         false,
		"":                       false,
		"buyer@.example.com":     false,
		// A quoted local part is no plain address, whichever side names a
		// reserved host.
		`"qa@example.com"@acme.de`: false,
		`"qa@acme.de"@example.com`: false,
	}
	for addr, want := range cases {
		if got := IsReservedAddress(addr); got != want {
			t.Errorf("IsReservedAddress(%q) = %v, want %v", addr, got, want)
		}
	}
}

// The erasure's own address cannot be stored, while the RFC names around it can.
//
// The distinction is the whole point of ErasedEmail: a seat is resolved by address, so
// an erasure writing this over one makes it name every seat any erasure ever wiped. A
// live record holding it would be indistinguishable from those. The RFC 2606 names are
// reserved in the weaker sense — no mailbox, but the test mailbox sends to them and
// fixtures seed contacts under them, so refusing those here would take the tree with it.
func TestParseEmailRefusesTheErasuresOwnAddressAndNoOtherReservedName(t *testing.T) {
	t.Parallel()
	if _, err := ParseEmail(ErasedEmail); err == nil {
		t.Error("ParseEmail stored the address an erasure writes over a seat, so a subject can " +
			"hold what marks a seat already erased")
	}
	// Case and padding are the same address: the parser lowercases before it decides.
	if _, err := ParseEmail("  Erased@Example.INVALID  "); err == nil {
		t.Error("ParseEmail stored the reserved address spelled differently")
	}
	for _, addr := range []string{"rita@reviewer.example", "a@anon.test", "x@example.com"} {
		if _, err := ParseEmail(addr); err != nil {
			t.Errorf("ParseEmail(%q) → %v: the RFC names stay storable, or the test mailbox and "+
				"every fixture under them stop working", addr, err)
		}
	}

	// And the address stays RESERVED by domain, which is a different question from
	// whether a record may hold it. The sweeps that skip reserved domains read rows
	// written before this reservation, so a tombstone that stopped classifying would
	// start being processed as an ordinary address.
	if !IsReservedAddress(ErasedEmail) {
		t.Error("IsReservedAddress no longer recognises the erasure's own address, so a sweep " +
			"that skips reserved domains stops skipping the rows an erasure already wrote")
	}
}
