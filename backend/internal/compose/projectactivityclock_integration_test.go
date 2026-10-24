// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The project clock follows the rule the contact, company and deal clocks do.
// A meeting still ahead or called off does not move it. The refold job moves
// it once a booked meeting starts. Re-kinding an activity refolds it.
// The clock repair reads bounded pages, however few records are wrong.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// logMeetingOnAll logs one meeting with status on the contact, deal and project.
func (f taskClockFixture) logMeetingOnAll(t *testing.T, status string, at time.Time) {
	t.Helper()
	subject := "Project clock meeting"
	if _, _, err := f.e.Activities.LogActivity(f.e.Admin(), activities.LogActivityInput{
		Kind: "meeting", Subject: &subject, MeetingStatus: &status, OccurredAt: &at, Source: "manual", Links: f.links(),
	}); err != nil {
		t.Fatalf("logging the %s meeting: %v", status, err)
	}
}

func TestAMeetingAheadOrCalledOffLeavesTheProjectClockAlone(t *testing.T) {
	f := newTaskClockFixture(t)
	f.logMeetingOnAll(t, "booked", f.now.AddDate(0, 0, 7))
	f.assertAll(t, f.mailed, "a meeting next week has not happened")
	f.logMeetingOnAll(t, "canceled", f.now.Add(-time.Hour))
	f.logMeetingOnAll(t, "no_show", f.now.Add(-2*time.Hour))
	f.assertAll(t, f.mailed, "a meeting nobody held is no contact")
}

func TestTheRefoldJobMovesTheProjectClockOnceAMeetingStarts(t *testing.T) {
	f := newTaskClockFixture(t)
	meetsAt := f.now.AddDate(0, 0, 2)
	f.logMeetingOnAll(t, "booked", meetsAt)
	runRefold(t, shiftedPool(t, 3*24*time.Hour))
	f.assertClocks(t, meetsAt, "once the meeting has started, the refold counts it")
	if at := f.projectClock(t); at == nil || !at.Equal(meetsAt) {
		t.Errorf("project last_activity_at = %v, want the meeting's start %v", at, meetsAt)
	}
}

// rekind changes one activity's kind in place, dropping the task-only fields a
// note may not carry. No writer re-kinds a row, so this holds the trigger to
// the column the clock rule reads.
func (f taskClockFixture) rekind(t *testing.T, activity ids.UUID, kind string) {
	t.Helper()
	f.e.WsExec(t, `UPDATE activity SET kind = $1,
		assignee_id = CASE WHEN $1 = 'task' THEN assignee_id END,
		due_at = CASE WHEN $1 = 'task' THEN due_at END,
		remind_at = CASE WHEN $1 = 'task' THEN remind_at END
		WHERE id = $2`, kind, activity)
}

func TestReKindingAnActivityRefoldsTheProjectClock(t *testing.T) {
	f := newTaskClockFixture(t)
	at := f.now.Add(-time.Hour)
	subject := "Re-kinded row"
	logged, _, err := f.e.Activities.LogActivity(f.e.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: &subject, OccurredAt: &at, Source: "manual", Links: f.links(),
	})
	if err != nil {
		t.Fatal(err)
	}
	f.assertAll(t, f.mailed, "a task is no contact")
	f.rekind(t, ids.UUID(logged.Id), "note")
	f.assertAll(t, at, "the task became a note")
	f.rekind(t, ids.UUID(logged.Id), "task")
	f.assertAll(t, f.mailed, "the note became a task again")
}

// A repair over a table of correct records and one wrong one far behind them
// reads bounded pages to the end and repairs that one.
func TestTheClockRepairReadsBoundedPagesToAWrongRecordFarBehind(t *testing.T) {
	f := newTaskClockFixture(t)
	for range 24 {
		f.e.SeedContact(t, "Correct clock", nil)
	}
	wrong := f.e.SeedContact(t, "Wrong clock", nil)
	f.e.WsExec(t, `UPDATE contact SET last_activity_at = now() - interval '1 day' WHERE id = $1`, wrong)

	step := taskActivityClocksDataTask().Steps[0]
	ctx := datatask.Acting(principal.WithWorkspaceID(context.Background(), f.e.WS), "core/1792846717_task_activity_clocks")
	in, batches, changed := datatask.BatchIn{Limit: 5}, 0, 0
	for {
		var out datatask.BatchOut
		if err := f.e.DB().Tx(ctx, func(tx pgx.Tx) (err error) {
			out, err = step.Batch(ctx, tx, in)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if out.Scanned > in.Limit {
			t.Fatalf("a batch inspected %d records, want at most its limit %d", out.Scanned, in.Limit)
		}
		batches, changed = batches+1, changed+out.Changed
		if out.Done {
			break
		}
		in.Cursor = out.Cursor
	}
	if batches < 6 || changed != 1 {
		t.Errorf("the repair took %d batches and changed %d records, want at least 6 bounded batches and one change", batches, changed)
	}
	var stored *time.Time
	if err := f.e.Pool.QueryRow(context.Background(), `SELECT last_activity_at FROM contact WHERE id = $1`, wrong).Scan(&stored); err != nil || stored != nil {
		t.Errorf("the wrong clock reads %v (%v) after the repair, want none: the contact has no activity", stored, err)
	}

	wsCtx := principal.WithWorkspaceID(context.Background(), f.e.WS)
	var count int64
	if err := database.WithWorkspaceTx(wsCtx, f.e.Pool, func(tx pgx.Tx) (err error) {
		count, err = storekit.CountClockRecords(wsCtx, tx, storekit.ClockContacts, 3)
		return err
	}); err != nil || count != 4 {
		t.Errorf("the estimate counted %d records (%v), want it to stop at the cap plus one, 4", count, err)
	}
}
