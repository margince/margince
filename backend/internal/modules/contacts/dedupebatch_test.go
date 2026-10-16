// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import "testing"

func TestCompanyBatchFindsTheLookAlikesTheLadderWouldScore(t *testing.T) {
	var batch CompanyBatch
	batch.Put("row:1", CompanyCandidate{DisplayName: "Skipco Trading", Domains: []string{"Skipco.Example"}})

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
		batch.Put("row:1", CompanyCandidate{DisplayName: pair[0]})
		_, ladder := companyPairRival(companyNamePairOf(pair[1], ""), companyNamePairOf(pair[0], ""))
		if !ladder {
			t.Fatalf("the ladder does not score %q against %q, so the case proves nothing", pair[1], pair[0])
		}
		if rivals := batch.Rivals(CompanyCandidate{DisplayName: pair[1]}); len(rivals.scored) == 0 {
			t.Errorf("Rivals(%q) after %q found no rival, but the ladder scores the pair", pair[1], pair[0])
		}
	}
}

// A record two rows write is one member under the name it was written last.
func TestABatchMemberTakesTheNameItsRecordWasWrittenLast(t *testing.T) {
	var companies CompanyBatch
	companies.Put("id:1", CompanyCandidate{DisplayName: "Nordwind Logistik"})
	companies.Put("id:1", CompanyCandidate{DisplayName: "Kestrel Data"})
	companies.Put("id:1", CompanyCandidate{})
	if rivals := companies.Rivals(CompanyCandidate{DisplayName: "Nordwind Logistik GmbH"}); len(rivals.scored) != 0 {
		t.Errorf("the company's first name still found a rival after the record was renamed")
	}
	if rivals := companies.Rivals(CompanyCandidate{DisplayName: "Kestrel Data GmbH"}); len(rivals.scored) != 1 {
		t.Errorf("the company's last name found %d rivals, want 1", len(rivals.scored))
	}

	var contacts ContactBatch
	contacts.Put("id:1", ContactCandidate{FullName: "Mareike Olsen", Emails: []string{"mareike@kestrel.example"}})
	contacts.Put("id:1", ContactCandidate{FullName: "Tom Brandt", Emails: []string{"tom@kestrel.example"}})
	if rivals := contacts.Rivals(ContactCandidate{FullName: "Mareike Olsen", Emails: []string{"m.olsen@kestrel.example"}}); rivals.Any() {
		t.Errorf("the contact's first name still found a rival after the record was renamed")
	}
	if rivals := contacts.Rivals(ContactCandidate{FullName: "Tom Brandt", Emails: []string{"t.brandt@kestrel.example"}}); len(rivals.names) != 1 {
		t.Errorf("the contact's last name found %d rivals, want 1, filed once under its one domain", len(rivals.names))
	}
}

func TestContactBatchNeedsTheSharedEmployerDomainTheLadderNeeds(t *testing.T) {
	var batch ContactBatch
	batch.Put("row:1", ContactCandidate{FullName: "Mareike Olsen", Emails: []string{"mareike@kestrel.example"}})
	batch.Put("row:2", ContactCandidate{FullName: "Tom Brandt", Emails: []string{"tom@gmail.com"}})

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
