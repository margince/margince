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
		if err != nil || len(unlisted) == 0 {
			return err
		}
		page := make([]ids.UUID, len(unlisted))
		for at, m := range unlisted {
			page[at] = m.id.UUID
		}
		// Every meeting import takes these locks before it writes its import row,
		// so an import racing this listing has committed once they are held.
		if err := storekit.LockMeetingAttendeePage(ctx, tx, page); err != nil {
			return err
		}
		for _, m := range unlisted {
			held, err := meetingHoldersTx(ctx, tx, m.id, actorUserID(ctx), runConnection(ctx))
			if err != nil {
				return err
			}
			if held.elsewhere {
				continue
			}
			key := connector.NaturalKey{SourceSystem: sourceSystem, SourceID: m.event}
			if err := s.removeCalendarCopyTx(ctx, tx, m.id, key, m.start, !held.byAnotherGuest); err != nil {
				return err
			}
		}
		return nil
	})
}

// unlistedMeeting is one stored meeting a listing left out, under this
// connection's own event id.
type unlistedMeeting struct {
	id    ids.ActivityID
	event string
	start time.Time
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
		SELECT DISTINCT ON (a.id) a.id, substr(k.key, length(conn.mark) + 1), a.occurred_at
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
		return m, row.Scan(&m.id, &m.event, &m.start)
	})
	if err != nil {
		return nil, fmt.Errorf("capture: reading the meetings a calendar listing left out: %w", err)
	}
	return unlisted, nil
}

// meetingHolders says who besides one connection still holds a meeting.
// elsewhere is another live connection of the seat whose events the seat's
// import records. byAnotherGuest is another seat, or a shared mailbox, that
// imports it and is still on its guest list.
type meetingHolders struct {
	elsewhere, byAnotherGuest bool
}

// meetingHoldersTx is asked under the meeting's locks. An import row outlives
// its seat's leaving, because it still decides who reads the meeting, so the
// guest list says who still holds it. A shared mailbox has no seat and is a
// guest by its address, matched on its folded domain as a seat's login is.
func meetingHoldersTx(ctx context.Context, tx pgx.Tx, meeting ids.ActivityID, seat ids.UUID, run *ids.UUID) (meetingHolders, error) {
	var mailboxes []string
	if err := tx.QueryRow(ctx, `SELECT coalesce(array_agg(lower(btrim(box.account_label))), '{}') FROM capture_import other
		  JOIN capture_connection box ON box.id = other.connection_id AND box.user_id IS NULL
		 WHERE other.activity_id = $1 AND nullif(btrim(box.account_label), '') IS NOT NULL`, meeting).Scan(&mailboxes); err != nil {
		return meetingHolders{}, fmt.Errorf("capture: reading the shared mailboxes that import a meeting: %w", err)
	}
	var held meetingHolders
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM capture_import i, capture_connection c2, unnest(i.provider_message_ids) o(key)
		                WHERE i.activity_id = $1 AND i.user_id = $2
		                  AND c2.user_id = $2 AND c2.id <> $3
		                  AND c2.status IN ('connected', 'error')
		                  AND starts_with(o.key, `+storekit.MailboxMarkSQL("c2")+` || ':')),
		       EXISTS (SELECT 1 FROM capture_import other
		                 JOIN activity_participant guest ON guest.activity_id = other.activity_id AND guest.user_id = other.user_id
		                 JOIN activity act ON act.id = other.activity_id
		                WHERE other.activity_id = $1 AND other.user_id <> $2 AND `+meetingPartySaidSQL("guest")+`
		                  AND `+auth.ParticipantStandsSQL("guest", "act")+`)
		       OR EXISTS (SELECT 1 FROM activity_participant guest, unnest($4::text[], $5::text[]) box(email, email_domain_folded)
		                   WHERE guest.activity_id = $1 AND `+meetingPartySaidSQL("guest")+`
		                     AND `+storekit.SeatLoginIsSQL("box", "lower(guest.address)", "guest.address_domain_folded")+`)`,
		meeting, seat, run, mailboxes, storedDomainsOf(mailboxes)).Scan(&held.elsewhere, &held.byAnotherGuest); err != nil {
		return meetingHolders{}, fmt.Errorf("capture: reading who else holds a meeting: %w", err)
	}
	return held, nil
}
