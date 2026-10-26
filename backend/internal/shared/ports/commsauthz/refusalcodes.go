// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package commsauthz

// RefusalCodes lists the codes a deny or review hands a client; the frontend mirrors it.
// Held by: TestEveryReasonIsARefusalOrAnAllow (refusalcodes_test.go)
func RefusalCodes() []string {
	return []string{
		ReasonObjection,
		ReasonRestricted,
		ReasonSubjectRequest,
		ReasonHardBounce,
		ReasonUnconfirmedDOI,
		ReasonNoEvidence,
		ReasonLegacyTransactionalUnevidenced,
		ReasonUnknownPurpose,
		ReasonClaimContradictsResolution,
		ReasonNoSubject,
		ReasonNoMarketingConsent,
		ReasonConsentWithdrawn,
		ReasonFrequencyCapReached,
	}
}
