// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Opening the work-calendar meetings that were captured while the limiter still
// held them.
//
// Capture no longer holds a calendar meeting at all: a connected calendar is a
// work calendar, so an event on it is workspace business. That rule binds every
// meeting captured from now on, and nothing re-asks the question for the ones
// captured before it — the ordinary recompute would read the hold off the row
// and honour it forever.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ReleaseCalendarMeetingHoldTx drops the limiter's hold from one captured
// calendar meeting and re-derives its audience without it.
//
// It removes one contributor — the hold the limiter would no longer place — and
// then asks the ordinary derivation what the remaining contributors say. A human
// who narrowed the row by hand outranks it, and so does a human's explicit
// member list.
//
// A mailbox posture is NOT among those contributors, and that is settled rather
// than overlooked: decideBirthTx returns before reading the mail-sharing posture
// for any kind but "email" (capture/birthdecision.go), because a meeting is not
// correspondence a mailbox posture was ever asked about, and holding one on the
// strength of a mail setting would empty the shared calendar for a reason nobody
// stated. TestACalendarEventFollowsTheWorkspaceDefaultNotTheMailPosture holds
// that. So a meeting's import row records no posture, deriveAudienceTx answers
// "workspace" for every meeting no person narrowed, and in practice this opens
// the row. Said plainly because the mechanism reads like a safety net it is not.
//
// ReasonNoRecord is otherwise the JUDGED hold — a suppressed sender, a thread
// judged the mailbox owner's private life — and opening one of those publishes
// private correspondence to the workspace. The read below refuses any row that
// is not shaped like a calendar record, so a caller that passes the wrong id
// gets no write rather than a disclosure. The two conditions, both required:
//
//   - kind = 'meeting'. Not a claim about who writes that kind: the extension
//     ingress copies it off a third-party unit's record unvalidated, so it is a
//     necessary condition and never a sufficient one.
//   - counterparty_email IS NULL, which is WHY a meeting reached the limiter at
//     all. This is the one no caller can forge by choosing a string: a judged
//     hold is always about a sender, so its row names one.
//
// Stated in the WHERE clause rather than trusted to the caller, because this is
// the function that does the opening.
func ReleaseCalendarMeetingHoldTx(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID) error {
	// The same locked read RecomputeAudienceTx takes, and for the same reasons:
	// the lock orders this against every other writer of a derived row, and
	// restricted_at excludes a row under a statutory hold, which this must never
	// make more readable.
	var stored string
	var storedReason *string
	err := tx.QueryRow(ctx, `
		SELECT audience, audience_reason FROM activity
		 WHERE id = $1 AND kind = 'meeting' AND counterparty_email IS NULL
		   AND restricted_at IS NULL AND archived_at IS NULL FOR UPDATE`,
		activityID).Scan(&stored, &storedReason)
	if err != nil {
		if err == pgx.ErrNoRows {
			// Erased, deleted, under a statutory hold, archived by the noise
			// sweep, not a meeting, or a row naming a counterparty — which is a
			// judged hold, not this function's. Releasing nothing is the right
			// answer to all of them.
			return nil
		}
		return fmt.Errorf("activities: reading the meeting being released: %w", err)
	}
	// A human's explicit member list is not a repair's to move, exactly as it is
	// not the recompute's.
	if stored == audienceSelected {
		return nil
	}
	// Everything else is refused by naming what this release IS about.
	//
	// RecomputeAudienceTx derives an audience from scratch, so it has to ask
	// separately whether a human's narrowing outranks what it derived. This
	// removes ONE named contributor instead, and a reason that is not that
	// contributor is a hold this function has nothing to say about — a person's
	// ReasonManual among them. One check rather than two: a second that refuses
	// the same rows only looks like defence in depth, because a mutation of
	// either one still passes and neither is then proven to hold anything.
	if !releasableHold(deref(storedReason)) {
		// A workspace floor, a counterparty hold, a confidentiality marker, a
		// person's own narrowing. Every one of them is still true.
		return nil
	}
	derived, reason, ok, err := deriveAudienceTx(ctx, tx, activityID)
	if err != nil {
		return err
	}
	if !ok {
		// No import rows: not a captured row, and not this function's to decide.
		return nil
	}
	if derived == stored && sameReason(storedReason, reason) {
		return nil
	}
	return writeDerivedAudienceTx(ctx, tx, activityID, stored, storedReason, derived, reason)
}

// releasableHold names the two spellings of the limiter's "named nobody" hold.
//
// Two, because the reason was split at the writer partway through this tree's
// life: a meeting held before the split carries ReasonNoRecord and one held
// after it carries ReasonNoCounterparty. Both meant the same thing on a calendar
// meeting — the mapper leaves the counterparty unset because attendance is a
// list — and neither was ever a judgement about a person.
func releasableHold(reason string) bool {
	return reason == ReasonNoRecord || reason == ReasonNoCounterparty
}
