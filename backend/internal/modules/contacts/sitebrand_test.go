// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import "testing"

func TestASiteSpeaksForACompanyOnlyUnderANameItAlreadyHas(t *testing.T) {
	acme := CompanyIdentity{Names: []string{"Acme Robotics GmbH", ""}, Domains: []string{"acme.example"}}
	cases := []struct {
		name   string
		stated []string
		known  CompanyIdentity
		want   bool
	}{
		{"a casino on a taken-over domain", []string{"Lucky Spin Casino"},
			CompanyIdentity{Names: []string{"Example Analytics"}, Domains: []string{"example-analytics.example"}}, false},
		{"a casino on a triage domain", []string{"SPAMBRAND Ltd"}, CompanyIdentity{Domains: []string{"example-analytics.example"}}, false},
		{"the stored name with its legal form", []string{"Acme Robotics GmbH"}, acme, true},
		{"the brand alone", []string{"Acme"}, acme, true},
		{"the domain label", []string{"ACME Corp."}, CompanyIdentity{Domains: []string{"acme.example"}}, true},
		{"initials of the stated name", []string{"Bayerische Motoren Werke AG"}, CompanyIdentity{Domains: []string{"bmw.example"}}, true},
		{"initials of the stored name", []string{"IBM"}, CompanyIdentity{Names: []string{"International Business Machines"}}, true},
		{"one of several names agrees", []string{"Holding Nord GmbH", "Acme"}, acme, true},
		{"the umlaut written out", []string{"Müller GmbH"}, domainOnly("mueller.example"), true},
		{"the umlaut written out with a second word", []string{"Schröder Bau GmbH"}, domainOnly("schroeder-bau.example"), true},
		{"a Vietnamese name with Đ", []string{"Công ty TNHH Đông Á"}, domainOnly("donga.example"), true},
		{"initials without the article", []string{"The Boston Consulting Group"}, domainOnly("bcg.example"), true},
		{"initials with the connective whole", []string{"Bank of America"}, domainOnly("bofa.example"), true},
		{"a shared word group", []string{"Lucky Spin Group"}, nameOnly("Nordic Group AG"), false},
		{"a shared word solutions", []string{"Bet Solutions Ltd"}, nameOnly("Acme Solutions GmbH"), false},
		{"a shared word services", []string{"Global Casino Services"}, nameOnly("Kestner Services"), false},
		{"a shared word tech", []string{"Spin Tech"}, nameOnly("Nova Tech"), false},
		{"a generic word in the domain", []string{"Lucky Solutions Ltd"}, domainOnly("nordic-solutions.example"), false},
		{"a generic word ending the domain", []string{"Casino Group"}, domainOnly("acme-group.example"), false},
		{"a two-letter label inside a word", []string{"Lucky Lab Casino"}, domainOnly("ab.example"), false},
		{"a three-letter label as one word of several", []string{"Slot Hub Online"}, domainOnly("hub.example"), false},
		{"a three-letter name inside a longer one", []string{"Bet"}, nameOnly("Alphabet"), false},
		{"no name stated", nil, acme, true},
		{"only blank names stated", []string{" ", ""}, acme, true},
	}
	for _, c := range cases {
		if got := SiteSpeaksFor(c.stated, c.known); got != c.want {
			t.Errorf("%s: SiteSpeaksFor(%q) = %v, want %v", c.name, c.stated, got, c.want)
		}
	}
}

func domainOnly(domain string) CompanyIdentity { return CompanyIdentity{Domains: []string{domain}} }

func nameOnly(name string) CompanyIdentity { return CompanyIdentity{Names: []string{name}} }

func TestSiteStatedNamesAreTheDisplayAndLegalName(t *testing.T) {
	got := SiteStatedNames([]DeepReadField{
		{Field: fieldDisplayName, Value: "Acme"}, {Field: "industry", Value: "62"}, {Field: fieldLegalName, Value: "Acme GmbH"},
	})
	if len(got) != 2 || got[0] != "Acme" || got[1] != "Acme GmbH" {
		t.Fatalf("SiteStatedNames = %q, want the display and the legal name", got)
	}
}
