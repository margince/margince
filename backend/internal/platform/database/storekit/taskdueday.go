// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"regexp"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// A task with a date and no time is due on the date in its chosen clock.
// These predicates are the one SQL spelling of "due before" and "due from" a
// moment, for every reader that counts tasks by due day. The Go counterpart is
// deadline.WallDayOffset, and a shared table holds the two to one rule.

// datedOnlySQL is true for a task whose due is a chosen date with no time.
func datedOnlySQL(alias string) string {
	return `(` + alias + `.due_wall IS NOT NULL AND ` + alias + `.due_has_time IS NOT TRUE)`
}

// zoneName is what an IANA zone name looks like. It is checked again after the
// name has loaded as a location, so a name never carries a quote into SQL.
var zoneName = regexp.MustCompile(`^[A-Za-z0-9_+/-]{1,64}$`)

// DayZoneSQL is the zone a task read counts its days in. A person's own list
// names its IANA zone, written as a literal because it comes from a loaded
// location and not from a request. Empty, or a name that does not load, means
// the installation's zone.
func DayZoneSQL(zone string) string {
	if _, err := time.LoadLocation(zone); zone == "" || err != nil || !zoneName.MatchString(zone) {
		return occurredday.InstallationZoneSQL
	}
	return "'" + zone + "'::text"
}

// dayInZoneSQL is the calendar day that an instant falls on in a zone.
func dayInZoneSQL(instant, zoneSQL string) string {
	return `((` + instant + `)::timestamptz AT TIME ZONE ` + zoneSQL + `)::date`
}

// taskDueCompareSQL compares a task's due with an instant. A chosen date is
// compared with the instant's day in the zone, so it is late once its day is
// over. A timed task is compared by instant. An undated task satisfies nothing.
func taskDueCompareSQL(alias, instant, zoneSQL, instantOp, dayOp string) string {
	due := TaskDueSQL(alias)
	return `(` + due + ` IS NOT NULL AND CASE WHEN ` + datedOnlySQL(alias) + ` THEN ` +
		alias + `.due_wall::date ` + dayOp + ` ` + dayInZoneSQL(instant, zoneSQL) +
		` ELSE ` + due + ` ` + instantOp + ` (` + instant + `)::timestamptz END)`
}

// TaskDueBeforeSQL admits a task due strictly before instant, a SQL expression
// such as "$3" or "now()".
func TaskDueBeforeSQL(alias, instant string) string {
	return TaskDueBeforeInSQL(alias, instant, occurredday.InstallationZoneSQL)
}

// TaskDueBeforeInSQL is TaskDueBeforeSQL with the days counted in zoneSQL, a
// SQL expression for a zone name (DayZoneSQL).
func TaskDueBeforeInSQL(alias, instant, zoneSQL string) string {
	return taskDueCompareSQL(alias, instant, zoneSQL, "<", "<")
}

// TaskDueByNowSQL admits a task due at or before instant. A task with a chosen
// date is still not due until its day is over, as in TaskDueBeforeSQL.
func TaskDueByNowSQL(alias, instant string) string {
	return taskDueCompareSQL(alias, instant, occurredday.InstallationZoneSQL, "<=", "<")
}

// TaskDueFromSQL admits a task due at or after instant. It meets
// TaskDueBeforeSQL, so no task falls between two windows or sits in both. The
// due_at bound lets the index narrow the read; two days exceeds any zone offset.
func TaskDueFromSQL(alias, instant string) string {
	return TaskDueFromInSQL(alias, instant, occurredday.InstallationZoneSQL)
}

// TaskDueFromInSQL is TaskDueFromSQL with the days counted in zoneSQL.
func TaskDueFromInSQL(alias, instant, zoneSQL string) string {
	return `(` + alias + `.due_at >= (` + instant + `)::timestamptz - interval '2 days' AND ` +
		taskDueCompareSQL(alias, instant, zoneSQL, ">=", ">=") + `)`
}

// TaskDueOrderSQL is the instant a due queue sorts a task by. A chosen date is
// due until the start of the next day in the installation's zone. It sorts at
// the last microsecond before that start. Any other task sorts by its due
// instant. The Go form is deadline.TaskDueOn, and a shared table holds them equal.
func TaskDueOrderSQL(alias string) string {
	return TaskDueOrderInSQL(alias, occurredday.InstallationZoneSQL)
}

// TaskDueOrderInSQL is TaskDueOrderSQL with the chosen date ending in zoneSQL,
// the same zone the list's overdue test counts days in (DayZoneSQL).
func TaskDueOrderInSQL(alias, zoneSQL string) string {
	return `CASE WHEN ` + datedOnlySQL(alias) + ` THEN (((` + alias + `.due_wall::date + 1)::timestamp AT TIME ZONE ` +
		zoneSQL + `) - interval '1 microsecond') ELSE ` + TaskDueSQL(alias) + ` END`
}
