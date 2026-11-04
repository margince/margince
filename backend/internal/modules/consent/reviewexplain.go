// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// refusalFacts says what is in the way for each refusal a statement cannot
// answer, and for a stop what can still go. What may be done about it is not
// written here: refusalNextMove derives it from the remedy table, so the
// sentence and the door cannot disagree.
//
// Held by: TestEveryRefusalExplanationFollowsTheRemedyTable (reviewexplain_test.go)
var refusalFacts = map[string]string{
	commsauthz.ReasonObjection: "this contact objected to marketing, so no marketing goes to them; " +
		"a reply or a message they asked for still can",
	commsauthz.ReasonSubjectRequest: "this contact asked us to stop; only security, privacy and " +
		"opt-out notices still go to them",
	commsauthz.ReasonRestricted: "this record is under a processing restriction; only security, " +
		"privacy and opt-out notices still go to them",
	commsauthz.ReasonConsentWithdrawn: "they took their consent back for this kind of message, and only " +
		"they can give it again",
	commsauthz.ReasonHardBounce: "this address does not accept mail, so the message would not arrive " +
		"whatever the evidence says; correct the address",
	commsauthz.ReasonNoMarketingConsent: "marketing needs their express consent, which no recollection " +
		"of a conversation can supply",
	commsauthz.ReasonUnconfirmedDOI: "marketing needs their confirmed consent, which only they can " +
		"confirm",
	commsauthz.ReasonFrequencyCapReached: "this jurisdiction's limit on how often we may write has been " +
		"reached; it passes with time rather than with evidence",
	commsauthz.ReasonNoSubject: "this address does not resolve to one contact, so there is nobody to " +
		"record the evidence against",
	commsauthz.ReasonUnknownPurpose: "this message names a purpose this installation does not define, " +
		"which is a problem with the message rather than with what you know about the contact",
	commsauthz.ReasonLegacyTransactionalUnevidenced: "this message was sent under the old transactional " +
		"key with nothing behind it; give it a communication context and the engine can judge it",
	commsauthz.ReasonClaimContradictsResolution: "this message says it is one kind of message and the " +
		"record reads it as another; send it with the kind that fits",
	commsauthz.ReasonNoEvidence: "nothing on record supports this message yet",
}

// refusalNextMove says what may still be done about a refusal, read from
// commsauthz.RemediesFor. A stop lists nothing, so nobody sends past it.
func refusalNextMove(reason string) string {
	switch {
	case commsauthz.HasRemedy(reason, commsauthz.RemedyException):
		return "a send that has to go anyway needs a recorded exception"
	case commsauthz.HasRemedy(reason, commsauthz.RemedyAdminException):
		return "a send that has to go anyway needs an exception an admin records"
	case len(commsauthz.RemediesFor(reason)) == 0:
		return "nobody can send past it, an admin included"
	}
	return ""
}

// whyAStatementCannotAnswer says what is really in the way, in the words the
// rep needs rather than the engine's code, and what may still be done.
func whyAStatementCannotAnswer(reason string) string {
	fact, ok := refusalFacts[reason]
	if !ok {
		fact = fmt.Sprintf("this refusal (%s) is not one a statement about what happened can answer", reason)
	}
	parts := []string{fact}
	if move := refusalNextMove(reason); move != "" {
		parts = append(parts, move)
	}
	return strings.Join(parts, "; ")
}
