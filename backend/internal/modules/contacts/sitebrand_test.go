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
		{"no name stated", nil, acme, true},
		{"only blank names stated", []string{" ", ""}, acme, true},
	}
	for _, c := range cases {
		if got := SiteSpeaksFor(c.stated, c.known); got != c.want {
			t.Errorf("%s: SiteSpeaksFor(%q) = %v, want %v", c.name, c.stated, got, c.want)
		}
	}
}

func TestSiteStatedNamesAreTheDisplayAndLegalName(t *testing.T) {
	got := SiteStatedNames([]DeepReadField{
		{Field: fieldDisplayName, Value: "Acme"}, {Field: "industry", Value: "62"}, {Field: fieldLegalName, Value: "Acme GmbH"},
	})
	if len(got) != 2 || got[0] != "Acme" || got[1] != "Acme GmbH" {
		t.Fatalf("SiteStatedNames = %q, want the display and the legal name", got)
	}
}
