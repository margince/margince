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
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
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

// calendarMethodScope is the mail the re-read is for, in either direction. A
// copy we sent of our own invitation is a calendar copy too. Paging by id
// needs no index of its own.
var calendarMethodScope = `FROM activity a
 WHERE a.kind = 'email' AND a.calendar_method IS NULL AND a.direction IN ('inbound', 'outbound')
   AND a.restricted_at IS NULL
   AND a.created_at < (SELECT m.applied_at FROM schema_migrations_core m WHERE m.version = '` + CalendarMethodMigration + `')
   AND NOT EXISTS (SELECT 1 FROM activity_calendar_method_backfill checked WHERE checked.activity_id = a.id)`

// CalendarMethodCandidatesTx offers the next limit unchecked mails after the
// given id, in id order, locked for the caller's batch.
func CalendarMethodCandidatesTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) ([]CalendarMethodCandidate, error) {
	rows, err := tx.Query(ctx, `
		SELECT a.id, (SELECT r.id FROM raw_capture r WHERE `+storekit.StoredOriginalSQL("a", "r")+` LIMIT 1)
		  `+calendarMethodScope+`
		   AND a.id > @after
		 ORDER BY a.id
		 LIMIT @batch_limit
		   FOR UPDATE OF a`, pgx.NamedArgs{"after": after, "batch_limit": limit})
	if err != nil {
		return nil, fmt.Errorf("activities: selecting mail whose calendar method can be re-read: %w", err)
	}
	out, err := pgx.CollectRows(rows, pgx.RowToStructByPos[CalendarMethodCandidate])
	if err != nil {
		return nil, fmt.Errorf("activities: reading the calendar-method candidates: %w", err)
	}
	return out, nil
}

// MarkCalendarMethodsUnreadableTx records the unchecked mail among activities
// as unreadable. Its original is about to be destroyed, so no later read can
// check it. A mail the task passed while it was held still ends checked.
func MarkCalendarMethodsUnreadableTx(ctx context.Context, tx pgx.Tx, activities []ids.UUID) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO activity_calendar_method_backfill (activity_id, activity_created_at, outcome, direction)
		SELECT a.id, a.created_at, @outcome, a.direction `+calendarMethodScope+` AND a.id = ANY(@activities)
		ON CONFLICT (activity_id) DO NOTHING`,
		pgx.NamedArgs{"outcome": CalendarMethodBackfillUnreadable, "activities": activities}); err != nil {
		return fmt.Errorf("activities: marking the calendar-method re-read of erased mail: %w", err)
	}
	return nil
}

// EstimateCalendarMethodCandidates counts the unchecked mails for the data
// task's estimate, reading at most maximum plus one.
func EstimateCalendarMethodCandidates(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	var count int64
	if err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT a.id `+calendarMethodScope+` LIMIT @cap) candidates`,
		pgx.NamedArgs{"cap": maximum + 1}).Scan(&count); err != nil {
		return 0, false, fmt.Errorf("activities: counting mail whose calendar method can be re-read: %w", err)
	}
	return min(count, maximum), count > maximum, nil
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
// never offered again and calendarUncheckedSQL reads it as checked.
func MarkCalendarMethodCheckedTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID, outcome string) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO activity_calendar_method_backfill (activity_id, activity_created_at, outcome, direction)
		SELECT a.id, a.created_at, $2, a.direction FROM activity a WHERE a.id = $1 AND a.restricted_at IS NULL
		ON CONFLICT (activity_id) DO NOTHING`, id, outcome); err != nil {
		return fmt.Errorf("activities: marking the calendar-method re-read of %s: %w", id, err)
	}
	return nil
}
