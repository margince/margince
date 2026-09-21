// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// When a captured contact was actually acquired.
//
// A creation door that says nothing about timing leaves the acquisition dated
// from the row's own write, and for capture that is routinely wrong. The sink
// creates a counterparty AFTER the capture transaction commits; the verdict
// path creates one when a human finally answers a triage question, which can be
// days later; a backfill creates one from a message that arrived last year.
//
// The consequence is not cosmetic. compose/noticecaseopen.go dates the Art. 14
// disclosure deadline from `coalesce(occurred_at, captured_at)`, so an
// acquisition with no time gets a month from whenever the row was written. A
// contact acquired in March and captured in September is owed a disclosure that
// was already six months late, and the case shows a deadline a month away. The
// control fails silently and in the one direction that matters.

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// acquisitionTimeFor answers when this counterparty was actually acquired: the
// EARLIEST captured message they are a party to, not the one that happened to
// trigger the creation.
//
// The distinction is the whole correctness of this function, and reading the
// triggering activity alone gets it wrong. Two paths make the triggering
// message arbitrary:
//
//   - A backfill walks the mailbox in whatever order the provider returns, with
//     no chronological sort. The first message processed for a sender can be
//     last September's or this one's.
//   - An ambiguous sender's pending row is written ON CONFLICT DO NOTHING
//     (capture/pending.go), so it keeps the FIRST activity seen and later
//     messages from the same sender do not replace it. The verdict that
//     eventually creates the contact forwards that retained id.
//
// So dating from the trigger would give a contact whose oldest captured message
// is from March a deadline computed from September, and the Art. 14 duty would
// look eleven months fresher than it is. The earliest participation is the
// answer that does not depend on processing order.
//
// activity_participant is the right table rather than activity_link, because at
// creation time exactly one message is linked to the new contact — the one
// being processed — while participation is recorded by ADDRESS for every
// captured message, including the ones captured before anybody decided this
// sender was real.
//
// The database is the source rather than the connector payload in hand, and not
// because the payload is untyped: capture normalizes every provider into
// capture.ActivityFields, which carries a typed OccurredAt. The reason is that
// the payload describes ONE message and the question is about all of them. A
// caller holding September's record cannot answer "when did we first hear from
// this contact"; only the captured history can, and it is what the deadline has
// to run from.
//
// NIL ON EVERY FAILURE, and never an error. A nil answer lands the evidence row
// with occurred_at NULL, which is what every door did before this existed, and
// captured_at stands in — a worse deadline than the message's own time, and a
// far better outcome than refusing to create the contact. A capture that failed
// because a timestamp could not be read would drop the record entirely.
func acquisitionTimeFor(ctx context.Context, tx pgx.Tx, email string, activityID ids.ActivityID) *time.Time {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil
	}
	// The triggering activity is included as a floor rather than trusted as the
	// answer: a message being captured right now may not have its participant
	// rows visible to this statement yet, and a counterparty whose only message
	// is this one must still be dated from it.
	// A LEFT JOIN, not an inner one, and the difference is the fallback. The
	// triggering activity is included as a floor rather than trusted as the
	// answer: a counterparty whose only message is the one being processed must
	// still be dated from it, and an inner join drops that row before the id
	// arm can match — it has no participant row yet, or none recording this
	// address. The first version of this query joined inner and silently left
	// exactly those acquisitions undated.
	var occurred *time.Time
	err := tx.QueryRow(ctx, `
		SELECT min(a.occurred_at)
		  FROM activity a
		  LEFT JOIN activity_participant ap ON ap.activity_id = a.id
		 WHERE lower(ap.address) = $1
		    OR a.id = $2`, email, activityID).Scan(&occurred)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return nil
	}
	return occurred
}

// acquisitionFromCapture is the one place the capture doors state WHEN an
// acquisition happened.
//
// The KIND is the caller's, because the two doors know different things about
// it: the mail door can tell a reply from an unanswered outbound and picks
// between subject_initiated and the weaker claim, while the channel door has no
// direction at all in its request and has always recorded subject_initiated.
// Deciding the kind here would mean either inventing a direction the channel
// door does not have, or weakening the mail door's distinction.
//
// Both the sink and the verdict path go through it, so the two cannot drift
// into dating the same kind of acquisition differently — which is exactly what
// would happen if each spelled the struct itself, since only one of them is
// obviously running in arrears.
//
// Held by: TestBothCaptureDoorsDateTheAcquisitionFromTheMessage
// (backend/gates/acquisitiontime_test.go)
func acquisitionFromCapture(
	ctx context.Context, tx pgx.Tx, kind, email string, activityID ids.ActivityID,
) Acquisition {
	return Acquisition{
		Kind:       kind,
		OccurredAt: acquisitionTimeFor(ctx, tx, email, activityID),
	}
}
