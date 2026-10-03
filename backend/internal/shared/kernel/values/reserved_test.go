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

func TestIsReservedAddressReadsTheDomainAfterTheLastAt(t *testing.T) {
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
	}
	for addr, want := range cases {
		if got := IsReservedAddress(addr); got != want {
			t.Errorf("IsReservedAddress(%q) = %v, want %v", addr, got, want)
		}
	}
}
