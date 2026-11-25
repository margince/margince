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

// ladderSuppressedClause holds for activity a when the counterparty ladder
// suppressed its message. Such a message is filed under nobody, whichever
// mailbox delivers it. The trace says so for the message itself. After the
// trace sweep, the disposition ledger still names every suppressed address,
// and the party the ladder judged is always one of the message's participants.
const ladderSuppressedClause = `(EXISTS (SELECT 1 FROM capture_trace t
	WHERE t.source_system = a.source_system AND t.source_id = a.source_id
	  AND t.stage = 'tier_ladder' AND t.outcome = 'suppressed')
 OR EXISTS (SELECT 1 FROM activity_participant sp
	JOIN capture_pending_counterparty d ON d.email = lower(btrim(sp.address)) AND d.status = 'suppressed'
	WHERE sp.activity_id = a.id))`

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
	addresses, err := joinedMailAddressesTx(ctx, tx, id, rec, fields)
	if err != nil || len(addresses) == 0 {
		return err
	}
	holders, err := storekit.LockIdentityHolders(ctx, tx, storekit.IdentityKeys{Emails: addresses})
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
	var filed int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM activity_link WHERE activity_id = $1`, id).Scan(&filed); err != nil {
		return fmt.Errorf("capture: counting a joined mail's links: %w", err)
	}
	_, err = fileUnderRecords(tx, records, participantFiling{
		decider: ctx, budget: max(maxDerivedMeetingLinks-filed, 0), private: private, write: insertParticipantLink(ctx, tx, id),
	})
	return err
}

// joinedMailAddressesTx answers the addresses a joined mail is filed by for
// this seat. A meeting, internal mail, a notes mail, a private thread, a
// restricted mail and a mail the ladder suppressed answer none.
func joinedMailAddressesTx(
	ctx context.Context, tx pgx.Tx, id ids.ActivityID, rec connector.NormalizedRecord, fields ActivityFields,
) ([]string, error) {
	if fields.Kind == meetingKind || internalMailAdmitted(ctx) || isMeetingNotesMail(rec) {
		return nil, nil
	}
	joined, err := seatHoldsImportTx(ctx, tx, id, actorUserID(ctx))
	if err != nil || !joined {
		return nil, err
	}
	if private, err := capturedThreadIsPrivateTx(ctx, tx, rec); err != nil || private {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT lower(ap.address) FROM activity a
		  JOIN activity_participant ap ON ap.activity_id = a.id
		 WHERE a.id = $1 AND a.restricted_at IS NULL AND NOT `+ladderSuppressedClause+`
		   AND ap.address IS NOT NULL AND NOT ap.past_party_cap`, id)
	if err != nil {
		return nil, fmt.Errorf("capture: reading the addresses on a joined mail: %w", err)
	}
	addresses, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("capture: reading the addresses on a joined mail: %w", err)
	}
	return addresses, nil
}
