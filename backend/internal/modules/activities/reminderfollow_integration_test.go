// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"errors"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func taskID(a crmcontracts.Activity) ids.ActivityID {
	return ids.From[ids.ActivityKind](ids.UUID(a.Id))
}

func hasTime(a crmcontracts.Activity) bool { return a.DueHasTime != nil && *a.DueHasTime }

// A task created with a time of day reminds at that time, through every door
// that creates one: the task endpoint, a logged activity, and a follow-up. A
// reminder the writer names stays its own, and a task with a day alone gets
// none.
func TestATaskWithATimeRemindsAtThatTimeThroughEveryDoor(t *testing.T) {
	e := setupSend(t)
	ctx, store := e.as(principal.RowScopeAll), e.store(nil)
	callback := time.Date(clocktest.Now(t).Year()+1, 3, 2, 14, 0, 0, 0, time.UTC)

	viaTaskDoor, err := TaskInputFrom(crmcontracts.CreateTaskRequest{
		Subject: "Call back", Source: "manual", DueAt: &callback, DueHasTime: new(true),
	})
	if err != nil {
		t.Fatal(err)
	}
	created, _, err := store.LogActivity(ctx, viaTaskDoor)
	if err != nil {
		t.Fatal(err)
	}
	if !hasTime(created) || created.RemindAt == nil || !created.RemindAt.Equal(callback) {
		t.Errorf("task door: has time %v, reminder %v; want a reminder at %v", created.DueHasTime, created.RemindAt, callback)
	}

	named := callback.Add(-30 * time.Minute)
	ownReminder, _, err := store.LogActivity(ctx, LogActivityInput{Kind: "task", Subject: new("Prepare"), Source: "manual", DueAt: &callback, DueHasTime: true, RemindAt: &named})
	if err != nil {
		t.Fatal(err)
	}
	if ownReminder.RemindAt == nil || !ownReminder.RemindAt.Equal(named) {
		t.Errorf("a reminder the writer named moved to %v, want %v", ownReminder.RemindAt, named)
	}

	day, _, err := store.LogActivity(ctx, LogActivityInput{Kind: "task", Subject: new("Send the quote"), Source: "manual", DueAt: &callback})
	if err != nil {
		t.Fatal(err)
	}
	if hasTime(day) || day.RemindAt != nil {
		t.Errorf("a task due by a day reads has time %v with reminder %v, want neither", day.DueHasTime, day.RemindAt)
	}

	// The activity door: POST /activities, and the agent tools, whose
	// create_task folds into this same request.
	viaActivityDoor, err := LogActivityInputFrom(crmcontracts.CreateActivityRequest{
		Kind: "task", Subject: new("Call back"), Source: "manual", DueAt: &callback, DueHasTime: new(true),
	})
	if err != nil {
		t.Fatal(err)
	}
	fromActivity, _, err := store.LogActivity(ctx, viaActivityDoor)
	if err != nil {
		t.Fatal(err)
	}
	if !hasTime(fromActivity) || fromActivity.RemindAt == nil || !fromActivity.RemindAt.Equal(callback) {
		t.Errorf("activity door: has time %v, reminder %v; want a reminder at %v", fromActivity.DueHasTime, fromActivity.RemindAt, callback)
	}

	logged, err := LogActivityInputFrom(crmcontracts.CreateActivityRequest{
		Kind: "call", Subject: new("Spoke to the buyer"), Source: "manual",
		FollowUpTask: &crmcontracts.ActivityFollowUpTask{Subject: "Call back", DueAt: &callback, DueHasTime: new(true)},
	})
	if err != nil {
		t.Fatal(err)
	}
	row, _, err := store.LogActivity(ctx, logged)
	if err != nil {
		t.Fatal(err)
	}
	source := ids.From[ids.ActivityKind](ids.UUID(row.Id))
	tasks, _, err := store.ListActivities(ctx, ListActivitiesInput{SourceActivityID: &source})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || !hasTime(tasks[0]) || tasks[0].RemindAt == nil || !tasks[0].RemindAt.Equal(callback) {
		t.Fatalf("follow-up tasks %+v, want one with a time reminding at %v", tasks, callback)
	}
}

// The flag vouches for due_at's time of day, so it is refused without one and
// on anything but a task.
func TestATimeOfDayNeedsADueDateOnATask(t *testing.T) {
	e := setupSend(t)
	ctx, store := e.as(principal.RowScopeAll), e.store(nil)
	_, _, err := store.LogActivity(ctx, LogActivityInput{Kind: "task", Subject: new("Call back"), Source: "manual", DueHasTime: true})
	var required *RequiredFieldError
	if !errors.As(err, &required) || required.Field != "due_at" {
		t.Errorf("a time without a due date answered %v, want due_at required", err)
	}
	due := time.Date(clocktest.Now(t).Year()+1, 3, 2, 14, 0, 0, 0, time.UTC)
	task, _, err := store.LogActivity(ctx, LogActivityInput{Kind: "task", Subject: new("Call back"), Source: "manual", DueAt: &due})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.UpdateActivity(ctx, taskID(task), UpdateActivityInput{DueHasTime: new(true)})
	if !errors.As(err, &required) {
		t.Errorf("a patch naming a time without a due date answered %v, want due_at required", err)
	}
}

// Moving a task with a time moves its reminder; moving it to a day with no
// time ends a reminder that sat on the old time; a reminder the patch names
// is kept; and an older client that sends due_at alone moves the task to a day.
func TestAReminderFollowsTheTaskTime(t *testing.T) {
	e := setupSend(t)
	ctx, store := e.as(principal.RowScopeAll), e.store(nil)
	callback := time.Date(clocktest.Now(t).Year()+1, 10, 27, 14, 0, 0, 0, time.UTC)
	newTask := func() crmcontracts.Activity {
		t.Helper()
		task, _, err := store.LogActivity(ctx, LogActivityInput{Kind: "task", Subject: new("Call back"), Source: "manual", DueAt: &callback, DueHasTime: true})
		if err != nil {
			t.Fatal(err)
		}
		return task
	}
	move := func(task crmcontracts.Activity, in UpdateActivityInput) crmcontracts.Activity {
		t.Helper()
		out, err := store.UpdateActivity(ctx, taskID(task), in)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}

	nextDay := callback.AddDate(0, 0, 1)
	moved := move(newTask(), UpdateActivityInput{DueAt: &nextDay, DueHasTime: new(true)})
	if !hasTime(moved) || moved.RemindAt == nil || !moved.RemindAt.Equal(nextDay) {
		t.Errorf("a task moved to a new time reminds at %v (has time %v), want %v", moved.RemindAt, moved.DueHasTime, nextDay)
	}

	endOfDay := time.Date(callback.Year(), 10, 29, 22, 59, 59, 0, time.UTC)
	toDay := move(newTask(), UpdateActivityInput{DueAt: &endOfDay})
	if hasTime(toDay) || toDay.RemindAt != nil {
		t.Errorf("a task moved with due_at alone reads has time %v with reminder %v, want a day and no reminder", toDay.DueHasTime, toDay.RemindAt)
	}

	asked := callback.Add(-30 * time.Minute)
	kept := move(newTask(), UpdateActivityInput{DueAt: &nextDay, DueHasTime: new(true), RemindAt: &asked})
	if kept.RemindAt == nil || !kept.RemindAt.Equal(asked) {
		t.Errorf("a reminder the patch named reads %v, want %v", kept.RemindAt, asked)
	}

	var cleared int
	if err := e.owner.QueryRow(context.Background(), `SELECT count(*) FROM event_outbox
		WHERE envelope->>'type'='activity.updated' AND envelope->'payload'->'changed_fields'->>'remind_at_cleared'='true'
		AND envelope->'entity'->>'id'=$1`, toDay.Id.String()).Scan(&cleared); err != nil {
		t.Fatal(err)
	}
	if cleared != 1 {
		t.Errorf("the update that ended the reminder emitted %d remind_at_cleared events, want 1", cleared)
	}
}
