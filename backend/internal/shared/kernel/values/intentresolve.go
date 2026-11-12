// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import "time"

func resolveIntent(wall time.Time, loc *time.Location) (time.Time, error) {
	until := wall.Add(48 * time.Hour)
	cursor := wall.Add(-48 * time.Hour)
	var first time.Time
	found := false
	for !cursor.After(until) {
		local := cursor.In(loc)
		_, offset := local.Zone()
		candidate := wall.Add(-time.Duration(offset) * time.Second)
		if localWall(candidate.In(loc)).Equal(wall) && (!found || candidate.Before(first)) {
			first, found = candidate, true
		}
		_, next := local.ZoneBounds()
		if next.IsZero() || next.After(until) {
			break
		}
		before := localWall(next.Add(-time.Microsecond).In(loc)).Add(time.Microsecond)
		after := localWall(next.In(loc))
		if !wall.Before(before) && wall.Before(after) {
			return next.UTC(), nil
		}
		cursor = next
	}
	if !found {
		return time.Time{}, invalidIntent()
	}
	return first.UTC(), nil
}

func localWall(local time.Time) time.Time {
	return time.Date(local.Year(), local.Month(), local.Day(), local.Hour(), local.Minute(), local.Second(), local.Nanosecond(), time.UTC)
}
