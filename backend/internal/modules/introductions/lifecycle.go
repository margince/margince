// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package introductions

// The states an ask moves through, and who may move it.
//
// Pure: no database, no clock, no principal. The store asks this file whether a
// move is legal and who is entitled to make it, so the rules can be read in one
// place and tested without a transaction.

import "github.com/margince/margince/backend/internal/shared/apperrors"

// Status is where an ask stands.
type Status string

const (
	// StatusRequested is waiting on the colleague.
	StatusRequested Status = "requested"
	// StatusAccepted is the colleague agreeing to introduce — and not yet
	// having done it. The distinction is the point: accepting is a promise,
	// and reporting it as an introduction would close a loop still open.
	StatusAccepted Status = "accepted"
	// StatusNameDropApproved is "use my name", which is NOT an introduction
	// and never becomes one. The colleague is lending credibility, not making
	// a handshake, and only the rep can act on it.
	StatusNameDropApproved Status = "name_drop_approved"
	// StatusSuggestOther is the colleague naming somebody better placed.
	// Terminal here: acting on it creates a new ask against that colleague,
	// so the suggestion cannot silently become an ask nobody agreed to.
	StatusSuggestOther Status = "suggest_other"
	// StatusDeclined is a clear no, and the reason is kept: it is what stops
	// the product recommending this route again next week.
	StatusDeclined Status = "declined"
	// StatusIntroduced is the handshake, marked by a human who made it.
	StatusIntroduced Status = "introduced"
	// StatusNameDropped is the rep having used the name they were lent.
	StatusNameDropped Status = "name_dropped"
	// StatusReplied is the contact answering. Reached ONLY from captured
	// activity — a checkbox here would make the product's best outcome the one
	// claim nobody had evidence for.
	StatusReplied Status = "replied"
	// StatusExpired is nobody having acted before the due date.
	StatusExpired Status = "expired"
	// StatusCancelled is the requester withdrawing.
	StatusCancelled Status = "cancelled"
)

// Actor is who is attempting a move, in the only terms the rules care about.
type Actor string

const (
	// ActorRequester is the rep who asked.
	ActorRequester Actor = "requester"
	// ActorIntroducer is the colleague being asked.
	ActorIntroducer Actor = "introducer"
	// ActorClock is the expiry sweep. Not a person, and it may only expire —
	// a job that could decline on somebody's behalf would put words in a
	// colleague's mouth.
	ActorClock Actor = "clock"
	// ActorCapture is the activity consumer, which may only record a reply it
	// has evidence for.
	ActorCapture Actor = "capture"
)

// transition is one legal move.
type transition struct {
	from Status
	to   Status
	by   Actor
}

// The whole state machine, as data. A table rather than nested switches,
// because the question a reader asks is "can X do Y from Z" and a table
// answers it by being read.
var transitions = []transition{
	// The colleague's four bounded answers. Nobody else may give them: an
	// introduction the introducer did not agree to is a claim about their
	// relationship, made in their name.
	{StatusRequested, StatusAccepted, ActorIntroducer},
	{StatusRequested, StatusNameDropApproved, ActorIntroducer},
	{StatusRequested, StatusSuggestOther, ActorIntroducer},
	{StatusRequested, StatusDeclined, ActorIntroducer},

	// The handshake. Either party may record it, because either may be the one
	// who sees it happen; both are named in the audit row.
	{StatusAccepted, StatusIntroduced, ActorIntroducer},
	{StatusAccepted, StatusIntroduced, ActorRequester},

	// Using a lent name is the REQUESTER's act, so only they can report it.
	{StatusNameDropApproved, StatusNameDropped, ActorRequester},

	// The contact answering, from captured activity alone.
	{StatusIntroduced, StatusReplied, ActorCapture},
	{StatusNameDropped, StatusReplied, ActorCapture},

	// Withdrawing, while it is still the requester's to withdraw.
	{StatusRequested, StatusCancelled, ActorRequester},
	{StatusAccepted, StatusCancelled, ActorRequester},
	{StatusNameDropApproved, StatusCancelled, ActorRequester},

	// Running out of time. Every state where somebody still owes an action,
	// not just the first — an accepted ask nobody completed is exactly the
	// case a queue silently loses.
	{StatusRequested, StatusExpired, ActorClock},
	{StatusAccepted, StatusExpired, ActorClock},
	{StatusNameDropApproved, StatusExpired, ActorClock},
}

// May reports whether this actor can move an ask from one state to another,
// and refuses with the sentinel the HTTP layer turns into 403.
//
// The two failures are told apart on purpose: a move nobody may make is a
// conflict with the record's state, while a move the WRONG person attempts is a
// permission failure.
func May(from, to Status, by Actor) error {
	legal := false
	for _, t := range transitions {
		if t.from != from || t.to != to {
			continue
		}
		legal = true
		if t.by == by {
			return nil
		}
	}
	if legal {
		return apperrors.ErrPermissionDenied
	}
	return apperrors.ErrConflict
}

// Open reports whether an ask is still live — somebody owes an action on it.
// The duplicate guard and the expiry sweep both ask this function rather than
// listing the live statuses themselves, so a new status is added here once.
func Open(s Status) bool {
	switch s {
	case StatusRequested, StatusAccepted, StatusNameDropApproved:
		return true
	default:
		return false
	}
}

// Decision is one of the colleague's four answers, as it arrives on the wire.
func Decision(answer string) (Status, bool) {
	switch Status(answer) {
	case StatusAccepted:
		return StatusAccepted, true
	case StatusNameDropApproved:
		return StatusNameDropApproved, true
	case StatusSuggestOther:
		return StatusSuggestOther, true
	case StatusDeclined:
		return StatusDeclined, true
	default:
		return "", false
	}
}
