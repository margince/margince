// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// LockMeetingAttendeeChecks takes the shared lock a judgement of these
// activities' attendees holds. A row lock does not hold off a newly inserted
// attendee; this lock does, until the judging transaction ends.
//
// Take it before any row lock on the activities it names.
func LockMeetingAttendeeChecks(ctx context.Context, tx pgx.Tx, activityIDs ...ids.UUID) error {
	return lockMeetingAttendees(ctx, tx, `pg_advisory_xact_lock_shared`, activityIDs)
}

// LockMeetingAttendeeWrites is taken by every writer of named activities'
// attendee rows. Take it before any row lock, so a judge holding the shared form
// never waits on a row this writer holds. TestEveryAttendeeWriterTakesTheMeetingLock
// lists the sweeps that name no activity and skip it.
func LockMeetingAttendeeWrites(ctx context.Context, tx pgx.Tx, activityIDs ...ids.UUID) error {
	return lockMeetingAttendees(ctx, tx, `pg_advisory_xact_lock`, activityIDs)
}

// LockMeetingAttendeePage takes a batch page's attendee locks and then its
// activity row locks, in that order. The page is read unlocked first, so each
// per-row writer checks its rule again under the row lock.
func LockMeetingAttendeePage(ctx context.Context, tx pgx.Tx, page []ids.UUID) error {
	if err := LockMeetingAttendeeWrites(ctx, tx, page...); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `SELECT id FROM activity WHERE id = ANY($1) ORDER BY id FOR UPDATE`, page); err != nil {
		return fmt.Errorf("locking a page of activities: %w", err)
	}
	return nil
}

// lockMeetingAttendees takes one lock per distinct key, in key order, so two
// holders of overlapping sets queue in one order. Two activities whose keys
// collide share one lock, which costs a wait and never reverses the order.
func lockMeetingAttendees(ctx context.Context, tx pgx.Tx, lock string, activityIDs []ids.UUID) error {
	present := slices.DeleteFunc(slices.Clone(activityIDs), ids.UUID.IsZero)
	if len(present) == 0 {
		return nil
	}
	rows, err := tx.Query(ctx, `SELECT DISTINCT hashtextextended('margince:meeting-attendees:' || id::text, 0) AS k
		FROM unnest($1::uuid[]) id ORDER BY k`, present)
	if err != nil {
		return fmt.Errorf("keying a meeting's attendee locks: %w", err)
	}
	keys, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return fmt.Errorf("keying a meeting's attendee locks: %w", err)
	}
	for _, key := range keys {
		if _, err := tx.Exec(ctx, `SELECT `+lock+`($1)`, key); err != nil {
			return fmt.Errorf("locking a meeting's attendees: %w", err)
		}
	}
	return nil
}
