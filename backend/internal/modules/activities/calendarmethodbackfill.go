// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The rows a calendar-method re-read is for, and the two writes it makes.
//
// Mail captured before activity.calendar_method existed reads NULL there. A
// contact's "Declined" then reads as owed, and our own invitation as waiting.
// Every such row is read, owed or not, because whether it is owed changes later.
//
// A row under a statutory hold is neither read nor marked. Its method stays
// NULL, which reads as ordinary mail: owed, never hidden.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The outcomes the calendar-method re-read records; the marker table's CHECK
// holds the same vocabulary.
const (
	CalendarMethodBackfillSet        = "set"
	CalendarMethodBackfillNone       = "none"
	CalendarMethodBackfillUnreadable = "unreadable"
)

// CalendarMethodMigration is the migration that added the column. Mail created
// after it ran was read by capture, so only older mail is the pass's to read.
const CalendarMethodMigration = "1791793531"

// calendarUncheckedSQL is a mail on alias a captured before calendar_method
// existed that may carry a calendar part and that the re-read has not checked
// yet. Its NULL says nothing, so it is not read as human mail until checked.
var calendarUncheckedSQL = `(a.kind = 'email' AND a.calendar_method IS NULL
 AND a.has_calendar_part IS NOT FALSE
 AND a.created_at < coalesce((SELECT m.applied_at FROM schema_migrations_core m
   WHERE m.version = '` + CalendarMethodMigration + `'), '-infinity'::timestamptz)
 AND NOT EXISTS (SELECT 1 FROM activity_calendar_method_backfill checked
   WHERE checked.activity_id = a.id))`

// CalendarMethodCandidate is one mail captured before calendar_method existed,
// not yet checked by the re-read.
type CalendarMethodCandidate struct {
	ActivityID ids.ActivityID
	// RawCaptureID is the stored original, nil when none survives.
	RawCaptureID *ids.UUID
}

// calendarMethodDirections are walked in this order, each to the end before
// the next. A copy we sent of our own invitation is as much a calendar copy as
// the one we received.
var calendarMethodDirections = []string{directionInbound, directionOutbound}

// CalendarMethodCandidatesTx offers the next limit unchecked mails in capture
// order, inbound first. Each direction resumes after its own newest checked
// mail, so outbound mail older than the inbound pointer is still offered.
// Every row it offers must be marked, or the walk would offer it again.
//
// Each walk is bounded by its activity_calendar_method_unread index and by
// the migration's own applied_at, so a drained pass reads two empty ranges.
func CalendarMethodCandidatesTx(ctx context.Context, tx pgx.Tx, limit int) ([]CalendarMethodCandidate, error) {
	var out []CalendarMethodCandidate
	for _, direction := range calendarMethodDirections {
		if len(out) >= limit {
			break
		}
		found, err := calendarMethodWalkTx(ctx, tx, direction, limit-len(out))
		if err != nil {
			return nil, err
		}
		out = append(out, found...)
	}
	return out, nil
}

func calendarMethodWalkTx(ctx context.Context, tx pgx.Tx, direction string, limit int) ([]CalendarMethodCandidate, error) {
	rows, err := tx.Query(ctx, `
		WITH resume AS (
		  SELECT b.activity_created_at AS at, b.activity_id AS id
		    FROM activity_calendar_method_backfill b
		   WHERE b.direction = $3
		   ORDER BY b.activity_created_at DESC, b.activity_id DESC
		   LIMIT 1
		), boundary AS (
		  SELECT m.applied_at AS at FROM schema_migrations_core m WHERE m.version = $2
		), walk AS (
		  SELECT a.id, a.created_at
		    FROM activity a
		   WHERE a.calendar_method IS NULL AND a.kind = 'email' AND a.direction = $3
		     AND a.restricted_at IS NULL
		     AND a.created_at < (SELECT at FROM boundary)
		     AND (a.created_at, a.id) > (
		         coalesce((SELECT at FROM resume), '-infinity'::timestamptz),
		         coalesce((SELECT id FROM resume), '00000000-0000-0000-0000-000000000000'::uuid))
		   ORDER BY a.created_at, a.id
		   LIMIT $1
		)
		SELECT w.id, rc.id
		  FROM walk w
		  JOIN activity src ON src.id = w.id
		  LEFT JOIN LATERAL (
		       SELECT r.id FROM raw_capture r
		        WHERE r.id = src.raw_capture_id
		           OR (src.raw_capture_id IS NULL
		               AND r.source_system = src.source_system AND r.source_id = src.source_id)
		        LIMIT 1) rc ON true
		 ORDER BY w.created_at, w.id`, limit, CalendarMethodMigration, direction)
	if err != nil {
		return nil, fmt.Errorf("activities: selecting %s mail whose calendar method can be re-read: %w", direction, err)
	}
	defer rows.Close()
	var out []CalendarMethodCandidate
	for rows.Next() {
		var c CalendarMethodCandidate
		if err := rows.Scan(&c.ActivityID, &c.RawCaptureID); err != nil {
			return nil, fmt.Errorf("activities: reading a calendar-method candidate: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("activities: reading the calendar-method candidates: %w", err)
	}
	return out, nil
}

// SetCalendarMethodTx stores the method read back from a message's original,
// with its audit row and event. It writes only onto a NULL, so a value capture
// recorded is never replaced, and reports whether it wrote. An archived row is
// written too: the method is a fact of the message, and an unarchived decline
// must still read as one.
func SetCalendarMethodTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID, method string) (bool, error) {
	changed := crmcontracts.PublicEventActivityChangedFieldsCalendarMethod(method)
	if !changed.Valid() {
		return false, fmt.Errorf("activities: %q is not an RFC 5546 calendar method", method)
	}
	tag, err := tx.Exec(ctx, `
		UPDATE activity SET calendar_method = $2
		 WHERE id = $1 AND calendar_method IS NULL AND restricted_at IS NULL`, id, method)
	if err != nil {
		return false, fmt.Errorf("activities: setting the calendar method of %s: %w", id, err)
	}
	if tag.RowsAffected() != 1 {
		return false, nil
	}
	auditID, err := storekit.Audit(ctx, tx, "update", "activity", id.UUID,
		map[string]any{auditFieldCalendarMethod: nil},
		map[string]any{auditFieldCalendarMethod: method})
	if err != nil {
		return false, err
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventActivityUpdated{
		ChangedFields: crmcontracts.PublicEventActivityChangedFields{CalendarMethod: &changed},
	}); err != nil {
		return false, err
	}
	return true, nil
}

const auditFieldCalendarMethod = "calendar_method"

// MarkCalendarMethodCheckedTx records that a message was checked, so it is
// never offered again and the next batch resumes after it.
func MarkCalendarMethodCheckedTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID, outcome string) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO activity_calendar_method_backfill (activity_id, activity_created_at, outcome, direction)
		SELECT a.id, a.created_at, $2, a.direction FROM activity a WHERE a.id = $1 AND a.restricted_at IS NULL
		ON CONFLICT (activity_id) DO NOTHING`, id, outcome); err != nil {
		return fmt.Errorf("activities: marking the calendar-method re-read of %s: %w", id, err)
	}
	return nil
}
