// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
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
// the records that already hold its counterparty's address and that this seat
// may see. The first mailbox filed it under its own seat's authority, which
// cannot see a contact private to this seat. Without this, the order of two
// syncs decides whether the mail reaches this seat's contact. A join creates
// no record: the first capture's ladder already judged the party.
func (s *Sink) fileJoinedMailForSeatTx(
	ctx context.Context, tx pgx.Tx, id ids.ActivityID, rec connector.NormalizedRecord, fields ActivityFields,
) error {
	if fields.Kind == meetingKind || internalMailAdmitted(ctx) || isMeetingNotesMail(rec) {
		return nil
	}
	joined, err := seatHoldsImportTx(ctx, tx, id, actorUserID(ctx))
	if err != nil || !joined {
		return err
	}
	subject, err := s.ladderSubjectTx(ctx, tx, rec)
	if err != nil || subject.Email == "" {
		return err
	}
	if private, err := capturedThreadIsPrivateTx(ctx, tx, rec); err != nil || private {
		return err
	}
	holders, err := storekit.LockIdentityHolders(ctx, tx, storekit.IdentityKeys{Emails: []string{normalizeEmail(subject.Email)}})
	if err != nil {
		return fmt.Errorf("capture: locking the counterparty's address: %w", err)
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
		decider: ctx, budget: laterLinkReservation, private: private, write: insertParticipantLink(ctx, tx, id),
	})
	return err
}
