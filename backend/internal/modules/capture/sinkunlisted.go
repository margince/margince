// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

var _ connector.UnlistedMeetingRetirer = (*Sink)(nil)

// RetireUnlistedMeetings settles each stored meeting a complete listing of the
// seat's calendar no longer holds, as a removal of the seat's copy. A calendar
// sends nothing for an instance a series split or a shortened series dropped,
// so a full listing is the only word on it. A meeting no other seat imports is
// cancelled: no calendar holds it any more.
func (s *Sink) RetireUnlistedMeetings(ctx context.Context, sourceSystem string, listed []string, from, until time.Time) error {
	if err := admitCalendarProvider(ctx, sourceSystem, "retire"); err != nil {
		return err
	}
	if _, calendar := calendarWriteScopes[sourceSystem]; !calendar {
		return fmt.Errorf("capture: %q is no calendar, so it lists no meetings to retire", sourceSystem)
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		ctx, err := s.mailboxRunAllowedTx(ctx, tx)
		if err != nil {
			return err
		}
		unlisted, err := unlistedMeetingsTx(ctx, tx, sourceSystem, listed, from, until)
		if err != nil {
			return err
		}
		for _, m := range unlisted {
			key := connector.NaturalKey{SourceSystem: sourceSystem, SourceID: m.event}
			if err := s.removeCalendarCopyTx(ctx, tx, m.id, key, m.start, m.alone); err != nil {
				return err
			}
		}
		return nil
	})
}

// unlistedMeeting is one stored meeting a listing left out. alone says no
// other seat imports it.
type unlistedMeeting struct {
	id    ids.ActivityID
	event string
	start time.Time
	alone bool
}

// unlistedMeetingsTx answers the open meetings this seat's connection stored
// under an event id the listing left out, starting within the window.
func unlistedMeetingsTx(
	ctx context.Context, tx pgx.Tx, sourceSystem string, listed []string, from, until time.Time,
) ([]unlistedMeeting, error) {
	seat := actorUserID(ctx)
	keys := storekit.ProviderMessageKeysSQL("$1", "$2", "$3", "a.source_id")
	rows, err := tx.Query(ctx, `
		SELECT a.id, a.source_id, a.occurred_at,
		       NOT EXISTS (SELECT 1 FROM capture_import other WHERE other.activity_id = a.id AND other.user_id IS DISTINCT FROM $1)
		  FROM activity a
		  JOIN capture_import i ON i.activity_id = a.id AND i.user_id = $1
		 WHERE a.source_system = $2 AND a.kind = 'meeting'
		   AND a.archived_at IS NULL AND a.restricted_at IS NULL
		   AND (a.meeting_status IS NULL OR a.meeting_status = 'booked')
		   AND a.occurred_at > $4 AND a.occurred_at <= $5
		   AND `+auth.ConnectorSeatSQL("a")+` = $1::uuid::text
		   AND a.source_id <> ALL($6::text[])
		   AND i.provider_message_ids @> `+keys+` AND cardinality(`+keys+`) > 0
		 ORDER BY a.id`, seat, sourceSystem, runConnection(ctx), from, until, listed)
	if err != nil {
		return nil, fmt.Errorf("capture: finding the meetings a calendar listing left out: %w", err)
	}
	unlisted, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (unlistedMeeting, error) {
		var m unlistedMeeting
		return m, row.Scan(&m.id, &m.event, &m.start, &m.alone)
	})
	if err != nil {
		return nil, fmt.Errorf("capture: reading the meetings a calendar listing left out: %w", err)
	}
	return unlisted, nil
}
