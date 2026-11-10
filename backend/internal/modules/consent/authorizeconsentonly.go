// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import "github.com/margince/margince/backend/internal/shared/ports/commsauthz"

// A staff exception supplies neither consent nor a missing statutory ground.
// Supported statutory routes have already returned an allow before this check.
func requireConsentWhereNothingStandsIn(d commsauthz.Decision, country RecipientCountry) commsauthz.Decision {
	if d.Verdict == commsauthz.VerdictAllow || !lacksMarketingConsent(d) {
		return d
	}
	rules, _, applicable := applicableRules(country.Code)
	if applicable && !rules.ConsentOnly && len(rules.MarketingExceptions) == 0 {
		return d
	}
	d.Verdict = commsauthz.VerdictDeny
	d.ReasonCode = commsauthz.ReasonMarketingConsentRequired
	if !applicable || !rules.ConsentOnly {
		d.ReasonCode = commsauthz.ReasonMarketingPermissionRequired
	}
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
