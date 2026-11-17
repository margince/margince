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
// A contact's own stop lists nothing, so nobody sends past it, an admin
// included. Every other policy refusal takes the one-message exception: a rep
// may send past it with a recorded reason, and the refusal stays on the record.
// No decision makes a dead address accept mail. A recipient that resolves to
// nobody, or to two contacts, was never checked for stops.
var remedies = map[string][]Remedy{
	ReasonNoEvidence:                     {RemedyAnswer, RemedyFact, RemedyException},
	ReasonDealNotRequested:               {RemedyAnswer, RemedyFact, RemedyException},
	ReasonLegacyTransactionalUnevidenced: {RemedyAnswer, RemedyFact, RemedyException},
	ReasonUnknownPurpose:                 {RemedyAnswer, RemedyException},
	ReasonClaimContradictsResolution:     {RemedyAnswer, RemedyException},
	ReasonNoMarketingConsent:             {RemedyException},
	ReasonLawfulBasisUnconfirmed:         {RemedyException},

	ReasonMarketingConsentRequired:    {RemedyException},
	ReasonMarketingPermissionRequired: {RemedyException},
	ReasonFrequencyCapReached:         {RemedyException},
	ReasonUnconfirmedDOI:              {RemedyException},

	// The subject's own stop. A stop recorded by mistake is corrected on the
	// record, which lifts the refusal.
	ReasonObjection:            {},
	ReasonRestricted:           {},
	ReasonSubjectRequest:       {},
	ReasonConsentWithdrawn:     {},
	ReasonOptedOutAfterRequest: {},

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
// over every refusal in this set. A stop or a dead address found after it was
// given still stops the message. A stop recorded under a reason the review
// already named is the consent module's question (instructionStillAnswers). givenByAdmin is the exception's
// own record of who gave it, never the authority of whoever is sending now.
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
