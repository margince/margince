// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Recording what a meeting BECAME, and when.
//
// activity.meeting_status answers "what is this meeting now". Every question
// about a period — how many did we book last week, what share of last month's
// meetings were held — is about the transition instead, and the column cannot
// answer those: a meeting booked on Monday and held on Friday reads as `held`,
// so a count over the column reports no bookings for the week it was booked in.
//
// ONE writer, called by every door that can set a status. Two writers would be
// two answers to one question, and the one that drifted would be whichever a
// reader did not read.
//
// Held by TestEveryMeetingStatusWriterRecordsHistory (backend/gates), which
// reads the tree for statements that WRITE the column and fails on one that
// does not reach this function.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// meetingTransition is one status change, as the writer takes it.
type meetingTransition struct {
	ActivityID ids.ActivityID
	// Status is what the meeting became. Empty means this write set no status,
	// and recordMeetingTransition returns without writing — a note being edited
	// is not a meeting transition.
	Status string
	// ScheduledStart is the start this meeting was booked for, as of NOW. Kept
	// per transition rather than read from the activity, because a reschedule
	// moves it and "which period was this due in" must be answerable at any
	// past instant.
	ScheduledStart *time.Time
	// SourceSystem and SourceID name the connector event this came from. Both
	// nil for a human action. Together they are the idempotency key: a resynced
	// calendar writes no second transition.
	SourceSystem *string
	SourceID     *string
}

// meetingStatusOrNone reads an optional status as the writer's "" sentinel: a
// write that names no status is not a transition.
func meetingStatusOrNone(status *string) string {
	if status == nil {
		return ""
	}
	return *status
}

// changedMeetingStatus is the status an update MOVED a meeting to, or "" when
// it moved nothing.
//
// Read from the rows before and after rather than from the request, because a
// PATCH carrying `held` on a meeting already held is not a transition — and the
// request cannot tell the difference. Counting it would make one meeting held
// twice.
func changedMeetingStatus(before, after crmcontracts.Activity) string {
	next := meetingStatusString(after.MeetingStatus)
	if next == "" || next == meetingStatusString(before.MeetingStatus) {
		return ""
	}
	return next
}

// meetingStatusString flattens the contract's optional enum to the writer's
// vocabulary; the two spell the same four values.
func meetingStatusString(status *crmcontracts.ActivityMeetingStatus) string {
	if status == nil {
		return ""
	}
	return string(*status)
}

// recordMeetingTransition writes one transition, or nothing.
//
// The actor comes from the authenticated principal and never from a caller's
// payload — the same rule storekit.Audit holds for captured_by, for the same
// reason: a history whose actor can be asserted answers "who did this" with
// whatever the request said.
//
// A duplicate connector event is not an error. The unique index refuses the
// second write and this reports success, because the caller's intent — that
// this transition be on record — is already satisfied. Returning a conflict
// would make an idempotent resync look like a failure and stop the sync.
func recordMeetingTransition(ctx context.Context, tx pgx.Tx, in meetingTransition) error {
	if in.Status == "" {
		return nil
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		return fmt.Errorf("activities: recording a meeting transition without an actor")
	}
	// The pair travels together or not at all — the column CHECK says so, and
	// half a key would index a transition nothing can match on replay.
	sourceSystem, sourceID := in.SourceSystem, in.SourceID
	if sourceSystem == nil || sourceID == nil {
		sourceSystem, sourceID = nil, nil
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO activity_meeting_history
		    (activity_id, status, effective_at, scheduled_start, actor, source_system, source_id)
		VALUES ($1, $2, now(), $3, $4, $5, $6)`,
		in.ActivityID.UUID, in.Status, in.ScheduledStart, actor.ID, sourceSystem, sourceID)
	if err != nil {
		if storekit.IsUniqueViolation(err) {
			// This exact connector event is already on record.
			return nil
		}
		return fmt.Errorf("activities: recording a meeting transition: %w", err)
	}
	return nil
}
