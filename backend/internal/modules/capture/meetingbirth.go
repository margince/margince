// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// What a CALENDAR event asks of the workspace, decided inside the capture
// transaction.
//
// The mail ladder in birthdecision.go answers a question a calendar was never
// asked — "what does this MAILBOX ask of its mail" — so a meeting runs its own
// three steps rather than borrowing five. What the two ladders share is the
// shape: every rule can only TIGHTEN, the first match decides, and all of them
// are evaluated so the row records every reason it was held.
//
// WHY A MEETING IS HELD BY DEFAULT AND A NOTE IS NOT.
//
// ActivityDiscoverClause reads an activity with no links as a workspace-shared
// note: coalesce(bool_or(...), true). For a note that is right — somebody wrote
// it down for the workspace. A calendar event is the opposite in every respect:
// it arrives from one person's mailbox without anybody choosing to share it, it
// carries their private appointments, and it links no record at all until
// something files it. 450 of the 468 meetings on the staging installation
// linked nothing, which is the common case rather than the corner.
//
// So the rule is inverted for this kind: a meeting is born held to its
// participants and OPENS when something files it against a record the reader
// can already see. That is the same shape the link-less message rule has in
// sinkaudience.go, reached by the same reasoning.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// decideMeetingBirthTx answers what this calendar event asks of the workspace.
//
// Three steps, strictest first:
//
//  1. a counterparty hold this seat placed — their lawyer's domain is held
//     whether it arrives as mail or as an invitation;
//  2. an explicit marker on the event's own subject — a title saying
//     [Vertraulich] is the organiser telling us, and no classifier is needed
//     to read it;
//  3. whether anybody outside the workspace is on it at all.
//
// The two mail-only steps are deliberately absent. The mail-sharing FLOOR and
// the mailbox POSTURE are both answers to "what does this mailbox ask of its
// mail", and a calendar is not a mailbox: borrowing them would let a workspace
// that shares its mail publish every private appointment on it, and one that
// holds its mail hide meetings it never meant to hide. A calendar posture of
// its own is a product decision, not something to infer from the mail one.
func decideMeetingBirthTx(
	ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord, fields ActivityFields,
) (birthDecision, error) {
	var decision birthDecision

	held, err := heldCounterpartyTx(ctx, tx, rec)
	if err != nil {
		return birthDecision{}, err
	}
	if held {
		decision = decision.hold(audienceReasonCounterparty)
	}

	if explicitlyConfidential(fields.Subject) {
		decision = decision.hold(audienceReasonConfidentialMarker)
	}

	// Nobody outside the workspace on it, so there is no correspondent this
	// meeting could be filed under and nothing to open it. This is the reason
	// audienceReasonNoCounterparty already names as "the calendar case", and it
	// is the one that catches the private dinner: it holds until something
	// files the event against a record, and then it stops being true.
	//
	// It is checked LAST of the three because it is the weakest claim. The two
	// above are judgements about this event; this one merely says nothing has
	// filed it yet.
	if rec.Counterparty.Email == "" {
		decision = decision.hold(audienceReasonNoCounterparty)
	}
	return decision, nil
}

// meetingHostUserID is WHOSE calendar a captured row came from, or nothing.
//
// A meeting's host is the seat whose connector read it: the sync loop mints a
// principal per seat, so the identity is already the one the write is running
// as. Reading it from the context rather than from the record is the same rule
// captured_by follows — a connector must not be able to name somebody else as
// the host of an event it is inserting.
//
// Nothing for every other kind, and that is deliberate rather than unfinished.
// A captured EMAIL belongs to the correspondence it is part of, not to whoever
// happens to sync it, and stamping a host on one would file every message in
// the workspace under a single seat's queue.
//
// Nothing, too, for a principal carrying no user — a system backfill or an
// engine-internal write. Those have no calendar to be the host of, and the
// column stays NULL rather than naming a machine.
func meetingHostUserID(ctx context.Context, kind string) *ids.UUID {
	if kind != "meeting" {
		return nil
	}
	user := actorUserID(ctx)
	if user.IsZero() {
		return nil
	}
	return &user
}
