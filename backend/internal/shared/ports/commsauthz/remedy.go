// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package commsauthz

import "slices"

// Remedy is what can resolve a refusal.
type Remedy string

const (
	// RemedyAnswer means the sender states what the message is.
	RemedyAnswer Remedy = "answer"
	// RemedyFact means a recorded fact the sender knows first-hand supports it.
	RemedyFact Remedy = "fact"
	// RemedyException means any holder of the exception grant may send this one
	// message anyway, with the refusal left on the record.
	RemedyException Remedy = "exception"
	// RemedyAdminException is the same act, taken only by an admin.
	RemedyAdminException Remedy = "admin_exception"
)

// remedies declares, for every reason code, what can resolve it. An empty list
// means nothing can. Every Reason constant has an entry, written out even when
// empty. A reason added without one fails TestEveryReasonDeclaresItsRemedy
// instead of becoming a silent dead end.
var remedies = map[string][]Remedy{
	ReasonNoEvidence:                     {RemedyAnswer, RemedyFact, RemedyException},
	ReasonLegacyTransactionalUnevidenced: {RemedyAnswer, RemedyFact, RemedyException},
	// Absolute for rollout modes (absoluteDenials), yet a sender's answer can
	// correct the request that caused it. The two are different axes.
	ReasonUnknownPurpose:             {RemedyAnswer, RemedyException},
	ReasonClaimContradictsResolution: {RemedyAnswer, RemedyException},
	// No answer or fact supplies marketing consent.
	ReasonNoMarketingConsent: {RemedyException},

	// The subject's own stop, a dead mailbox, an unconfirmed opt-in, a rolling
	// cap and an unresolvable recipient: only an admin may send past these.
	ReasonObjection:           {RemedyAdminException},
	ReasonRestricted:          {RemedyAdminException},
	ReasonSubjectRequest:      {RemedyAdminException},
	ReasonConsentWithdrawn:    {RemedyAdminException},
	ReasonHardBounce:          {RemedyAdminException},
	ReasonUnconfirmedDOI:      {RemedyAdminException},
	ReasonFrequencyCapReached: {RemedyAdminException},
	ReasonNoSubject:           {RemedyAdminException},

	// Allows, not refusals.
	ReasonAllowed:           {},
	ReasonAllowedByOverride: {},
}

// RemediesFor lists what can resolve a refusal reason. An unknown reason has
// none, so a code nobody declared offers nothing.
func RemediesFor(reason string) []Remedy {
	return append([]Remedy(nil), remedies[reason]...)
}

// HasRemedy reports whether the reason declares the remedy.
func HasRemedy(reason string, remedy Remedy) bool {
	return slices.Contains(remedies[reason], remedy)
}
