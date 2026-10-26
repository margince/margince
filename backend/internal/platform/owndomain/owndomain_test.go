// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package owndomain

import "testing"

// CAP-AC1.3c: a registered domain covers its subdomains and no lookalike, and
// both spellings of an internationalized domain are one value. Exact equality
// would leak the internal mail of every company sending from a subdomain.
func TestARegisteredDomainCoversItsSubdomainsAndNothingElse(t *testing.T) {
	own := New([]string{"acme.com", "münchen.de"})

	covered := []string{
		"rep@acme.com",
		"rep@mail.acme.com",
		"rep@a.deep.chain.acme.com",
		"REP@ACME.COM",
		"rep@münchen.de",
		"rep@xn--mnchen-3ya.de", // the same domain, punycode
		"rep@post.münchen.de",
	}
	for _, address := range covered {
		if !own.Covers(address) {
			t.Errorf("Covers(%q) = false, want true", address)
		}
	}

	// acme.com.evil.tld is the attack the separating dot exists to refuse: it
	// ENDS with a string containing acme.com, and it belongs to evil.tld.
	notCovered := []string{
		"rep@acme.com.evil.tld",
		"rep@notacme.com",
		"rep@acme.co",
		"rep@example.com",
		"no-at-sign",
		"trailing@",
		"",
	}
	for _, address := range notCovered {
		if own.Covers(address) {
			t.Errorf("Covers(%q) = true, want false", address)
		}
	}
}

// Registering the same domain twice, in two spellings or two cases, is one
// domain. The set is compared in one normalized form so a workspace cannot
// half-register itself.
func TestTheDomainSetNormalizesAndDeduplicates(t *testing.T) {
	own := New([]string{"ACME.com", "acme.com.", "münchen.de", "xn--mnchen-3ya.de", "", "  "})
	if !own.Covers("rep@acme.com") || !own.Covers("rep@münchen.de") {
		t.Fatal("normalized spellings must all resolve")
	}
	if len(own.Domains()) != 2 {
		t.Errorf("domains = %v, want the two distinct domains", own.Domains())
	}
}

// A public suffix never enters the set, whichever writer supplied it. One
// writer is a website field no admin validation sees. A `co.uk` would make every
// company beneath it internal, and their skipped mail could not be recovered.
func TestAPublicSuffixIsNeverPartOfTheOwnDomainSet(t *testing.T) {
	own := New([]string{"com", "co.uk", "com.br", "de", "acme.co.uk"})

	for _, address := range []string{
		"someone@bbc.co.uk",
		"someone@example.com",
		"someone@loja.com.br",
		"someone@beispiel.de",
	} {
		if own.Covers(address) {
			t.Errorf("Covers(%q) = true — a public suffix must never make a stranger internal", address)
		}
	}
	// The real registration beside them still counts.
	if !own.Covers("rep@acme.co.uk") {
		t.Error("a domain someone actually registers must still be covered")
	}
	if len(own.Domains()) != 1 {
		t.Errorf("set = %v, want only the ownable domain", own.Domains())
	}
}

// An empty set covers nobody: an installation that named no domain of its own
// makes no claim about whose mail is internal.
func TestAnEmptySetCoversNobody(t *testing.T) {
	own := New(nil)
	if !own.Empty() || own.Covers("rep@acme.com") {
		t.Error("a set built from nothing must be empty and cover nobody")
	}
}

// A domain is compared in one form, so a Unicode address at a domain registered
// in punycode is covered, and the other way round.
func TestFoldGivesOneFormForEitherSpelling(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"Bücher.Example.", "xn--bcher-kva.example"},
		{"xn--bcher-kva.example", "xn--bcher-kva.example"},
		{"  ACME.com ", "acme.com"},
		{"", ""},
	} {
		if got := Fold(c.in); got != c.want {
			t.Errorf("Fold(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	if !New([]string{"xn--bcher-kva.example"}).Covers("anna@bücher.example") {
		t.Error("a Unicode address at a punycode own domain must be covered")
	}
	if !New([]string{"bücher.example"}).Covers("anna@xn--bcher-kva.example") {
		t.Error("a punycode address at a Unicode own domain must be covered")
	}
}

// A domain IDNA cannot read stays lowercased, so a parse failure never reads
// as ours.
func TestFoldKeepsADomainIDNACannotRead(t *testing.T) {
	if got := Fold("-Ü.Example"); got != "-ü.example" {
		t.Errorf("Fold kept %q, want the lowercased domain", got)
	}
}

func TestOfAddressReadsTheDomainAfterTheLastAt(t *testing.T) {
	for in, want := range map[string]string{
		"rep@Acme.com":        "acme.com",
		`"a@b" @acme.com`:     "acme.com",
		"anna@bücher.example": "xn--bcher-kva.example",
		"no-at-sign":          "",
		"trailing@":           "",
	} {
		if got := OfAddress(in); got != want {
			t.Errorf("OfAddress(%q) = %q, want %q", in, got, want)
		}
	}
}
