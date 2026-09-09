// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Closing a captured meeting the calendar says is off.
//
// A meeting reaches the timeline as `booked` and every surface that draws a day
// reads it that way — the schedule rail, the brief, the awaiting-outcome lane.
// When the organizer calls it off, or the seat whose calendar it is declines it,
// the provider simply stops listing the event: no later pull mentions it, so the
// row stands as booked forever and the rep keeps reading a meeting that is not
// happening.
//
// This is the write that closes it. It belongs here rather than in capture
// because this module owns `activity` and its status history; compose injects it
// into the capture Sink, which never imports a sibling.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// auditFieldMeetingStatus is the audit image's key for the column this writes.
const auditFieldMeetingStatus = "meeting_status"

// CancelCapturedMeetingTx marks the meeting captured under one connector's
// natural key as cancelled, and records the transition.
//
// It answers which activity it closed, and whether it closed one at all. Not
// finding a meeting is the ORDINARY case rather than a fault: most cancelled
// events were never worth capturing — an internal meeting, a solo block — so
// there is nothing under the key, and a calendar pull must not fail over it.
//
// Idempotent. A meeting already cancelled is left alone and reported as
// unchanged, so a resynced calendar writes no second audit row, no second event
// and no second transition. The status is compared under the row's own lock
// rather than before it, because a decision taken before the lock is a decision
// about a state that can move.
//
// `at` is the meeting's own scheduled start, and it is what the transition is
// stamped with. A pull that runs days after the fact would otherwise record the
// cancellation as having happened when we noticed, and every question about when
// bookings fell through would answer with the sync schedule.
func CancelCapturedMeetingTx(
	ctx context.Context, tx pgx.Tx, key connector.NaturalKey, at time.Time,
) (ids.ActivityID, bool, error) {
	if key.SourceSystem == "" || key.SourceID == "" {
		return ids.ActivityID{}, false, errors.New("activities: cancelling a captured meeting needs a natural key")
	}
	// The row FIRST, and locked, in the order every other writer of this table
	// takes it. The status is read under the same lock that will write it, so
	// two syncs landing at once cannot both decide the meeting still needs
	// closing and both record the transition.
	//
	// The kind is part of the WHERE rather than checked afterwards: a natural
	// key is unique across the table, so a non-meeting row under it is a
	// connector writing something other than a meeting, and cancelling it would
	// put a meeting status on a row whose kind forbids one (the activity CHECK
	// would refuse it, and the refusal would fail the whole pull).
	//
	// An ARCHIVED meeting is left alone, and that is a decision rather than an
	// oversight. Somebody retired that row deliberately; a calendar sync is not
	// the caller entitled to move its status back under them, and a cancellation
	// changes nothing a reader of an archived row can see.
	var id ids.ActivityID
	var status *string
	err := tx.QueryRow(ctx, `
		SELECT id, meeting_status FROM activity
		 WHERE source_system = $1 AND source_id = $2
		   AND kind = 'meeting' AND restricted_at IS NULL AND archived_at IS NULL
		 FOR UPDATE`,
		key.SourceSystem, key.SourceID).Scan(&id, &status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Never captured, not a meeting, archived, or under a statutory
			// hold. Nothing to close in any of the four.
			return ids.ActivityID{}, false, nil
		}
		return ids.ActivityID{}, false, fmt.Errorf("activities: reading the meeting being cancelled: %w", err)
	}
	cancelled := string(crmcontracts.ActivityMeetingStatusCanceled)
	if status != nil && *status == cancelled {
		return id, false, nil
	}
	if err := writeMeetingCancellationTx(ctx, tx, id, status, cancelled, at, key); err != nil {
		return ids.ActivityID{}, false, err
	}
	return id, true, nil
}

// writeMeetingCancellationTx performs the write shape: the column, its audit
// row, the event, and the transition the history is read from.
func writeMeetingCancellationTx(
	ctx context.Context, tx pgx.Tx, id ids.ActivityID,
	stored *string, cancelled string, at time.Time, key connector.NaturalKey,
) error {
	// The status pin re-states what the caller read under FOR UPDATE, so a
	// change that slipped in between writes nothing rather than overwriting it.
	// The archived and restricted predicates are re-stated here rather than
	// trusted to the caller's read: this is the statement that writes, and a
	// row that was retired between the lock and the write must not be moved by
	// it.
	tag, err := tx.Exec(ctx, `
		UPDATE activity SET meeting_status = $2
		 WHERE id = $1 AND meeting_status IS NOT DISTINCT FROM $3
		   AND restricted_at IS NULL AND archived_at IS NULL`,
		id, cancelled, stored)
	if err != nil {
		return fmt.Errorf("activities: cancelling the captured meeting %s: %w", id, err)
	}
	if tag.RowsAffected() != 1 {
		// The row moved under the lock. An audit row about a change that did not
		// happen would put a lie in the compliance trail.
		return nil
	}
	before := map[string]any{auditFieldMeetingStatus: deref(stored)}
	after := map[string]any{auditFieldMeetingStatus: cancelled}
	auditID, err := storekit.Audit(ctx, tx, "update", "activity", id.UUID, before, after)
	if err != nil {
		return err
	}
	status := crmcontracts.PublicEventActivityChangedFieldsMeetingStatus(cancelled)
	if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventActivityUpdated{
		ChangedFields: crmcontracts.PublicEventActivityChangedFields{MeetingStatus: &status},
	}); err != nil {
		return err
	}
	// The history, through the ONE recorder every status writer reaches. The
	// scheduled start travels as the transition's own moment; the natural key
	// travels as its idempotency key, so a calendar resynced twice records one
	// cancellation.
	scheduledStart := &at
	if at.IsZero() {
		// An event whose start we could not read. The transition is still
		// recorded — the meeting was cancelled either way — with no scheduled
		// start rather than with the zero instant, which would file it at the
		// beginning of history and count as a booking due in year one.
		scheduledStart = nil
	}
	return recordMeetingTransition(ctx, tx, meetingTransition{
		ActivityID:     id,
		Status:         cancelled,
		ScheduledStart: scheduledStart,
		SourceSystem:   &key.SourceSystem,
		SourceID:       &key.SourceID,
	})
}
