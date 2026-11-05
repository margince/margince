// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package weekly

// A meeting stays in the week it was held, over a real Postgres.
//
// A redate is an edit to the activity, not to what happened. So the week a
// meeting counts in is the one its held transition recorded, and the follow-up
// cutoff is that same moment.

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// heldMeetingWeekCount is the two meeting figures a week's assembly reads.
func heldMeetingWeekCount(t *testing.T, e *weekEnv, start time.Time) (int, int) {
	t.Helper()
	var held, withNextStep int
	if err := database.WithWorkspaceTx(e.repCtx, e.Pool, func(tx pgx.Tx) error {
		var err error
		held, withNextStep, err = countWeekMeetings(e.repCtx, tx, e.Rep1, afterEveryTransition,
			start, start.AddDate(0, 0, 7))
		return err
	}); err != nil {
		t.Fatalf("counting the week from %v: %v", start, err)
	}
	return held, withNextStep
}

func TestAMeetingRedatedAfterItWasHeldStaysInItsHeldWeek(t *testing.T) {
	e := setupWeekly(t)
	heldWeek := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	laterWeek := heldWeek.AddDate(0, 0, 7)

	held, host := "held", ids.From[ids.UserKind](e.Rep1)
	at := heldWeek.AddDate(0, 0, 2).Add(10 * time.Hour)
	logged, _, err := e.Activities.LogActivity(e.repCtx, activities.LogActivityInput{
		Kind: "meeting", Source: "manual", MeetingStatus: &held, HostUserID: &host,
		OccurredAt: &at, OccurredPrecision: string(activities.PrecisionInstant),
	})
	if err != nil {
		t.Fatalf("logging the held meeting: %v", err)
	}
	if got, _ := heldMeetingWeekCount(t, e, heldWeek); got != 1 {
		t.Fatalf("the week it was held counts %d, want 1 before any edit", got)
	}

	// A follow-up recorded while the meeting stood in its own week. The cutoff
	// for this figure is when the meeting began, so redating the meeting must
	// not strand its task before the start.
	assignee := ids.From[ids.UserKind](e.Rep1)
	source := ids.UUID(logged.Id)
	task, _, err := e.Activities.LogActivity(e.repCtx, activities.LogActivityInput{
		Kind: "task", Source: "manual", AssigneeID: &assignee, SourceActivityID: &source,
	})
	if err != nil {
		t.Fatalf("recording the next step: %v", err)
	}
	// Postgres stamps created_at as it writes, so the row lands today while the
	// week under test is in September. The figure counts a task recorded before
	// the week ended, so the stamp is pinned into the week after the real write.
	recordedAt := at.Add(time.Hour)
	if err := database.WithWorkspaceTx(e.repCtx, e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(e.repCtx, `UPDATE activity SET created_at = $1 WHERE id = $2`,
			recordedAt, ids.UUID(task.Id))
		return err
	}); err != nil {
		t.Fatalf("pinning when the next step was recorded: %v", err)
	}
	if counted, next := heldMeetingWeekCount(t, e, heldWeek); counted != 1 || next != 1 {
		t.Fatalf("before the edit the week reads %d held and %d with a next step, want 1 and 1", counted, next)
	}

	// Redated through the verb a member uses, which is what makes this the
	// defect's own path rather than a fixture's.
	moved := laterWeek.AddDate(0, 0, 2).Add(10 * time.Hour)
	if _, err := e.Activities.UpdateActivity(e.repCtx, ids.From[ids.ActivityKind](ids.UUID(logged.Id)),
		activities.UpdateActivityInput{OccurredAt: &moved}); err != nil {
		t.Fatalf("redating the meeting: %v", err)
	}

	counted, next := heldMeetingWeekCount(t, e, heldWeek)
	if counted != 1 {
		t.Errorf("the week it was held counts %d after the edit, want 1", counted)
	}
	// The figure the review shows beside it. Cutting the task on the meeting's
	// new time would report a held meeting with no follow-up.
	if next != 1 {
		t.Errorf("the week it was held reads %d with a next step after the edit, want 1", next)
	}
	if moved, _ := heldMeetingWeekCount(t, e, laterWeek); moved != 0 {
		t.Errorf("the week it was moved into counts %d, want 0", moved)
	}
}
