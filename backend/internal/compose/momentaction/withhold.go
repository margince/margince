// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package momentaction decides which verbs a moment card may offer, and to
// whom.
//
// The card is the same card on both record pages — the contract says so, and
// both the contact page and the account page fill `PersonMoment` and mint the
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
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// writesAnActivity classifies the whole action-kind vocabulary by whether
// pressing the verb reaches POST /activities, which needs `activity.create` and
// is where the log form and the task form both post.
//
// EVERY kind carries an answer, including the false ones, because a lookup
// miss and a "no" are the same value in Go and the miss is the direction that
// fails open. What makes the false entries load-bearing rather than noise is
// that nothing here may be absent: gates/momentactionvocabulary_test.go holds
// these keys equal to the contract's own enum, so a ninth kind cannot reach a
// reader until somebody has said which of the two it is.
var writesAnActivity = map[crmcontracts.PersonMomentActionKind]bool{
	// The log form, under its own name and asked as a task. One POST, one grant.
	crmcontracts.PersonMomentActionKindLogActivity:  true,
	crmcontracts.PersonMomentActionKindCompleteTask: true,
	// The composer sends through POST /emails, which is a side service rather
	// than a record write and asks nothing of `activity.create`.
	crmcontracts.PersonMomentActionKindDraftReply: false,
	// Blocked wherever it is minted: nothing in the destination vocabulary
	// opens a scheduler yet.
	crmcontracts.PersonMomentActionKindScheduleMeeting: false,
	// Reads. They navigate; they write nothing.
	crmcontracts.PersonMomentActionKindOpenRecord:       false,
	crmcontracts.PersonMomentActionKindOpenMeetingBrief: false,
	crmcontracts.PersonMomentActionKindOpenResearch:     false,
	crmcontracts.PersonMomentActionKindAskColleague:     false,
}

// Withhold turns every activity-writing verb on this card into a blocked one
// that says so, for a caller who may not create an activity.
//
// The ladders that build these cards derive their verbs from the page alone
// and know nothing about the caller, so a verb offered as available to a
// reader without the grant opens a form whose save is refused. The destination
// goes with the state: a blocked verb that still names a surface is a button
// the client can still route on.
func Withhold(ctx context.Context, moment *crmcontracts.PersonMoment) {
	if auth.Require(ctx, "activity", principal.ActionCreate) == nil {
		return
	}
	reason := "You do not have permission to log activities"
	block := func(action *crmcontracts.PersonMomentAction) {
		if !writesAnActivity[action.Kind] {
			return
		}
		// An action the ladder already blocked was blocked for its own reason,
		// and that reason is the one the reader needs: the account card's
		// "Open it from the task list" is off because the task names no record
		// beneath it, not because of a grant. Relabelling it would answer a
		// question the reader did not ask, about a control that was already
		// unavailable.
		if action.State == crmcontracts.PersonMomentActionStateBlocked {
			return
		}
		action.State = crmcontracts.PersonMomentActionStateBlocked
		action.BlockedReason = &reason
		action.Destination = nil
	}
	block(&moment.RecommendedAction)
	if moment.SecondaryActions == nil {
		return
	}
	for i := range *moment.SecondaryActions {
		block(&(*moment.SecondaryActions)[i])
	}
}
