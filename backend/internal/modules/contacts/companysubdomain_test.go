// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"slices"
	"testing"
)

func TestDomainAncestorsStopAtTheRegistrableDomain(t *testing.T) {
	t.Parallel()

	cases := map[string][]string{
		"team.eu.example.com": {"eu.example.com", "example.com"},
		"example.com":         nil,
		"a.example.co.uk":     {"example.co.uk"},
		"x.alice.github.io":   {"alice.github.io"},
	}
	for domain, want := range cases {
		if got := domainAncestors(domain); !slices.Equal(got, want) {
			t.Errorf("domainAncestors(%q) = %v, want %v", domain, got, want)
		}
	}
}

func TestTwoDomainsNestWhenOneSitsUnderTheOther(t *testing.T) {
	t.Parallel()

	if !domainNests("team.eu.example.com", "eu.example.com") || !domainNests("example.com", "cloud.example.com") {
		t.Error("a subdomain does not nest under the domain it sits under")
	}
	if domainNests("example.com", "other.example") || domainNests("alice.github.io", "bob.github.io") {
		t.Error("two separate registrable domains nest")
	}
}
