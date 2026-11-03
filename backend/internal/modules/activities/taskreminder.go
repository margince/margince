// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// When a task's reminder is due, and for whom. It is read, never fired. A due
// reminder is a state of the row when it is read. So the Worklist, its count
// and the reminder_due list filter cannot disagree.

package activities

import (
	"context"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// reminderWindow bounds how old a passed reminder may be and still be due:
// the Worklist's age-out in days, as a fixed span. A calendar step would move
// the bound by an hour across a daylight-saving change in the reader's zone.
const reminderWindow = worklistAgeOutDays * 24 * time.Hour

// dueReminderClause and ReminderDueFor are two writers of one rule. The SQL
// bounds a page on it, and the Go judges a row already read.
// TestTheReminderClauseAndPredicateAgree runs one table of cases through both.
//
// The first four terms are idx_activity_reminders' own predicate. reader is a
// SQL expression naming the user the reminder is for, its assignee.
func dueReminderClause(alias, reader string, asOf time.Time, arg func(any) int) string {
	return sprintf(`(%[1]s.kind = 'task' AND %[1]s.remind_at IS NOT NULL AND %[1]s.is_done = false
		AND %[1]s.archived_at IS NULL AND %[1]s.remind_at <= $%[2]d AND %[1]s.remind_at >= $%[3]d
		AND %[1]s.assignee_id = %[4]s)`,
		alias, arg(asOf), arg(asOf.Add(-reminderWindow)), reader)
}

// ReminderDueFor reports whether row is a reminder due for reader at asOf.
func ReminderDueFor(row crmcontracts.Activity, reader ids.UUID, asOf time.Time) bool {
	if reader.IsZero() || row.Kind != crmcontracts.ActivityKindTask || row.RemindAt == nil {
		return false
	}
	if (row.IsDone != nil && *row.IsDone) || row.ArchivedAt != nil {
		return false
	}
	if row.AssigneeId == nil || ids.UUID(*row.AssigneeId) != reader {
		return false
	}
	at := *row.RemindAt
	return !at.After(asOf) && !at.Before(asOf.Add(-reminderWindow))
}

// ReminderReader is the user a read's reminders are for: the caller's own
// seat, the identity the `mine` Worklist scope answers for. Zero means nobody,
// and the clause then matches no row.
func ReminderReader(ctx context.Context) ids.UUID {
	actor, ok := principal.Actor(ctx)
	if !ok {
		return ids.UUID{}
	}
	return actor.UserID
}

// asReminderQueue makes a reminder_due read the Worklist's own: judged at the
// read's one instant, on the reader's own queue, under the Worklist's clauses.
// So the list filter and the Worklist answer with one set of tasks.
func asReminderQueue(ctx context.Context, in *ListActivitiesInput) {
	if !in.RemindersDue {
		return
	}
	if in.WorklistAsOf == nil {
		in.WorklistAsOf = in.AsOf
	}
	if in.OwnQueueOf == nil {
		reader := ids.From[ids.UserKind](ReminderReader(ctx))
		in.OwnQueueOf = &reader
	}
}

// reminderClockOf is the instant a list read judges reminders against. With
// neither instant set it is the zero time, at which no reminder is due.
func reminderClockOf(in ListActivitiesInput) time.Time {
	switch {
	case in.WorklistAsOf != nil:
		return *in.WorklistAsOf
	case in.AsOf != nil:
		return *in.AsOf
	}
	return time.Time{}
}

// endPassedReminder clears a reminder that has already passed when the patch
// moves or clears the task's due date. Changing the date is the reader dealing
// with the reminder, so it stops leading their Worklist. A patch that names
// remind_at itself, or a reminder still to come, is left alone.
func endPassedReminder(current crmcontracts.Activity, in *UpdateActivityInput) {
	if in.recordedAt == nil || in.RemindAt != nil || in.clears("remind_at") {
		return
	}
	if current.Kind != crmcontracts.ActivityKindTask || current.RemindAt == nil || current.RemindAt.After(*in.recordedAt) {
		return
	}
	if movesDueDate(current.DueAt, in) {
		in.Clear = append(in.Clear, "remind_at")
	}
}

// movesDueDate compares the due date the patch leaves with the one it found.
func movesDueDate(current *time.Time, in *UpdateActivityInput) bool {
	switch {
	case in.clears("due_at"):
		return current != nil
	case in.DueAt == nil:
		return false
	}
	return current == nil || !current.Equal(*in.DueAt)
}
