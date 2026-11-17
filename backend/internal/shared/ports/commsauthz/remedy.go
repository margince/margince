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
//
// Every policy refusal takes the one-message exception: a human sender may
// always send past the engine with a recorded reason, and the refusal and any
// stop stay on the record. Two refusals are not policy answers. No decision
// makes a dead address accept mail, and a recipient that resolves to nobody,
// or to two contacts, was never checked for the stops a reviewer would weigh.
var remedies = map[string][]Remedy{
	ReasonNoEvidence:                     {RemedyAnswer, RemedyFact, RemedyException},
	ReasonDealNotRequested:               {RemedyAnswer, RemedyFact, RemedyException},
	ReasonLegacyTransactionalUnevidenced: {RemedyAnswer, RemedyFact, RemedyException},
	ReasonUnknownPurpose:                 {RemedyAnswer, RemedyException},
	ReasonClaimContradictsResolution:     {RemedyAnswer, RemedyException},
	ReasonNoMarketingConsent:             {RemedyException},
	ReasonLawfulBasisUnconfirmed:         {RemedyException},

	ReasonObjection:                   {RemedyException},
	ReasonRestricted:                  {RemedyException},
	ReasonSubjectRequest:              {RemedyException},
	ReasonConsentWithdrawn:            {RemedyException},
	ReasonOptedOutAfterRequest:        {RemedyException},
	ReasonMarketingConsentRequired:    {RemedyException},
	ReasonMarketingPermissionRequired: {RemedyException},
	ReasonFrequencyCapReached:         {RemedyException},
	ReasonUnconfirmedDOI:              {RemedyException},

	ReasonHardBounce: {},
	ReasonNoSubject:  {},

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
	if len(s.Decisions) == 0 {
		return false
	}
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
