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

	// The subject's own stop: nobody sends past it, an admin included. A stop
	// recorded by mistake is corrected on the record, which lifts the refusal.
	ReasonObjection:        {},
	ReasonRestricted:       {},
	ReasonSubjectRequest:   {},
	ReasonConsentWithdrawn: {},

	// A dead mailbox, an unconfirmed opt-in, a rolling cap and an unresolvable
	// recipient: only an admin may send past these.
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

// ExceptionMayAnswer reports whether a recorded one-message exception may stand
// over every refusal in this set. Staging and dispatch both ask it, so a stop
// or an admin-only refusal recorded after the exception was given still stops
// the message. givenByAdmin is the exception's own record of who gave it, never
// the authority of whoever is sending now.
func (s DecisionSet) ExceptionMayAnswer(givenByAdmin bool) bool {
	for _, d := range s.Decisions {
		if d.Verdict == VerdictAllow || HasRemedy(d.ReasonCode, RemedyException) {
			continue
		}
		if givenByAdmin && HasRemedy(d.ReasonCode, RemedyAdminException) {
			continue
		}
		return false
	}
	return true
}
