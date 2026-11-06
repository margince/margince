// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import "github.com/margince/margince/backend/internal/shared/ports/commsauthz"

// requireConsentWhereNothingStandsIn turns a refusal for want of marketing
// consent into one nobody sends past, when the recipient's country allows
// nothing in its place (messaging.Rules.ConsentOnly). Everywhere else the
// refusal keeps the exception its reason declares.
//
// It runs before a standing override is asked, because the new reason is
// absolute and an override answers only a refusal that is not.
func requireConsentWhereNothingStandsIn(d commsauthz.Decision, country RecipientCountry) commsauthz.Decision {
	if d.Verdict == commsauthz.VerdictAllow || !lacksMarketingConsent(d) {
		return d
	}
	rules, _, applicable := applicableRules(country.Code)
	if !applicable || !rules.ConsentOnly {
		return d
	}
	d.Verdict = commsauthz.VerdictDeny
	d.ReasonCode = commsauthz.ReasonMarketingConsentRequired
	return d
}

// lacksMarketingConsent reports whether a refusal is marketing with no consent
// behind it. That is the legacy verdict's own reason, or a marketing claim the
// record does not bear out. Resolved is not read for the second, because it starts as
// marketing on every decision and an unproven follow-up keeps that default.
func lacksMarketingConsent(d commsauthz.Decision) bool {
	switch d.ReasonCode {
	case commsauthz.ReasonNoMarketingConsent:
		return true
	case commsauthz.ReasonNoEvidence:
		return d.Requested == commsauthz.CategoryMarketing
	default:
		return false
	}
}
