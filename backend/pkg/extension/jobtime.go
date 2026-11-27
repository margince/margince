// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

import (
	"fmt"
	"slices"
	"time"
)

// minutesPerDay bounds TimeOfDay.Minute.
const minutesPerDay = 24 * 60

// TimeOfDay is a job that runs at one local clock time, such as "weekdays at
// 07:30", in place of a fixed interval. The core counts the clock time in the
// installation's time zone.
type TimeOfDay struct {
	// Minute is the clock time as minutes after local midnight, 0 to 1439.
	Minute int
	// Days are the weekdays it runs on. Empty means every day.
	Days []time.Weekday
}

// Validate refuses a clock time past the day and a weekday that is not one or
// is named twice.
func (t TimeOfDay) Validate() error {
	if t.Minute < 0 || t.Minute >= minutesPerDay {
		return fmt.Errorf("time of day %d is not a minute of the day (0 to %d)", t.Minute, minutesPerDay-1)
	}
	for i, d := range t.Days {
		if d < time.Sunday || d > time.Saturday {
			return fmt.Errorf("day %d is not a weekday", int(d))
		}
		if slices.Contains(t.Days[:i], d) {
			return fmt.Errorf("day %s is named twice", d)
		}
	}
	return nil
}

// RunsOn reports whether the job runs on the given weekday.
func (t TimeOfDay) RunsOn(d time.Weekday) bool {
	return len(t.Days) == 0 || slices.Contains(t.Days, d)
}

// LongestGap is the longest wait between two runs. That is a day for a daily
// job, three days for one that skips the weekend, a week for a weekly one. Job
// health reads it as the time after which a missing run is overdue.
func (t TimeOfDay) LongestGap() time.Duration {
	longest, gap := 0, 0
	// Two weeks, so the gap that wraps from Saturday into Sunday is counted.
	for day := range 14 {
		gap++
		if t.RunsOn(time.Weekday(day % 7)) {
			longest, gap = max(longest, gap), 0
		}
	}
	return time.Duration(longest) * 24 * time.Hour
}
