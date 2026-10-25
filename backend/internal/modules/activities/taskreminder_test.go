// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"slices"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type reminderHolder int

const (
	heldByReader reminderHolder = iota
	heldByColleague
	heldByNobody
)

// reminderCase is one task as the reminder rule judges it. remindedAgo is how
// long before the read instant the reminder fell, nil for a task with none.
type reminderCase struct {
	name        string
	remindedAgo *time.Duration
	done        bool
	archived    bool
	holder      reminderHolder
	want        bool
}

// reminderCases is the table both writers of the rule are held to: Go here,
// SQL in taskreminder_integration_test.go.
var reminderCases = []reminderCase{
	{name: "came due a minute ago", remindedAgo: new(time.Minute), want: true},
	{name: "comes due at the read instant", remindedAgo: new(time.Duration(0)), want: true},
	{name: "came due 29 days ago", remindedAgo: new(29 * 24 * time.Hour), want: true},
	{name: "still to come", remindedAgo: new(-time.Minute)},
	{name: "came due 31 days ago", remindedAgo: new(31 * 24 * time.Hour)},
	{name: "done", remindedAgo: new(time.Minute), done: true},
	{name: "archived", remindedAgo: new(time.Minute), archived: true},
	{name: "held by a colleague", remindedAgo: new(time.Minute), holder: heldByColleague},
	{name: "held by nobody", remindedAgo: new(time.Minute), holder: heldByNobody},
	{name: "no reminder", remindedAgo: nil},
}

// reminderInstant is a read instant at whole seconds, which Postgres stores
// without rounding.
func reminderInstant(t *testing.T) time.Time { return clocktest.Now(t).Truncate(time.Second) }

func (c reminderCase) row(reader, colleague ids.UUID, asOf time.Time) crmcontracts.Activity {
	row := crmcontracts.Activity{Kind: crmcontracts.ActivityKindTask, IsDone: new(c.done)}
	if c.remindedAgo != nil {
		row.RemindAt = new(asOf.Add(-*c.remindedAgo))
	}
	if c.archived {
		row.ArchivedAt = new(asOf)
	}
	switch c.holder {
	case heldByReader:
		row.AssigneeId = new(openapi_types.UUID(reader))
	case heldByColleague:
		row.AssigneeId = new(openapi_types.UUID(colleague))
	case heldByNobody:
	}
	return row
}

func TestTheReminderClauseAndPredicateAgree(t *testing.T) {
	asOf := reminderInstant(t)
	reader, colleague := ids.NewV7(), ids.NewV7()
	for _, c := range reminderCases {
		t.Run(c.name, func(t *testing.T) {
			if got := ReminderDueFor(c.row(reader, colleague, asOf), reader, asOf); got != c.want {
				t.Errorf("ReminderDueFor = %v, want %v", got, c.want)
			}
		})
	}
}

func TestNoReaderIsRemindedOfAnything(t *testing.T) {
	asOf := reminderInstant(t)
	row := reminderCases[0].row(ids.UUID{}, ids.NewV7(), asOf)
	if ReminderDueFor(row, ids.UUID{}, asOf) {
		t.Error("a read with no reader matched a task with no assignee")
	}
}

func TestANoteIsNeverAReminder(t *testing.T) {
	asOf := reminderInstant(t)
	reader := ids.NewV7()
	row := reminderCases[0].row(reader, ids.NewV7(), asOf)
	row.Kind = crmcontracts.ActivityKindNote
	if ReminderDueFor(row, reader, asOf) {
		t.Error("a note carrying a passed reminder counts as a due reminder")
	}
}

func TestMovingTheDueDateEndsAPassedReminder(t *testing.T) {
	at := reminderInstant(t)
	due := at.Add(time.Hour)
	moved := at.AddDate(0, 0, 3)
	task := crmcontracts.Activity{Kind: crmcontracts.ActivityKindTask, DueAt: &due, RemindAt: new(at.Add(-time.Minute))}

	cases := []struct {
		name      string
		current   crmcontracts.Activity
		in        UpdateActivityInput
		wantClear bool
	}{
		{"a passed reminder on a moved task", task, UpdateActivityInput{DueAt: &moved}, true},
		{"the same due date sent again", task, UpdateActivityInput{DueAt: &due}, false},
		{"a patch naming the reminder", task, UpdateActivityInput{DueAt: &moved, RemindAt: new(moved)}, false},
		{"a patch clearing the reminder", task, UpdateActivityInput{DueAt: &moved, Clear: []string{"remind_at"}}, false},
		{"a reminder still to come", withReminder(task, at.Add(time.Minute)), UpdateActivityInput{DueAt: &moved}, false},
		{"no move of the due date", task, UpdateActivityInput{Subject: new("Call back")}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := c.in
			in.recordedAt = &at
			cleared := slices.Contains(in.Clear, "remind_at")
			endPassedReminder(c.current, &in)
			if got := slices.Contains(in.Clear, "remind_at") && !cleared; got != c.wantClear {
				t.Errorf("the patch clears the reminder = %v, want %v", got, c.wantClear)
			}
		})
	}
}

func withReminder(task crmcontracts.Activity, at time.Time) crmcontracts.Activity {
	task.RemindAt = &at
	return task
}
