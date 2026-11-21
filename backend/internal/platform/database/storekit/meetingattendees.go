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

// LockMeetingAttendeeWrites is taken by every writer that changes the attendee
// rows of named activities. It waits for every judgement of them in flight.
// Take it before any row lock, so a judge holding the shared form never waits
// on a row this writer holds.
//
// A writer that sweeps by subject or domain names no activity and does not take
// it. TestEveryAttendeeWriterTakesTheMeetingLock lists each such sweep.
func LockMeetingAttendeeWrites(ctx context.Context, tx pgx.Tx, activityIDs ...ids.UUID) error {
	return lockMeetingAttendees(ctx, tx, `pg_advisory_xact_lock`, activityIDs)
}

// lockMeetingAttendees takes one lock per distinct activity, sorted, so two
// holders of overlapping sets queue in one order.
func lockMeetingAttendees(ctx context.Context, tx pgx.Tx, lock string, activityIDs []ids.UUID) error {
	keys := make([]string, 0, len(activityIDs))
	for _, id := range activityIDs {
		if !id.IsZero() {
			keys = append(keys, id.String())
		}
	}
	slices.Sort(keys)
	for _, key := range slices.Compact(keys) {
		if _, err := tx.Exec(ctx, `SELECT `+lock+`(hashtext('margince:meeting-attendees:' || $1)::bigint)`, key); err != nil {
			return fmt.Errorf("locking a meeting's attendees: %w", err)
		}
	}
	return nil
}
