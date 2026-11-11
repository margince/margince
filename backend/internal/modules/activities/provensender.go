// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Naming the author of a received message after it was stored, and only on
// proof. This covers a contact linked later and a message stored before its
// sender had a participant row. The contact gets the `from` role only when the
// stated sender address is theirs and was never another contact's.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// auditFieldProvenSender names the contact a write stamped as the proven
// sender, in the audit images.
const auditFieldProvenSender = "proven_sender"

// evidenceDataTaskRun names the data task run that made a repair, in the audit
// evidence of every activity repair a data task writes.
const evidenceDataTaskRun = "data_task_run"

// provenSenderSQL holds when contact (a SQL expression) is the proven author of
// the received, live, unheld activity a, linked to it and naming no sender. The
// stated address is the bare `from` address, else the counterparty, and it
// must prove the contact (relstrength.AddressProvesSQL).
func provenSenderSQL(contact string) string {
	return `a.direction = 'inbound' AND a.archived_at IS NULL AND a.restricted_at IS NULL
	   AND a.kind IN (` + relstrength.ParticipantKindSQLList() + `)
	   AND EXISTS (SELECT 1 FROM activity_link sl WHERE sl.activity_id = a.id AND sl.contact_id = ` + contact + `)
	   AND NOT EXISTS (SELECT 1 FROM activity_participant sf WHERE sf.activity_id = a.id AND sf.role = 'from'
	         AND (sf.contact_id IS NOT NULL OR sf.user_id IS NOT NULL OR sf.address IS NULL))
	   AND EXISTS (
	       SELECT 1 FROM (SELECT CASE
	                WHEN EXISTS (SELECT 1 FROM activity_participant sf WHERE sf.activity_id = a.id AND sf.role = 'from')
	                THEN (SELECT CASE WHEN count(DISTINCT lower(sf.address)) = 1 THEN min(lower(sf.address)) END
	                        FROM activity_participant sf WHERE sf.activity_id = a.id AND sf.role = 'from')
	                ELSE lower(nullif(btrim(a.counterparty_email), '')) END AS address) stated
	        WHERE ` + relstrength.AddressProvesSQL(contact, "stated.address") + `)`
}

// stampProvenSender is StampProvenSendersTx for one activity, reporting
// whether it wrote.
func stampProvenSender(ctx context.Context, tx pgx.Tx, activityID, contact ids.UUID) (bool, error) {
	stamped, err := StampProvenSendersTx(ctx, tx, []ids.UUID{activityID}, contact)
	return len(stamped) > 0, err
}

// StampProvenSendersTx makes contact the sender of each activity where
// provenSenderSQL holds, and answers the activities it wrote. The contact's own
// copy row becomes the `from` row, so one contact is not both author and copy
// of one message. Every writer that names a sender after the fact calls this;
// the caller holds the activities and audits the change.
func StampProvenSendersTx(ctx context.Context, tx pgx.Tx, activityIDs []ids.UUID, contact ids.UUID) ([]ids.UUID, error) {
	rows, err := tx.Query(ctx, `
		WITH proven AS (SELECT a.id FROM activity a WHERE a.id = ANY($1) AND `+provenSenderSQL("$2::uuid")+`),
		promoted AS (
		  UPDATE activity_participant ap SET role = 'from'
		    FROM proven
		   WHERE ap.activity_id = proven.id AND ap.contact_id = $2 AND ap.role = 'cc'
		     AND ap.user_id IS NULL AND ap.address IS NULL AND ap.channel_user_id IS NULL
		  RETURNING ap.activity_id),
		inserted AS (
		  INSERT INTO activity_participant (activity_id, contact_id, role)
		  SELECT proven.id, $2, 'from' FROM proven
		   WHERE NOT EXISTS (SELECT 1 FROM promoted WHERE promoted.activity_id = proven.id)
		  ON CONFLICT DO NOTHING
		  RETURNING activity_id)
		SELECT activity_id FROM promoted UNION SELECT activity_id FROM inserted`,
		activityIDs, contact)
	if err != nil {
		return nil, fmt.Errorf("activities: naming the proven sender of %d messages: %w", len(activityIDs), err)
	}
	stamped, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("activities: naming the proven sender of %d messages: %w", len(activityIDs), err)
	}
	return stamped, nil
}

// provenLinkSQL holds for an activity a that has a linked contact
// provenSenderSQL proves to be its sender.
func provenLinkSQL() string {
	return `EXISTS (SELECT 1 FROM activity_link l
	         WHERE l.activity_id = a.id AND l.contact_id IS NOT NULL AND ` + provenSenderSQL("l.contact_id") + `)`
}

// ProvenSenderActivitiesTx locks the next limit activities after the cursor,
// in id order, whose sender a linked contact is proven to be.
func ProvenSenderActivitiesTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) ([]ids.UUID, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT a.id FROM activity a WHERE a.id > $1 AND a.restricted_at IS NULL AND `+provenLinkSQL()+`
		 ORDER BY a.id LIMIT $2 FOR UPDATE OF a`, after, limit)
	if err != nil {
		return nil, fmt.Errorf("activities: reading received messages with a proven sender: %w", err)
	}
	found, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("activities: reading received messages with a proven sender: %w", err)
	}
	return found, nil
}

// CountProvenSenderActivities counts the activities the sender repair would
// stamp, reading at most maximum+1. It is the repair's preview.
func CountProvenSenderActivities(ctx context.Context, q datatask.Querier, maximum int64) (int64, error) {
	var count int64
	if err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 FROM activity a WHERE a.restricted_at IS NULL AND `+provenLinkSQL()+`
		 LIMIT $1) proven`, maximum+1).Scan(&count); err != nil {
		return 0, fmt.Errorf("activities: counting received messages with a proven sender: %w", err)
	}
	return count, nil
}

// StampProvenSenderTx names the proven sender of one stored activity. The rule
// is read again under the row lock, so a replay changes nothing.
func StampProvenSenderTx(ctx context.Context, tx pgx.Tx, activityID, runID ids.UUID) (bool, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return false, err
	}
	if _, err := storekit.LockRow(ctx, tx, "activity", activityID, storekit.LiveOnly); err != nil {
		return false, err
	}
	var contact ids.UUID
	err := tx.QueryRow(ctx, `SELECT l.contact_id FROM activity a JOIN activity_link l ON l.activity_id = a.id
		 WHERE a.id = $1 AND l.contact_id IS NOT NULL AND `+provenSenderSQL("l.contact_id"), activityID).Scan(&contact)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("activities: reading the proven sender of %s: %w", activityID, err)
	}
	stamped, err := stampProvenSender(ctx, tx, activityID, contact)
	if err != nil || !stamped {
		return false, err
	}
	auditID, err := storekit.AuditWithEvidence(ctx, tx, "update", "activity", activityID,
		map[string]any{auditFieldProvenSender: nil}, map[string]any{auditFieldProvenSender: contact},
		map[string]any{evidenceDataTaskRun: runID.String()})
	if err != nil {
		return false, err
	}
	// The interaction graph refolds an activity's participants on this event.
	if err := storekit.EmitEvent(ctx, tx, auditID, activityID, crmcontracts.PublicEventActivityUpdated{}); err != nil {
		return false, err
	}
	return true, nil
}
