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
		rivals := batch.Rivals(tc.candidate)
		if got := rivals.spelled || len(rivals.scored) > 0; got != tc.want {
			t.Errorf("Rivals(%q) found a rival = %v, want %v", tc.candidate.DisplayName, got, tc.want)
		}
	}
}

// A typo in a name's one distinctive word still passes the gate, which matches
// words fuzzily. The batch must find the member the ladder would score.
func TestCompanyBatchFindsATypoInTheOneDistinctiveWord(t *testing.T) {
	pairs := [][2]string{
		{"Siemens", "Siemenss"},
		{"Microsoft", "Mircosoft"},
		{"Kestrel", "Kestral"},
		{"Nordwind Logistik", "Nordwnd Logistic"},
	}
	for _, pair := range pairs {
		var batch CompanyBatch
		batch.Add(CompanyCandidate{DisplayName: pair[0]})
		_, ladder := companyPairRival(companyNamePairOf(pair[1], ""), companyNamePairOf(pair[0], ""))
		if !ladder {
			t.Fatalf("the ladder does not score %q against %q, so the case proves nothing", pair[1], pair[0])
		}
		if rivals := batch.Rivals(CompanyCandidate{DisplayName: pair[1]}); len(rivals.scored) == 0 {
			t.Errorf("Rivals(%q) after %q found no rival, but the ladder scores the pair", pair[1], pair[0])
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
		if got := batch.Rivals(candidate).Any(); got != tc.want {
			t.Errorf("Rivals(%q <%s>) found a rival = %v, want %v", tc.name, tc.email, got, tc.want)
		}
	}
}
