// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ReplacedSeriesMeetingsTx answers the live meetings of seat's calendar that a
// split series replaced at meeting's slot. Such a row starts when meeting does
// and belongs to another series. It has the same organizer and one more party
// in common besides the seat. Two instances of one series moved onto one slot
// stay two meetings, and a meeting called off is left alone.
func ReplacedSeriesMeetingsTx(ctx context.Context, tx pgx.Tx, meeting ids.ActivityID, seat ids.UUID) ([]ids.ActivityID, error) {
	rows, err := tx.Query(ctx, `
		SELECT old.id FROM activity new
		  JOIN activity old ON old.source_system = new.source_system AND old.source_id <> new.source_id
		   AND old.occurred_at = new.occurred_at AND old.kind = 'meeting'
		   AND old.archived_at IS NULL AND old.restricted_at IS NULL
		   AND old.meeting_status IS DISTINCT FROM 'canceled'
		   AND `+auth.ConnectorSeatSQL("old")+` = $2::text
		 WHERE new.id = $1
		   AND EXISTS (SELECT 1 FROM activity_identity oi WHERE oi.activity_id = old.id AND oi.identity_kind = 'meeting')
		   AND NOT EXISTS (SELECT 1 FROM activity_identity oi
		                     JOIN activity_identity ni ON ni.activity_id = new.id AND ni.identity_kind = 'meeting'
		                      AND `+meetingUIDOfKeySQL("ni.identity_key")+` = `+meetingUIDOfKeySQL("oi.identity_key")+`
		                    WHERE oi.activity_id = old.id AND oi.identity_kind = 'meeting')
		   AND EXISTS (SELECT 1 FROM activity_participant o
		                 JOIN activity_participant n ON n.activity_id = new.id AND n.role = 'organizer'
		                  AND lower(n.address) = lower(o.address)
		                WHERE o.activity_id = old.id AND o.role = 'organizer')
		   AND EXISTS (SELECT 1 FROM activity_participant o
		                 JOIN activity_participant n ON n.activity_id = new.id AND n.role IN ('organizer', 'attendee')
		                  AND lower(n.address) = lower(o.address) AND n.user_id IS DISTINCT FROM $3
		                WHERE o.activity_id = old.id AND o.role IN ('organizer', 'attendee')
		                  AND o.user_id IS DISTINCT FROM $3
		                  AND NOT EXISTS (SELECT 1 FROM app_user u WHERE u.id = $3 AND lower(u.email) = lower(o.address)))
		 ORDER BY old.id`, meeting, seat.String(), seat)
	if err != nil {
		return nil, fmt.Errorf("activities: finding the meetings a split series replaced: %w", err)
	}
	replaced, err := pgx.CollectRows(rows, pgx.RowTo[ids.ActivityID])
	if err != nil {
		return nil, fmt.Errorf("activities: reading the meetings a split series replaced: %w", err)
	}
	return replaced, nil
}
