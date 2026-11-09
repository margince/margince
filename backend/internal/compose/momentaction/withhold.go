// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package momentaction decides which verbs a moment card may offer, and to
// whom.
//
// The card is the same card on both record pages — the contract says so, and
// both the contact page and the account page fill `ContactMoment` and mint the
// verbs under it. The grant a verb needs is a property of the VERB rather than
// of the page that drew it, so both assemblers hand their finished card here
// instead of deciding it for themselves. They did not always: the contact page
// decided it and the account page, which shipped its card later, did not — and
// offered "Log something" to a reader whose POST /activities the store then
// refuses, which is a button that looks live and does nothing.
package momentaction

import (
	"context"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// writesAnActivity classifies the whole action-kind vocabulary by whether
// pressing the verb needs `activity.create`. POST /activities does, where the
// log form and the task form both post, and so do POST /bookings and every send.
//
// Every kind carries an answer, including the false ones. A lookup miss and a
// "no" are the same value in Go, and the miss fails open.
// gates/momentactionvocabulary_test.go holds these keys equal to the enum.
var writesAnActivity = map[crmcontracts.ContactMomentActionKind]bool{
	// The log form, under its own name and asked as a task. One POST, one grant.
	crmcontracts.ContactMomentActionKindLogActivity:  true,
	crmcontracts.ContactMomentActionKindCompleteTask: true,
	// The composer's send writes the message as an activity, so it asks `activity.create`.
	crmcontracts.ContactMomentActionKindDraftReply: true,
	// Booking creates a meeting through POST /bookings.
	crmcontracts.ContactMomentActionKindScheduleMeeting: true,
	// Reads. They navigate; they write nothing.
	crmcontracts.ContactMomentActionKindOpenRecord:       false,
	crmcontracts.ContactMomentActionKindOpenMeetingBrief: false,
	crmcontracts.ContactMomentActionKindOpenResearch:     false,
	crmcontracts.ContactMomentActionKindAskColleague:     false,
}

// Withhold turns every activity-writing verb on this card into a blocked one
// that says so, for a caller who may not create an activity.
//
// The ladders that build these cards derive their verbs from the page alone
// and know nothing about the caller, so a verb offered as available to a
// reader without the grant opens a form whose save is refused. The destination
// goes with the state: a blocked verb that still names a surface is a button
// the client can still route on.
func Withhold(ctx context.Context, language string, moment *crmcontracts.ContactMoment) {
	said := langcopy.For(language)
	if moment.MayBeDone != nil {
		withholdDone(ctx, said, moment)
	}
	if auth.Allows(ctx, "activity", principal.ActionCreate) {
		return
	}
	block := func(action *crmcontracts.ContactMomentAction) {
		if !writesAnActivity[action.Kind] {
			return
		}
		// Done on a may-be-done card writes the promise, not a new activity;
		// withholdDone asked that promise's own grant.
		if moment.MayBeDone != nil && action == &moment.RecommendedAction {
			return
		}
		// An action the ladder already blocked was blocked for its own reason,
		// and that reason is the one the reader needs: the account card's
		// "Open it from the task list" is off because the task names no record
		// beneath it, not because of a grant. Relabelling it would answer a
		// question the reader did not ask, about a control that was already
		// unavailable.
		if action.State == crmcontracts.ContactMomentActionStateBlocked {
			return
		}
		blockAction(action, said.Say(blockedReason(action.Kind)))
	}
	block(&moment.RecommendedAction)
	if moment.SecondaryActions == nil {
		return
	}
	for i := range *moment.SecondaryActions {
		block(&(*moment.SecondaryActions)[i])
	}
}

// withholdDone blocks Done on a may-be-done card for a reader who may not
// write the promise it completes: a task is an activity update, a claim
// settles under contact update. The endpoint still asks the row's own scope.
func withholdDone(ctx context.Context, said langcopy.Spoken, moment *crmcontracts.ContactMoment) {
	object, reason := "activity", said.Say(blockedWords.Complete)
	if moment.MayBeDone.PromiseType == crmcontracts.ContactMomentMayBeDonePromiseTypeClaim {
		object, reason = "contact", said.Say(blockedWords.Settle)
	}
	if auth.Allows(ctx, object, principal.ActionUpdate) {
		return
	}
	done := &moment.RecommendedAction
	blockAction(done, reason)
}

// blockedReason names what the reader may not do, in the verb's own words.
func blockedReason(kind crmcontracts.ContactMomentActionKind) langcopy.Phrase {
	switch kind {
	case crmcontracts.ContactMomentActionKindScheduleMeeting:
		return blockedWords.Book
	case crmcontracts.ContactMomentActionKindDraftReply:
		return blockedWords.Send
	default:
		return blockedWords.Log
	}
}

func blockAction(action *crmcontracts.ContactMomentAction, reason string) {
	action.State = crmcontracts.ContactMomentActionStateBlocked
	action.BlockedReason = &reason
	action.Destination = nil
}
