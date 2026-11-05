// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reportperiod

import "time"

// WeekStart is local midnight on the Monday of the week holding at, in zone.
// Every reporting surface that names a week counts it from here. So the
// review, the forecast cut and an analytics period put a Sunday-night instant
// in the same week.
func WeekStart(at time.Time, zone *time.Location) time.Time {
	local := at.In(zone)
	back := (int(local.Weekday()) + 6) % 7
	return time.Date(local.Year(), local.Month(), local.Day()-back, 0, 0, 0, 0, zone)
}
