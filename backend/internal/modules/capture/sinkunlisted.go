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
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

var _ connector.UnlistedMeetingRetirer = (*Sink)(nil)

// RetireUnlistedMeetings settles each stored meeting a complete listing of
// the syncing connection no longer holds. A calendar sends nothing for an
// instance a series split or a shortened series dropped, so a full listing is
// the only word on it. An omission is no cancellation. A meeting that another
// seat or shared mailbox imports and is still a guest of loses only this seat.
// One the seat's other connection still holds stays.
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
		retiring := make([]ids.UUID, 0, len(unlisted))
		for _, m := range unlisted {
			if !m.heldElsewhere {
				retiring = append(retiring, m.id.UUID)
			}
		}
		// Every meeting import takes these locks before it writes its import row,
		// so an import racing this listing has committed once they are held.
		if err := storekit.LockMeetingAttendeePage(ctx, tx, retiring); err != nil {
			return err
		}
		for _, m := range unlisted {
			if m.heldElsewhere {
				continue
			}
			kept, err := anotherGuestImportsTx(ctx, tx, m.id, actorUserID(ctx))
			if err != nil {
				return err
			}
			key := connector.NaturalKey{SourceSystem: sourceSystem, SourceID: m.event}
			if err := s.removeCalendarCopyTx(ctx, tx, m.id, key, m.start, !kept); err != nil {
				return err
			}
		}
		return nil
	})
}

// unlistedMeeting is one stored meeting a listing left out, under this
// connection's own event id. heldElsewhere says another live connection of
// the seat still holds it.
type unlistedMeeting struct {
	id            ids.ActivityID
	event         string
	start         time.Time
	heldElsewhere bool
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
		               WHERE c.id = $3 AND c.user_id IS NOT NULL AND c.user_id = $1 AND c.provider = $2)
		SELECT DISTINCT ON (a.id) a.id, substr(k.key, length(conn.mark) + 1), a.occurred_at,
		       EXISTS (SELECT 1 FROM capture_connection c2, unnest(i.provider_message_ids) o(key)
		                WHERE c2.user_id = $1 AND c2.id <> $3 AND c2.archived_at IS NULL
		                  AND c2.status IN ('connected', 'error')
		                  AND starts_with(o.key, `+storekit.MailboxMarkSQL("c2")+` || ':'))
		  FROM conn
		  JOIN capture_import i ON i.user_id = $1
		 CROSS JOIN LATERAL unnest(i.provider_message_ids) k(key)
		  JOIN activity a ON a.id = i.activity_id
		 WHERE starts_with(k.key, conn.mark) AND a.kind = 'meeting'
		   AND a.archived_at IS NULL AND a.restricted_at IS NULL
		   AND (a.meeting_status IS NULL OR a.meeting_status = 'booked')
		   AND `+occurredday.AheadSQL("a", "$4::timestamptz")+`
		   AND a.occurred_at <= $5 -- occurred_at raw: the capture horizon, which capture bounds stored starts by
		   AND NOT EXISTS (SELECT 1 FROM unnest(i.provider_message_ids) l(key)
		                    WHERE starts_with(l.key, conn.mark) AND substr(l.key, length(conn.mark) + 1) = ANY($6::text[]))
		 ORDER BY a.id`, actorUserID(ctx), sourceSystem, *run, from, until, listed)
	if err != nil {
		return nil, fmt.Errorf("capture: finding the meetings a calendar listing left out: %w", err)
	}
	unlisted, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (unlistedMeeting, error) {
		var m unlistedMeeting
		return m, row.Scan(&m.id, &m.event, &m.start, &m.heldElsewhere)
	})
	if err != nil {
		return nil, fmt.Errorf("capture: reading the meetings a calendar listing left out: %w", err)
	}
	return unlisted, nil
}

// anotherGuestImportsTx reports whether a seat other than seat, or a shared
// mailbox, imports the meeting and is still on its guest list. An import row
// outlives its seat's leaving, because it still decides who reads the
// meeting, so the guest list says who still holds it. A shared mailbox has no
// seat and is a guest by its address.
func anotherGuestImportsTx(ctx context.Context, tx pgx.Tx, meeting ids.ActivityID, seat ids.UUID) (bool, error) {
	var kept bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM capture_import other
		  JOIN activity_participant guest ON guest.activity_id = other.activity_id
		  LEFT JOIN capture_connection box ON box.id = other.connection_id AND box.user_id IS NULL
		 WHERE other.activity_id = $1 AND other.user_id IS DISTINCT FROM $2
		   AND (guest.user_id = other.user_id OR lower(guest.address) = lower(box.account_label)))`,
		meeting, seat).Scan(&kept); err != nil {
		return false, fmt.Errorf("capture: reading whether another guest imports a meeting: %w", err)
	}
	return kept, nil
}
