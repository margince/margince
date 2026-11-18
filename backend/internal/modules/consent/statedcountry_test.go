// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// The sender's statement about one recipient comes before the record, and an
// entry about another address says nothing about this one.
func TestASenderStatementNamesItsOwnRecipientOnly(t *testing.T) {
	t.Parallel()
	stated := []commsauthz.CountryOverride{
		{Address: "Anna@Example.test", Country: "AT"},
		{Address: "bert@example.test"},
		{Address: "carl@example.test", Country: "Austria"},
	}
	for _, tc := range []struct {
		address string
		want    RecipientCountry
		found   bool
	}{
		{"anna@example.test", RecipientCountry{Code: "at", Source: CountryFromSender}, true},
		{"bert@example.test", RecipientCountry{Source: CountryClearedBySender}, true},
		{"carl@example.test", RecipientCountry{}, false},
		{"dora@example.test", RecipientCountry{}, false},
	} {
		got, found := statedCountry(stated, connector.Recipient{Email: tc.address})
		if got != tc.want || found != tc.found {
			t.Errorf("%s: got %+v found=%v, want %+v found=%v", tc.address, got, found, tc.want, tc.found)
		}
	}
}

// A cleared country is the baseline: no pack applies, even though the sender
// said something about the recipient.
func TestAClearedCountryIsTheBaseline(t *testing.T) {
	t.Parallel()
	cleared, _ := statedCountry([]commsauthz.CountryOverride{{Address: "anna@example.test"}}, connector.Recipient{Email: "anna@example.test"})
	if cleared.Known() {
		t.Fatalf("a cleared country reads as known: %+v", cleared)
	}
	if _, _, ok := applicableRules(cleared.Code); ok {
		t.Error("a cleared country applied a pack")
	}
	if stamp := rulesetStamp(cleared); stamp.Codes != nil || stamp.Version != nil {
		t.Errorf("a cleared country stamped %+v, want no jurisdiction", stamp)
	}
}

// A decision row keeps its own recipient's statement without an address, and
// gives it back under the row's own address.
func TestTheStagedEvidenceCarriesTheStatedCountry(t *testing.T) {
	t.Parallel()
	for name, stated := range map[string][]commsauthz.CountryOverride{
		"set":     {{Address: "anna@example.test", Country: "at"}},
		"cleared": {{Address: "anna@example.test"}},
	} {
		raw, err := evidenceJSON(commsauthz.Evidence{RecipientCountries: stated})
		if err != nil {
			t.Fatal(err)
		}
		got := evidenceFrom(raw, "anna@example.test").RecipientCountries
		if len(got) != 1 || got[0] != stated[0] {
			t.Errorf("%s: carried %+v, want %+v", name, got, stated)
		}
	}
	raw, err := evidenceJSON(commsauthz.Evidence{})
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "{}" {
		t.Errorf("no statement rendered %s, want {}", raw)
	}
	if got := evidenceFrom(raw, "anna@example.test").RecipientCountries; got != nil {
		t.Errorf("no statement read back as %+v", got)
	}
}

// A changed country is a changed assessment: a confirmation given under one
// country does not answer the same refusal under another.
func TestANewRecipientCountryChangesTheAcknowledgement(t *testing.T) {
	t.Parallel()
	refusal := func(country string) Review {
		return Review{Refusals: []RefusedRecipient{{Address: "anna@example.test", ReasonCode: commsauthz.ReasonSubjectRequest, Country: country}}}
	}
	decided := func(country string) commsauthz.DecisionSet {
		return commsauthz.DecisionSet{Decisions: []commsauthz.Decision{{
			Recipient: connector.Recipient{Email: "anna@example.test"}, Verdict: commsauthz.VerdictDeny,
			ReasonCode: commsauthz.ReasonSubjectRequest, Country: country, Mode: commsauthz.ModeEnforce,
		}}}
	}
	original := refusal("de")
	if !original.MatchesAssessment(decided("de")) {
		t.Fatal("the same refusal under the same country does not match, so the cases below prove nothing")
	}
	for _, country := range []string{"at", ""} {
		changed := refusal(country)
		if err := changed.checkAcknowledgement(original.AssessmentRevision()); err == nil {
			t.Errorf("country %q accepted the acknowledgement given under de", country)
		}
		if original.MatchesAssessment(decided(country)) {
			t.Errorf("a review under de matched the same refusal under %q", country)
		}
	}
}

// The rep is shown the country and its source, and the revision they confirm
// moves when either does.
func TestTheServedReviewShowsTheCountryItWasJudgedUnder(t *testing.T) {
	t.Parallel()
	review := func(country, source string) Review {
		return Review{Refusals: []RefusedRecipient{{
			Address: "anna@example.test", ReasonCode: commsauthz.ReasonSubjectRequest,
			Country: country, CountrySource: source,
		}}}
	}
	recorded := review("de", string(CountryFromContactAddress))
	wire := wireReview(recorded).Refusals[0]
	if wire.Country == nil || *wire.Country != "de" || wire.CountrySource == nil || string(*wire.CountrySource) != "contact_address" {
		t.Fatalf("served refusal country %v source %v, want de from the contact's address", wire.Country, wire.CountrySource)
	}
	for name, refreshed := range map[string]Review{
		"stated":               review("at", string(CountryFromSender)),
		"cleared":              review("", string(CountryClearedBySender)),
		"same country, stated": review("de", string(CountryFromSender)),
	} {
		if refreshed.AssessmentRevision() == recorded.AssessmentRevision() {
			t.Errorf("%s: the revision did not move", name)
		}
		served := wireReview(refreshed).Refusals[0]
		if served.CountrySource == nil || string(*served.CountrySource) != refreshed.Refusals[0].CountrySource {
			t.Errorf("%s: served source %v", name, served.CountrySource)
		}
	}
	if cleared := wireReview(review("", string(CountryClearedBySender))).Refusals[0]; cleared.Country != nil {
		t.Errorf("a cleared country was served as %q", *cleared.Country)
	}
}
