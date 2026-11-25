// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// joinActivityTx completes a capture that landed on a row already filed.
func (s *Sink) joinActivityTx(
	ctx context.Context, tx pgx.Tx, id ids.ActivityID,
	rec connector.NormalizedRecord, fields ActivityFields, birth birthDecision, joined joinedCapture,
) error {
	heldBefore, err := seatHoldsImportTx(ctx, tx, id, actorUserID(ctx))
	if err != nil {
		return err
	}
	if err := s.finishJoinedActivity(ctx, tx, id, rec, fields, birth, joined); err != nil {
		return err
	}
	if heldBefore {
		return nil
	}
	return s.fileJoinedMailForSeatTx(ctx, tx, id, rec, fields)
}

// ladderSuppressedClause holds for activity a when the counterparty ladder
// suppressed its message, given subject, the SQL expression for the address the
// ladder judged. Such a message is filed under nobody, whichever mailbox
// delivers it. The trace says so for the message itself, and the disposition
// ledger still says so for the judged address after the trace sweep.
func ladderSuppressedClause(subject string) string {
	return `(EXISTS (SELECT 1 FROM capture_trace t
	WHERE t.source_system = a.source_system AND t.source_id = a.source_id
	  AND t.stage = 'tier_ladder' AND t.outcome = 'suppressed')
 OR EXISTS (SELECT 1 FROM capture_pending_counterparty d
	WHERE d.email = lower(btrim(` + subject + `)) AND d.status = 'suppressed'))`
}

// seatHoldsImportTx answers whether the seat has an import row on the activity.
func seatHoldsImportTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID, seat ids.UUID) (bool, error) {
	var held bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM capture_import WHERE activity_id = $1 AND user_id = $2)`, id, seat).Scan(&held); err != nil {
		return false, fmt.Errorf("capture: reading the seat's import of an activity: %w", err)
	}
	return held, nil
}

// fileJoinedMailForSeatTx files a mail another mailbox captured first under
// the records that hold an address on it and that this seat may see. The first
// mailbox filed it under its own seat's authority, which cannot see a contact
// private to this seat. Without this, the order of two syncs decides whether
// the mail reaches this seat's contact. A join creates no record: the first
// capture's ladder already judged the parties.
func (s *Sink) fileJoinedMailForSeatTx(
	ctx context.Context, tx pgx.Tx, id ids.ActivityID, rec connector.NormalizedRecord, fields ActivityFields,
) error {
	joined, err := s.joinedMailTx(ctx, tx, id, rec, fields)
	if err != nil || len(joined.addresses) == 0 {
		return err
	}
	holders, err := storekit.LockIdentityHolders(ctx, tx, storekit.IdentityKeys{Emails: joined.addresses})
	if err != nil {
		return fmt.Errorf("capture: locking the addresses on a joined mail: %w", err)
	}
	var targets storekit.LastActivityTargets
	records := make([]participantRecord, 0, len(holders.Contacts)+len(holders.Leads))
	for _, contact := range holders.Contacts {
		targets.Contacts = append(targets.Contacts, contact.UUID)
		records = append(records, participantRecord{entity: datasource.EntityContact, id: contact.UUID})
	}
	for _, lead := range holders.Leads {
		records = append(records, participantRecord{entity: datasource.EntityLead, id: lead.UUID})
	}
	if err := storekit.LockLastActivityTargets(ctx, tx, targets); err != nil {
		return fmt.Errorf("capture: %w", err)
	}
	private, _, err := capturedMessageIsPrivateTx(ctx, tx, rec, fields)
	if err != nil {
		return err
	}
	_, err = fileUnderRecords(tx, records, participantFiling{
		decider: ctx, budget: max(maxDerivedMeetingLinks-joined.filed-laterLinkReservation, 0),
		private: private, write: insertParticipantLink(ctx, tx, id),
	})
	return err
}

// joinedMail is what a join files by: the addresses on the message, and how
// many links the activity holds already.
type joinedMail struct {
	addresses []string
	filed     int
}

// joinedMailTx answers what a joined mail is filed by for this seat. It answers
// no addresses for a meeting, internal mail, a notes mail or a private thread.
// Nor for a restricted or suppressed mail, or one this seat did not import.
func (s *Sink) joinedMailTx(
	ctx context.Context, tx pgx.Tx, id ids.ActivityID, rec connector.NormalizedRecord, fields ActivityFields,
) (joinedMail, error) {
	if fields.Kind == meetingKind || internalMailAdmitted(ctx) || isMeetingNotesMail(rec) {
		return joinedMail{}, nil
	}
	if private, err := capturedThreadIsPrivateTx(ctx, tx, rec); err != nil || private {
		return joinedMail{}, err
	}
	// The party this message's ladder judges, which on an introduction is a
	// copied outsider rather than the stored counterparty.
	subject, err := s.ladderSubjectTx(ctx, tx, rec)
	if err != nil {
		return joinedMail{}, err
	}
	var joined joinedMail
	err = tx.QueryRow(ctx, `
		SELECT coalesce(array_agg(DISTINCT lower(ap.address)), '{}'),
		       (SELECT count(*) FROM activity_link l WHERE l.activity_id = a.id)
		  FROM activity a
		  JOIN activity_participant ap ON ap.activity_id = a.id
		       AND ap.address IS NOT NULL AND NOT ap.past_party_cap
		 WHERE a.id = $1 AND a.restricted_at IS NULL AND NOT `+ladderSuppressedClause("$2::text")+`
		   AND EXISTS (SELECT 1 FROM capture_import ci WHERE ci.activity_id = a.id AND ci.user_id = $3)
		 GROUP BY a.id`, id, subject.Email, actorUserID(ctx)).Scan(&joined.addresses, &joined.filed)
	if errors.Is(err, pgx.ErrNoRows) {
		return joinedMail{}, nil
	}
	if err != nil {
		return joinedMail{}, fmt.Errorf("capture: reading what a joined mail is filed by: %w", err)
	}
	return joined, nil
}
