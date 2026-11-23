// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package occurredday

import "time"

// The one rule for "which day is it for this reader", in SQL. Its Go twin is
// compose/dayrule, and a shared table in that package holds the two equal.

// EditionGap is the shortest time between two editions of a member's brief or
// digest. A zone change gives the same real day a different date. No new edition
// is filed while the last one is younger than this.
const EditionGap = 20 * time.Hour

// Surface says whose clock a screen reads the day on.
type Surface int

const (
	// Combined is a screen that puts several colleagues' work side by side, such as
	// a team worklist, the pipeline, a report or a forecast. It reads the
	// installation's zone, so every viewer sees the same day boundary.
	Combined Surface = iota
	// Personal is a list that belongs to one member, such as their Home, their
	// own worklist, their brief or their own overdue tasks. It reads that
	// member's own zone and falls back to the installation's.
	Personal
)

// ZoneSQL is the zone name a surface reads the day in. user is a SQL expression
// for the member's app_user id, and Combined ignores it. A stored zone the
// database does not know falls back to the installation's, as dayrule.Zone does.
func ZoneSQL(surface Surface, user string) string {
	if surface != Personal {
		return InstallationZoneSQL
	}
	return `COALESCE((SELECT pz.timezone FROM app_user pz WHERE pz.id = ` + user + `
		AND EXISTS (SELECT 1 FROM pg_timezone_names zn WHERE zn.name = pz.timezone)), ` + InstallationZoneSQL + `)`
}

// DaySQL is the calendar day at asOf on a surface, a date.
func DaySQL(surface Surface, asOf, user string) string {
	return `(CAST(` + asOf + ` AS timestamptz) AT TIME ZONE ` + ZoneSQL(surface, user) + `)::date`
}
