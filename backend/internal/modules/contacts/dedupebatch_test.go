// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import "testing"

func TestCompanyBatchFindsTheLookAlikesTheLadderWouldScore(t *testing.T) {
	var batch CompanyBatch
	batch.Add(CompanyCandidate{DisplayName: "Skipco Trading", Domains: []string{"Skipco.Example"}})

	cases := []struct {
		candidate CompanyCandidate
		want      bool
	}{
		{CompanyCandidate{DisplayName: "Skipco Trading GmbH"}, true},
		{CompanyCandidate{DisplayName: "Skipco Tradng"}, true},
		{CompanyCandidate{DisplayName: "skipco.example"}, true},
		{CompanyCandidate{DisplayName: "Nordwind Logistik"}, false},
		{CompanyCandidate{DisplayName: "  "}, false},
	}
	for _, tc := range cases {
		if got := batch.HasRival(tc.candidate); got != tc.want {
			t.Errorf("HasRival(%q) = %v, want %v", tc.candidate.DisplayName, got, tc.want)
		}
	}
}

func TestContactBatchNeedsTheSharedEmployerDomainTheLadderNeeds(t *testing.T) {
	var batch ContactBatch
	batch.Add(ContactCandidate{FullName: "Mareike Olsen", Emails: []string{"mareike@kestrel.example"}})
	batch.Add(ContactCandidate{FullName: "Tom Brandt", Emails: []string{"tom@gmail.com"}})

	cases := []struct {
		name, email string
		want        bool
	}{
		{"Mareike Olson", "m.olson@kestrel.example", true},
		{"Mareike Olson", "m.olson@nordwind.example", false},
		{"Tom Brandt", "tom.b@gmail.com", false},
		{"Jonas Weber", "jonas@kestrel.example", false},
	}
	for _, tc := range cases {
		candidate := ContactCandidate{FullName: tc.name, Emails: []string{tc.email}}
		if got := batch.HasRival(candidate); got != tc.want {
			t.Errorf("HasRival(%q <%s>) = %v, want %v", tc.name, tc.email, got, tc.want)
		}
	}
}
