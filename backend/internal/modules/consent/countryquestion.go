// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import "github.com/margince/margince/backend/internal/shared/ports/commsauthz"

// countryCouldChangeTheAnswer says whether the composer should ask the rep for
// this recipient's country. It asks only when an answer can matter: a contact
// whose record states no country, who was not cleared by the sender for this
// message, is refused for want of marketing consent, and an installed pack
// allows advertising until a refusal. A country that cannot move the decision
// is never asked for.
func countryCouldChangeTheAnswer(d commsauthz.Decision, country RecipientCountry, optOutInstalled bool) bool {
	if country.Known() || country.Source == CountryClearedBySender || d.SubjectKind != entityContact {
		return false
	}
	if d.Verdict == commsauthz.VerdictAllow || !refusedForWantOfMarketingConsent(d) {
		return false
	}
	return optOutInstalled
}

// refusedForWantOfMarketingConsent reports the three refusals that mean no
// consent and no exception stood in for it.
func refusedForWantOfMarketingConsent(d commsauthz.Decision) bool {
	switch d.ReasonCode {
	case commsauthz.ReasonMarketingConsentRequired, commsauthz.ReasonMarketingPermissionRequired:
		return true
	}
	return lacksMarketingConsent(d)
}
