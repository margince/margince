// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package occurredday

// The one rule for "which day is it for this reader", in SQL. Its Go twin is
// compose/dayrule, and a shared table in that package holds the two equal.

// Surface says whose clock a screen reads the day on.
type Surface int

const (
	// Combined is a screen that puts several people's work side by side, such as
	// a team worklist, the pipeline, a report or a forecast. It reads the
	// installation's zone, so every viewer sees the same day boundary.
	Combined Surface = iota
	// Personal is a list that belongs to one person, such as their Home, their
	// own worklist, their brief or their own overdue tasks. It reads that
	// person's own zone and falls back to the installation's.
	Personal
)

// ZoneSQL is the zone name a surface reads the day in. user is a SQL expression
// for the person's app_user id, and Combined ignores it.
func ZoneSQL(surface Surface, user string) string {
	if surface != Personal {
		return InstallationZoneSQL
	}
	return `COALESCE(NULLIF((SELECT pz.timezone FROM app_user pz WHERE pz.id = ` + user + `), ''), ` + InstallationZoneSQL + `)`
}

// DaySQL is the calendar day at asOf on a surface, a date.
func DaySQL(surface Surface, asOf, user string) string {
	return `(CAST(` + asOf + ` AS timestamptz) AT TIME ZONE ` + ZoneSQL(surface, user) + `)::date`
}
