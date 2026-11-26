// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

func TestTheComposerAsksForACountryOnlyWhenAnOptOutCountryCouldChangeTheAnswer(t *testing.T) {
	refused := commsauthz.Decision{
		SubjectKind: entityContact, Verdict: commsauthz.VerdictDeny, ReasonCode: commsauthz.ReasonMarketingPermissionRequired,
	}
	for _, tc := range []struct {
		name      string
		decision  func(commsauthz.Decision) commsauthz.Decision
		country   RecipientCountry
		installed bool
		want      bool
	}{
		{"unknown country, consent missing, opt-out pack installed", nil, RecipientCountry{}, true, true},
		{"no opt-out pack is installed", nil, RecipientCountry{}, false, false},
		{"the record states a country", nil, RecipientCountry{Code: "de", Source: CountryFromContactAddress}, true, false},
		{"the sender cleared the country", nil, RecipientCountry{Source: CountryClearedBySender}, true, false},
		{"the sender stated a country", nil, RecipientCountry{Code: "de", Source: CountryFromSender}, true, false},
		{"the message is already allowed", func(d commsauthz.Decision) commsauthz.Decision {
			d.Verdict, d.ReasonCode = commsauthz.VerdictAllow, commsauthz.ReasonAllowed
			return d
		}, RecipientCountry{}, true, false},
		{"the refusal is the recipient's objection", func(d commsauthz.Decision) commsauthz.Decision {
			d.ReasonCode = commsauthz.ReasonObjection
			return d
		}, RecipientCountry{}, true, false},
		{"the recipient is a lead", func(d commsauthz.Decision) commsauthz.Decision {
			d.SubjectKind = entityLead
			return d
		}, RecipientCountry{}, true, false},
		{"a marketing claim with no consent", func(d commsauthz.Decision) commsauthz.Decision {
			d.Verdict, d.ReasonCode, d.Requested = commsauthz.VerdictReview, commsauthz.ReasonNoEvidence, commsauthz.CategoryMarketing
			return d
		}, RecipientCountry{}, true, true},
	} {
		d := refused
		if tc.decision != nil {
			d = tc.decision(d)
		}
		if got := countryCouldChangeTheAnswer(d, tc.country, tc.installed); got != tc.want {
			t.Errorf("%s: asks = %v, want %v", tc.name, got, tc.want)
		}
	}
}
