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
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// subjectWroteNote is the ground a case closed by SettleWhenSubjectWroteTx
// carries. It names the evidence rather than the contact, so the row says why
// the duty ended without copying anything about them into it.
const subjectWroteNote = "They wrote to us: a captured mail from their own address shows the data came from them (Art. 13, not Art. 14), so no separate notice is owed."

// SubjectMailFromSQL is the `FROM` clause of the subject mail contact (a SQL
// expression) wrote, as activity a and its sender row p. The sender is read by
// relstrength.ParticipantNamesSQL, so a moved address keeps its older mail.
func SubjectMailFromSQL(contact string) string {
	return relstrength.ParticipantRowsOfContactSQL(contact) + ` p JOIN activity a ON a.id = p.activity_id
 WHERE p.role = 'from' AND ` + correspondence.SubjectMailSQL("a")
}

// SubjectReachedOutSQL names evidence of this contact's own approach. An
// unanswered invitation or mail we sent provides no such evidence. A public
// booking names only its booker, never the other people invited to the room.
func SubjectReachedOutSQL(activity, contact string) string {
	party := relstrength.ParticipantRowsOfContactSQL(contact)
	mail := `EXISTS (SELECT 1 FROM ` + party + ` sp WHERE sp.activity_id = ` + activity + `.id AND sp.role = 'from')`
	meeting := `EXISTS (SELECT 1 FROM ` + party + ` sp WHERE sp.activity_id = ` + activity + `.id AND sp.user_id IS NULL AND (sp.role = 'organizer' OR (sp.role = 'attendee' AND sp.response = 'accepted')))`
	booking := `EXISTS (SELECT 1 FROM meeting_invitation mi WHERE mi.activity_id = ` + activity + `.id AND mi.appointment->>'ContactID' = (` + contact + `)::text)`
	return `(` + activity + `.archived_at IS NULL AND ((` + correspondence.SubjectMailSQL(activity) + ` AND ` + mail + `) OR (` + activity + `.kind = 'meeting' AND (` + meeting + ` OR (` + activity + `.source = 'public_booking' AND ` + booking + `)))))`
}

// SubjectReachedOutAtSQL dates a booking when it was recorded, rather than at
// the future time the meeting will start.
func SubjectReachedOutAtSQL(activity string) string {
	return `CASE WHEN ` + activity + `.kind = 'meeting' THEN ` + activity + `.created_at ELSE ` + activity + `.occurred_at END`
}

// SubjectReachedOutFromSQL returns the activities proving a contact approached
// us. Every opener, settler and repair reads the same evidence rule.
func SubjectReachedOutFromSQL(contact string) string {
	return `activity a WHERE ` + SubjectReachedOutSQL("a", contact)
}

// SettleWhenSubjectReachedOutTx closes every still-owed source on the named
// activity's evidence. Assessed sources remain recorded on the closed case.
func SettleWhenSubjectReachedOutTx(ctx context.Context, tx pgx.Tx, acquisitions []ids.UUID, activity ids.UUID, now time.Time) (int, error) {
	rows, err := tx.Query(ctx, `SELECT id FROM privacy_notice_case WHERE acquisition_id = ANY($1) AND state = ANY($2) ORDER BY id`, acquisitions, SettleableNoticeStates())
	if err != nil {
		return 0, err
	}
	cases, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return 0, err
	}
	note := subjectReachedOutNote(activity)
	return closeNoticeCasesTx(ctx, tx, cases, note, now, false)
}

func subjectReachedOutNote(activity ids.UUID) string {
	return "The contact reached out to us themselves; their mail or meeting is recorded as activity " + activity.String() + ". No separate privacy notice is owed."
}

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
	return closeNoticeCasesTx(ctx, tx, caseIDs, note, now, true)
}

func closeNoticeCasesTx(ctx context.Context, tx pgx.Tx, caseIDs []ids.UUID, note string, now time.Time, unverifiedOnly bool) (int, error) {
	closed := 0
	for _, id := range caseIDs {
		current, err := lockNoticeCase(ctx, tx, id)
		if err != nil {
			return closed, err
		}
		if terminalNoticeStates()[current.State] || current.State == NoticeQueued || (unverifiedOnly && current.SourceKind != nil) {
			continue
		}
		if err := attachOneAcquisition(ctx, tx, &current); err != nil {
			return closed, err
		}
		if current.Acquisition == nil {
			continue
		}
		duty, needsCase := DutyFor(current.Acquisition.Kind)
		if unverifiedOnly && needsCase && duty.State != NoticeSourceReview {
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
 WHERE n.id = ANY(@cases) AND n.state = ANY(@states)
 AND EXISTS (SELECT 1 FROM `+SubjectReachedOutFromSQL("n.contact_id")+`)`,
		pgx.NamedArgs{"cases": cases, "states": SettleableNoticeStates()})
	if err != nil {
		return fmt.Errorf("read direct-source duties after delivery failed: %w", err)
	}
	acquisitions, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return err
	}
	for _, acquisition := range acquisitions {
		var activity ids.UUID
		if err := tx.QueryRow(ctx, `SELECT a.id FROM contact_acquisition_evidence e CROSS JOIN LATERAL (SELECT a.id FROM `+SubjectReachedOutFromSQL("e.contact_id")+` ORDER BY a.occurred_at, a.id LIMIT 1) a WHERE e.id = $1`, acquisition).Scan(&activity); err != nil {
			return err
		}
		if _, err := SettleWhenSubjectReachedOutTx(ctx, tx, []ids.UUID{acquisition}, activity, now); err != nil {
			return err
		}
	}
	err = nil
	return err
}
