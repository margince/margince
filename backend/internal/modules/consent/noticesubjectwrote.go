// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// subjectWroteNote is the ground a case closed by SettleWhenSubjectWroteTx
// carries. It names the evidence rather than the contact, so the row says why
// the duty ended without copying anything about them into it.
const subjectWroteNote = "They wrote to us: a captured mail from their own address shows the data came from them (Art. 13, not Art. 14), so no separate notice is owed."

// settleableOwedStates are the states a case may be closed FROM when it ends
// without a notice. A queued case is left alone: a disclosure is already on its
// way, and closing the case under it would leave a delivery nobody tracks.
// Every terminal state is left alone as well.
var settleableOwedStates = []string{
	string(NoticeOpen), string(NoticeAssigned), string(NoticeBlocked), string(NoticeDeliveryFailed),
}

// SettleWhenSubjectWroteTx ends the open duties recorded for these acquisitions,
// because the contact they belong to has since written to us.
//
// A mailbox backfill does not read a mailbox oldest first. When it reads our
// reply before their first mail, it creates the contact from our side of the
// conversation, cannot say where the address came from, and an Art. 14 case is
// opened (DutyFor, "unknown_legacy"). Their own mail, captured minutes later,
// is the evidence that settles it: the data came from them, the case that
// DutyFor opens for nobody (subject_initiated).
//
// The caller names the acquisitions, so this reads nothing of the contact: it
// is the acquisitions' OWN cases it closes, and only the ones still owed. The
// state is exempt_with_reason with a fixed ground, not not_required, so an
// auditor reading the row sees why it ended. resolved_by stays empty — no seat
// decided this; the captured mail did — and the audit row names the system
// actor the caller runs as.
//
// Returns how many cases it closed.
func SettleWhenSubjectWroteTx(ctx context.Context, tx pgx.Tx, acquisitions []ids.UUID, now time.Time) (int, error) {
	if len(acquisitions) == 0 {
		return 0, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT id FROM privacy_notice_case
		 WHERE acquisition_id = ANY($1) AND state = ANY($2)
		 ORDER BY id`, acquisitions, settleableOwedStates)
	if err != nil {
		return 0, fmt.Errorf("read the duties owed for these acquisitions: %w", err)
	}
	caseIDs, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return 0, fmt.Errorf("read the duties owed for these acquisitions: %w", err)
	}
	return closeOwedCasesTx(ctx, tx, caseIDs, subjectWroteNote, now)
}

// closeOwedCasesTx ends each named case as exempt_with_reason on one ground,
// with one audit entry each. Every reason a case ends without a disclosure
// closes it here, so the lock, the re-check and the audit cannot drift apart.
//
// Re-checked under the lock: an officer may have closed one between the
// caller's read and here, and their ground must not be overwritten. A queued
// case keeps the disclosure already on its way.
func closeOwedCasesTx(ctx context.Context, tx pgx.Tx, caseIDs []ids.UUID, note string, now time.Time) (int, error) {
	closed := 0
	for _, id := range caseIDs {
		current, err := lockNoticeCase(ctx, tx, id)
		if err != nil {
			return closed, err
		}
		if terminalNoticeStates()[current.State] || current.State == NoticeQueued {
			continue
		}
		out, err := scanNoticeCase(tx.QueryRow(ctx, `
			UPDATE privacy_notice_case
			   SET state = $2, resolution_note = $3, completed_at = $4,
			       blocked_reason = NULL, updated_at = now()
			 WHERE id = $1
			RETURNING`+noticeCaseColumns,
			id, string(NoticeExemptWithReason), note, now))
		if err != nil {
			return closed, fmt.Errorf("close a duty that ended without a notice: %w", err)
		}
		if err := auditNoticeCase(ctx, tx, current, out, map[string]any{fieldResolutionNote: true}); err != nil {
			return closed, err
		}
		closed++
	}
	return closed, nil
}
