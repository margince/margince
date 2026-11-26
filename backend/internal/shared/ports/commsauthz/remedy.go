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
// Policy findings remain on record after a human directs one message.
// Transport failures and an unknown recipient still need a real repair.
var remedies = map[string][]Remedy{
	ReasonFooterIncomplete:               {RemedyException},
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

	ReasonObjection:            {RemedyException},
	ReasonRestricted:           {RemedyException},
	ReasonSubjectRequest:       {RemedyException},
	ReasonConsentWithdrawn:     {RemedyException},
	ReasonOptedOutAfterRequest: {RemedyException},

	ReasonHardBounce: {},
	ReasonNoSubject:  {},
	// The subject is edited or the recipients split; nothing overrules a label.
	ReasonSubjectLabelMissing:   {},
	ReasonSubjectLabelsConflict: {},

	// Allows, not refusals.
	ReasonAllowed:           {},
	ReasonOptOutAllowed:     {},
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

// ExceptionMayAnswer checks whether a recorded one-message decision may answer every finding.
// Current facts and message scope are rechecked by consent before the decision is spent.
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
