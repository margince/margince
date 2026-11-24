// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

var _ connector.UnlistedMeetingRetirer = (*Sink)(nil)

// RetireUnlistedMeetings settles each stored meeting a complete listing of
// the syncing connection no longer holds. A calendar sends nothing for an
// instance a series split or a shortened series dropped, so a full listing is
// the only word on it. An omission is no cancellation: a meeting another seat
// imports loses only this seat, and one the seat's other connection still
// holds stays. Only a meeting no other calendar contributes is cancelled.
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
			if m.heldElsewhere {
				continue
			}
			key := connector.NaturalKey{SourceSystem: sourceSystem, SourceID: m.event}
			if err := s.removeCalendarCopyTx(ctx, tx, m.id, key, m.start, !m.otherImporter); err != nil {
				return err
			}
		}
		return nil
	})
}

// unlistedMeeting is one stored meeting a listing left out, under this
// connection's own event id. heldElsewhere says another live connection of
// the seat still holds it; otherImporter says another seat or mailbox does.
type unlistedMeeting struct {
	id                           ids.ActivityID
	event                        string
	start                        time.Time
	heldElsewhere, otherImporter bool
}

// unlistedMeetingsTx answers the open meetings whose event id this connection
// recorded on the seat's import, which the listing left out, starting within
// the window. A run bound to no connection answers none.
func unlistedMeetingsTx(
	ctx context.Context, tx pgx.Tx, sourceSystem string, listed []string, from, until time.Time,
) ([]unlistedMeeting, error) {
	run := runConnection(ctx)
	if run == nil {
		return nil, nil
	}
	rows, err := tx.Query(ctx, `
		WITH conn AS (SELECT `+storekit.MailboxMarkSQL("c")+` || ':' AS mark FROM capture_connection c
		               WHERE c.id = $3 AND c.user_id = $1 AND c.provider = $2)
		SELECT DISTINCT ON (a.id) a.id, substr(k.key, length(conn.mark) + 1), a.occurred_at,
		       EXISTS (SELECT 1 FROM capture_connection c2, unnest(i.provider_message_ids) o(key)
		                WHERE c2.user_id = $1 AND c2.id <> $3 AND c2.archived_at IS NULL
		                  AND c2.status IN ('connected', 'error')
		                  AND starts_with(o.key, `+storekit.MailboxMarkSQL("c2")+` || ':')),
		       EXISTS (SELECT 1 FROM capture_import other WHERE other.activity_id = a.id AND other.user_id IS DISTINCT FROM $1)
		  FROM conn
		  JOIN capture_import i ON i.user_id = $1
		 CROSS JOIN LATERAL unnest(i.provider_message_ids) k(key)
		  JOIN activity a ON a.id = i.activity_id
		 WHERE starts_with(k.key, conn.mark) AND a.kind = 'meeting'
		   AND a.archived_at IS NULL AND a.restricted_at IS NULL
		   AND (a.meeting_status IS NULL OR a.meeting_status = 'booked')
		   AND a.occurred_at > $4 AND a.occurred_at <= $5
		   AND NOT EXISTS (SELECT 1 FROM unnest(i.provider_message_ids) l(key)
		                    WHERE starts_with(l.key, conn.mark) AND substr(l.key, length(conn.mark) + 1) = ANY($6::text[]))
		 ORDER BY a.id`, actorUserID(ctx), sourceSystem, *run, from, until, listed)
	if err != nil {
		return nil, fmt.Errorf("capture: finding the meetings a calendar listing left out: %w", err)
	}
	unlisted, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (unlistedMeeting, error) {
		var m unlistedMeeting
		return m, row.Scan(&m.id, &m.event, &m.start, &m.heldElsewhere, &m.otherImporter)
	})
	if err != nil {
		return nil, fmt.Errorf("capture: reading the meetings a calendar listing left out: %w", err)
	}
	return unlisted, nil
}
