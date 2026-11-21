// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import "github.com/margince/margince/backend/internal/shared/kernel/occurredday"

// A task with a date and no time is due on the date in its chosen clock.
// These predicates are the one SQL spelling of "due before" and "due from" a
// moment, for every reader that counts tasks by due day. The Go counterpart is
// deadline.WallDayOffset, and a shared table holds the two to one rule.

// datedOnlySQL is true for a task whose due is a chosen date with no time.
func datedOnlySQL(alias string) string {
	return `(` + alias + `.due_wall IS NOT NULL AND ` + alias + `.due_has_time IS NOT TRUE)`
}

// installationDaySQL is the installation's calendar day that an instant falls on.
func installationDaySQL(instant string) string {
	return `((` + instant + `)::timestamptz AT TIME ZONE ` + occurredday.InstallationZoneSQL + `)::date`
}

// taskDueCompareSQL compares a task's due with an instant. A chosen date is
// compared with the instant's installation day, so it is late once its day is
// over. A timed task is compared by instant. An undated task satisfies nothing.
func taskDueCompareSQL(alias, instant, instantOp, dayOp string) string {
	due := TaskDueSQL(alias)
	return `(` + due + ` IS NOT NULL AND CASE WHEN ` + datedOnlySQL(alias) + ` THEN ` +
		alias + `.due_wall::date ` + dayOp + ` ` + installationDaySQL(instant) +
		` ELSE ` + due + ` ` + instantOp + ` (` + instant + `)::timestamptz END)`
}

// TaskDueBeforeSQL admits a task due strictly before instant, a SQL expression
// such as "$3" or "now()".
func TaskDueBeforeSQL(alias, instant string) string {
	return taskDueCompareSQL(alias, instant, "<", "<")
}

// TaskDueByNowSQL admits a task due at or before instant. A task with a chosen
// date is still not due until its day is over, as in TaskDueBeforeSQL.
func TaskDueByNowSQL(alias, instant string) string {
	return taskDueCompareSQL(alias, instant, "<=", "<")
}

// TaskDueFromSQL admits a task due at or after instant. It meets
// TaskDueBeforeSQL, so no task falls between two windows or sits in both. The
// due_at bound lets the index narrow the read; two days exceeds any zone offset.
func TaskDueFromSQL(alias, instant string) string {
	return `(` + alias + `.due_at >= (` + instant + `)::timestamptz - interval '2 days' AND ` +
		taskDueCompareSQL(alias, instant, ">=", ">=") + `)`
}

// TaskDueOrderSQL is the instant a due queue sorts a task by. A chosen date is
// due until the start of the next day in the installation's zone. It sorts at
// the last microsecond before that start. Any other task sorts by its due
// instant. The Go form is deadline.TaskDueOn, and a shared table holds them equal.
func TaskDueOrderSQL(alias string) string {
	return `CASE WHEN ` + datedOnlySQL(alias) + ` THEN (((` + alias + `.due_wall::date + 1)::timestamp AT TIME ZONE ` +
		occurredday.InstallationZoneSQL + `) - interval '1 microsecond') ELSE ` + TaskDueSQL(alias) + ` END`
}
