// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// An organizer's cancellation after a guest's false one writes nothing. The
// meeting is already cancelled, and a stored original is never replaced. So
// the repair reopens a meeting only while the guest's cancellation is the last
// thing recorded about it. No stored copy of it may read as cancelled either.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture/meetingmap"
	"github.com/margince/margince/backend/internal/platform/auth"
)

// cancellationIsTheLastSignTx answers whether no audit row and no import came
// after the cancellation. It also asks that no stored calendar copy of the
// meeting settles as cancelled under today's rules.
func cancellationIsTheLastSignTx(ctx context.Context, tx pgx.Tx, c activities.CalendarCancellation) (bool, error) {
	var later bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM audit_log au
 WHERE au.entity_type = 'activity' AND au.entity_id = $1 AND (au.occurred_at, au.id) > ($2, $3))
 OR EXISTS (SELECT 1 FROM capture_import ci WHERE ci.activity_id = $1 AND ci.imported_at > $2)`,
		c.ID, c.CancelledAt, c.AuditID).Scan(&later); err != nil {
		return false, fmt.Errorf("compose: reading what followed a calendar cancellation: %w", err)
	}
	if later {
		return false, nil
	}
	return noStoredCopyCancelledTx(ctx, tx, c)
}

// meetingCaptureKeySQL reads the key meeting $1 was captured under, and the
// seat whose calendar captured it.
var meetingCaptureKeySQL = `SELECT coalesce(a.source_system, ''), coalesce(a.source_id, ''), coalesce(` + auth.ConnectorSeatSQL("a") + `, '')
  FROM activity a WHERE a.id = $1 AND a.archived_at IS NULL AND a.restricted_at IS NULL`

// storedCopiesSQL lists each calendar copy stored for meeting $1, with the
// calendar it was read from. The copies are the key it was captured under ($2,
// $3, read by seat $4) and each event id a seat's import recorded.
const storedCopiesSQL = `SELECT copy.source_system, rc.payload, coalesce(owner.account_label, '')
  FROM (SELECT $2::text AS source_system, $3::text AS source_id, $4::text AS seat
        UNION
        SELECT split_part(alias, ':', 1), regexp_replace(alias, '^[^:]*:[^:]*:', ''), ci.user_id::text
          FROM capture_import ci, unnest(ci.provider_message_ids) alias WHERE ci.activity_id = $1) copy
  JOIN raw_capture rc ON rc.source_system = copy.source_system AND rc.source_id = copy.source_id
  LEFT JOIN LATERAL (SELECT c.account_label FROM capture_connection c
        WHERE c.provider = copy.source_system AND c.user_id::text = copy.seat LIMIT 1) owner ON true`

// noStoredCopyCancelledTx answers whether no stored calendar copy of the
// meeting settles as an organizer's cancellation.
func noStoredCopyCancelledTx(ctx context.Context, tx pgx.Tx, c activities.CalendarCancellation) (bool, error) {
	var system, id, seat string
	if err := tx.QueryRow(ctx, meetingCaptureKeySQL, c.ID).Scan(&system, &id, &seat); err != nil {
		return false, fmt.Errorf("compose: reading the key a meeting was captured under: %w", err)
	}
	rows, err := tx.Query(ctx, storedCopiesSQL, c.ID, system, id, seat)
	if err != nil {
		return false, fmt.Errorf("compose: reading a meeting's stored calendar copies: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var source, owner string
		var payload []byte
		if err := rows.Scan(&source, &payload, &owner); err != nil {
			return false, fmt.Errorf("compose: reading a meeting's stored calendar copies: %w", err)
		}
		if settlement, readable := storedMeetingSettlement(source, owner, payload); readable && settlement == meetingmap.SettleCancel {
			return false, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("compose: reading a meeting's stored calendar copies: %w", err)
	}
	return true, nil
}
