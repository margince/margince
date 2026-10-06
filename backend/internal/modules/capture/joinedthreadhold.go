// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What a message's audience owes the thread it ENDED UP in, rather than the one
// its own header named.

package capture

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// holdFromJoinedThreadTx closes a message the join moved into a held
// conversation.
//
// The birth decision reads the verdict of the thread key the message's own
// References header gives it, and the join runs afterwards and may move the
// message to a different key. A shortened References header is enough: a reply
// roots on a key with no verdict, is born open under a shared posture, and then
// joins a conversation the classifier is holding. The thread is held and one
// message in it is not.
//
// Re-derived from the row's CURRENT key rather than from the one the capture
// arrived with, because that is the question the audience answers — who may read
// this message now that it is filed here.
//
// It only ever CLOSES. A message born held stays held whatever the joined thread
// says: the hold was placed for a reason of its own — a counterparty judgement, a
// workspace floor, a marker — and none of those is undone by the filing. That is
// the same direction ThreadVerdictMergeTx takes for the thread itself, which
// keeps a merged verdict at least as closed as either half was.
func (s *Sink) holdFromJoinedThreadTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID) error {
	seat := actorUserID(ctx)
	if seat == ids.Nil {
		return nil
	}
	var status string
	err := tx.QueryRow(ctx, `
		SELECT v.status
		  FROM activity a
		  JOIN capture_thread_verdict v
		    ON v.thread_key = a.thread_key AND v.user_id = $2
		 WHERE a.id = $1 AND a.audience = $3
		   AND a.archived_at IS NULL AND a.restricted_at IS NULL`,
		id, seat, audienceWorkspace).Scan(&status)
	if err != nil {
		// No verdict on the joined thread, or the message is not open anyway.
		if err == pgx.ErrNoRows {
			return nil
		}
		return fmt.Errorf("capture: reading the joined thread's verdict: %w", err)
	}
	if !holdsItsMessages(status) {
		return nil
	}
	// THE IMPORT ROW FIRST, because that is what survives.
	// activities.RecomputeAudienceTx derives an audience from capture_import's
	// posture_at_import and verdict_status, so narrowing only the activity leaves a
	// hold the next recompute re-derives away: the import row still carries the
	// verdict this message was born under, which is the one that did not hold it.
	// Recording the joined thread's verdict here is what the owner's own decision
	// does for the same reason — a ledger entry nothing acts on is not a decision.
	// The REASON too, not only the status, because the two writes below have to
	// agree about it. activities.contributionOf derives a held verdict's reason from
	// verdict_reason and falls back to its own word when the column is null, so an
	// import row that records the status and not the reason makes the next recompute
	// rewrite the activity with a different word than this one stamps — a spurious
	// write, and a reader shown a reason that changes under them.
	//
	// inherited_verdict is already the word for this: the thread carried a holding
	// verdict and this message joined it. Recording it here is what lets the
	// derivation reach the same answer instead of a mapping kept in two places.
	if _, err := tx.Exec(ctx, `
		UPDATE capture_import SET verdict_status = $3, verdict_reason = $4
		 WHERE activity_id = $1 AND user_id = $2`,
		id, seat, status, audienceReasonInheritedVerdict); err != nil {
		return fmt.Errorf("capture: recording the joined thread's verdict on the import row: %w", err)
	}
	// The same predicate as the read, archived row included: a message nobody can
	// read any more needs no narrowing, and leaving it out of both halves is what
	// lets the row count below mean something. Inside one transaction nothing else
	// writes this row, so exactly one row answers the read and the write alike —
	// a count of zero means an assumption broke, not a race.
	tag, err := tx.Exec(ctx, `
		UPDATE activity SET audience = $2, audience_reason = $3
		 WHERE id = $1 AND audience = $4
		   AND archived_at IS NULL AND restricted_at IS NULL`,
		id, audienceParticipants, audienceReasonInheritedVerdict, audienceWorkspace)
	if err != nil {
		return fmt.Errorf("capture: holding a message its joined thread holds: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf(
			"capture: holding a message its joined thread holds: %d rows, want 1", tag.RowsAffected())
	}
	return nil
}

// holdsItsMessages names the verdicts under which a conversation's messages are
// not the workspace's to read. It is bornAudience's own list, asked of a verdict
// the join supplied rather than one the birth decision read.
func holdsItsMessages(status string) bool {
	switch status {
	case VerdictHeld, VerdictUnsure, VerdictHeldByOwner, VerdictPending:
		return true
	}
	return false
}
