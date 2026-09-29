// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package reportperiod defines calendar-aligned comparison windows.
package reportperiod

import "time"

// Context records the temporal and definition boundary for period comparison.
type Context struct {
	Start, End                  time.Time
	Timezone, Currency, Version string
	Complete                    bool
}

// Comparable requires adjacent complete periods with matching currency and definitions.
func Comparable(left, right Context) bool {
	if !left.Complete || !right.Complete || left.Version == "" || left.Version != right.Version || left.Currency != right.Currency || left.Timezone != right.Timezone {
		return false
	}
	zone, err := time.LoadLocation(left.Timezone)
	if err != nil {
		return false
	}
	kind := CompletedKind(left.Start, left.End, zone)
	return kind != "" && kind == CompletedKind(right.Start, right.End, zone) && left.End.Equal(right.Start)
}

// CompletedKind recognizes complete civil-calendar periods, including DST transitions.
func CompletedKind(start, end time.Time, zone *time.Location) string {
	start, end = start.In(zone), end.In(zone)
	if start.Hour() != 0 || start.Minute() != 0 || start.Second() != 0 || start.Nanosecond() != 0 {
		return ""
	}
	if start.Weekday() == time.Monday && start.AddDate(0, 0, 7).Equal(end) {
		return "week"
	}
	if start.Day() != 1 {
		return ""
	}
	if start.AddDate(0, 1, 0).Equal(end) {
		return "month"
	}
	if start.AddDate(0, 3, 0).Equal(end) {
		return "quarter"
	}
	return ""
}
