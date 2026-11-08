// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Reopening a meeting a calendar sync cancelled although only one attendee
// declined it or removed their own copy.
//
// The reopen mirrors the cancellation. The column goes back to what the
// cancellation's audit row says it held, with its own audit row, event and
// transition. The transition names the calendar event whose cancellation it
// reverses. The history keeps both rows, and a reader can leave the pair out.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// CalendarCancellationReopened names a reopen in its audit row's evidence.
const CalendarCancellationReopened = "calendar_cancellation_a_guest_caused"

// CalendarCancellation is how one meeting came to be cancelled. It holds the
// newest audit row that set its status, and the calendar event the cancelling
// transition names.
type CalendarCancellation struct {
	ID ids.UUID
	// ActorType and Seat are the newest status audit row's actor_type and
	// on_behalf_of. A connector acts on behalf of the seat whose calendar synced.
	ActorType string
	Seat      *ids.UUID
	// Restore is the status the newest status audit row replaced.
	Restore *string
	// Key is the calendar event the newest cancelling transition came from.
	Key connector.NaturalKey
}

// calendarCancellationScope selects live cancelled meetings scheduled on or
// after $1 whose newest cancelling transition came from a calendar event.
var calendarCancellationScope = `
  FROM activity a
  JOIN LATERAL (
       SELECT au.actor_type, au.on_behalf_of, au.after ->> 'meeting_status' AS became,
              NULLIF(au.before ->> 'meeting_status', '') AS restore
         FROM audit_log au
        WHERE au.entity_type = 'activity' AND au.entity_id = a.id AND au.after ? 'meeting_status'
        ORDER BY au.occurred_at DESC, au.id DESC LIMIT 1) last ON true
  JOIN LATERAL (
       SELECT h.source_system, h.source_id
         FROM activity_meeting_history h
        WHERE h.activity_id = a.id AND h.status = 'canceled' AND h.source_system IS NOT NULL
        ORDER BY h.effective_at DESC, h.id DESC LIMIT 1) cancel ON true
 WHERE a.kind = 'meeting' AND a.meeting_status = 'canceled'
   AND a.archived_at IS NULL AND a.restricted_at IS NULL
   AND ` + occurredday.FromSQL("a", "$1") + ` AND last.became = 'canceled'`

// CalendarCancellationsAfterTx reads the next limit cancelled meetings after
// the cursor in id order. A reopened meeting leaves the selection, so a second
// run finds it no more.
func CalendarCancellationsAfterTx(
	ctx context.Context, tx pgx.Tx, since time.Time, after ids.UUID, limit int,
) ([]CalendarCancellation, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT a.id, last.actor_type, last.on_behalf_of, last.restore, cancel.source_system, cancel.source_id `+
		calendarCancellationScope+` AND a.id > $2 ORDER BY a.id LIMIT $3`, since, after, limit)
	if err != nil {
		return nil, fmt.Errorf("activities: selecting cancelled calendar meetings: %w", err)
	}
	found, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (CalendarCancellation, error) {
		var c CalendarCancellation
		return c, row.Scan(&c.ID, &c.ActorType, &c.Seat, &c.Restore, &c.Key.SourceSystem, &c.Key.SourceID)
	})
	if err != nil {
		return nil, fmt.Errorf("activities: reading cancelled calendar meetings: %w", err)
	}
	return found, nil
}

// CountCalendarCancellationsTx counts the cancelled meetings still to judge, up
// to limit.
func CountCalendarCancellationsTx(ctx context.Context, q pgx.Tx, since time.Time, limit int64) (int64, error) {
	var count int64
	if err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT a.id `+calendarCancellationScope+` LIMIT $2) found`,
		since, limit).Scan(&count); err != nil {
		return 0, fmt.Errorf("activities: counting cancelled calendar meetings: %w", err)
	}
	return count, nil
}

// ReopenCalendarCancellationTx puts a cancelled meeting back to the status its
// cancellation replaced. It answers false and writes nothing when the meeting
// is no longer a live cancelled one.
func ReopenCalendarCancellationTx(ctx context.Context, tx pgx.Tx, c CalendarCancellation, runID ids.UUID) (bool, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return false, err
	}
	cancelled := string(crmcontracts.ActivityMeetingStatusCanceled)
	var start time.Time
	var precision string
	err := tx.QueryRow(ctx, `
		UPDATE activity SET meeting_status = $2
		 WHERE id = $1 AND kind = 'meeting' AND meeting_status = $3
		   AND restricted_at IS NULL AND archived_at IS NULL
		 RETURNING occurred_at, occurred_precision`,
		c.ID, c.Restore, cancelled).Scan(&start, &precision)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("activities: reopening the cancelled meeting %s: %w", c.ID, err)
	}
	auditID, err := storekit.AuditWithEvidence(ctx, tx, "update", "activity", c.ID,
		map[string]any{auditFieldMeetingStatus: cancelled}, map[string]any{auditFieldMeetingStatus: deref(c.Restore)},
		map[string]any{"data_task_run": runID.String(), "reopened": CalendarCancellationReopened})
	if err != nil {
		return false, err
	}
	var changed crmcontracts.PublicEventActivityChangedFields
	if c.Restore != nil {
		changed.MeetingStatus = new(crmcontracts.PublicEventActivityChangedFieldsMeetingStatus(*c.Restore))
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, c.ID, crmcontracts.PublicEventActivityUpdated{ChangedFields: changed}); err != nil {
		return false, err
	}
	return true, recordMeetingTransition(ctx, tx, meetingTransition{
		ActivityID:   ids.From[ids.ActivityKind](c.ID),
		Status:       string(crmcontracts.ActivityMeetingStatusBooked),
		Scheduled:    scheduledAs(start, precision),
		SourceSystem: &c.Key.SourceSystem,
		SourceID:     &c.Key.SourceID,
	})
}

// ReversedCancellationSQL is true for a transition no period count may read: a
// cancellation a later reopen reversed, and that reopen. Both name the same
// calendar event, which no other writer pairs.
func ReversedCancellationSQL(transition string) string {
	return fmt.Sprintf(`(%[1]s.source_system IS NOT NULL AND %[1]s.status IN ('canceled','booked')
 AND EXISTS (SELECT 1 FROM activity_meeting_history reversal
  WHERE reversal.activity_id = %[1]s.activity_id AND reversal.source_system = %[1]s.source_system
  AND reversal.source_id = %[1]s.source_id AND reversal.status IN ('canceled','booked') AND reversal.status <> %[1]s.status))`, transition)
}
