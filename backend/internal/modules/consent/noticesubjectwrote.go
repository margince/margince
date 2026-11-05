// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// subjectWroteNote is the ground a case closed by SettleWhenSubjectWroteTx
// carries. It names the evidence rather than the contact, so the row says why
// the duty ended without copying anything about them into it.
const subjectWroteNote = "They wrote to us: a captured mail from their own address shows the data came from them (Art. 13, not Art. 14), so no separate notice is owed."

// SettleableNoticeStates leaves deliveries in flight and completed outcomes intact.
func SettleableNoticeStates() []string {
	return []string{string(NoticeOpen), string(NoticeAssigned), string(NoticeBlocked), string(NoticeDeliveryFailed), string(NoticeSourceReview)}
}

// SettleWhenSubjectWroteTx ends the open duties recorded for these acquisitions,
// because the contact they belong to has since written to us.
//
// A backfill may capture our reply before the subject's own mail. The unknown
// source needs review until their authenticated mail answers it. Confirmed
// indirect sources and human assessments remain separate obligations.
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
	return settleAcquisitionCasesTx(ctx, tx, acquisitions, subjectWroteNote, now)
}

func settleAcquisitionCasesTx(ctx context.Context, tx pgx.Tx, acquisitions []ids.UUID, note string, now time.Time) (int, error) {
	if len(acquisitions) == 0 {
		return 0, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT id FROM privacy_notice_case
		 WHERE acquisition_id = ANY($1) AND state = ANY($2) AND source_kind IS NULL
		 ORDER BY id`, acquisitions, SettleableNoticeStates())
	if err != nil {
		return 0, fmt.Errorf("read the duties owed for these acquisitions: %w", err)
	}
	caseIDs, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return 0, fmt.Errorf("read the duties owed for these acquisitions: %w", err)
	}
	return closeOwedCasesTx(ctx, tx, caseIDs, note, now)
}

// closeOwedCasesTx ends each named case as exempt_with_reason on one ground,
// with one audit entry each, for the settlers that end a duty without a
// disclosure.
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
		if terminalNoticeStates()[current.State] || current.State == NoticeQueued || current.SourceKind != nil {
			continue
		}
		if err := attachOneAcquisition(ctx, tx, &current); err != nil {
			return closed, err
		}
		if current.Acquisition == nil {
			continue
		}
		duty, needsCase := DutyFor(current.Acquisition.Kind)
		if needsCase && duty.State != NoticeSourceReview {
			continue
		}
		out, err := scanNoticeCase(tx.QueryRow(ctx, `
			UPDATE privacy_notice_case
			   SET state = $2, rule = 'none', due_at = NULL, allowed_routes = '{}', resolution_note = $3, completed_at = $4,
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

// A failed delivery cannot revive a capture duty answered by direct-source evidence.
func settleFailedSenderDutiesTx(ctx context.Context, tx pgx.Tx, cases []ids.UUID, now time.Time) error {
	rows, err := tx.Query(ctx, `SELECT DISTINCT n.acquisition_id
 FROM privacy_notice_case n JOIN contact_acquisition_evidence e ON e.id = n.acquisition_id
 WHERE n.id = ANY(@cases) AND n.state = ANY(@states) AND e.kind = 'unknown_legacy'
 AND `+correspondence.CapturedAcquisitionSQL("e")+`
 AND EXISTS (SELECT 1 FROM contact_email ce
 JOIN activity_participant p ON lower(p.address) = ce.email AND p.role = 'from'
 JOIN activity a ON a.id = p.activity_id
 WHERE ce.contact_id = n.contact_id AND ce.archived_at IS NULL AND `+correspondence.SubjectMailSQL("a")+`)`,
		pgx.NamedArgs{"cases": cases, "states": SettleableNoticeStates()})
	if err != nil {
		return fmt.Errorf("read direct-source duties after delivery failed: %w", err)
	}
	acquisitions, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return err
	}
	_, err = SettleWhenSubjectWroteTx(ctx, tx, acquisitions, now)
	return err
}
