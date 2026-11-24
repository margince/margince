// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import "github.com/margince/margince/backend/internal/shared/ports/commsauthz"

// countryCouldChangeTheAnswer says whether the composer should ask the rep for
// this recipient's country, and only when an answer can matter. That is a
// contact with no stated country, whom the sender did not clear. The contact
// is refused for want of marketing consent while an installed pack is opt-out.
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
