// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package commsauthz

// RefusalCodes lists the reason codes a deny or review decision can hand a client.
// A client that words refusals mirrors it, so a new code is not shown as a generic
// refusal. It leaves out the two allow codes, ReasonAllowed and ReasonAllowedByOverride.
//
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
