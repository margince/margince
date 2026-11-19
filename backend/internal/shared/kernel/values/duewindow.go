// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import "time"

// A task's due date lives within these years around now. Past the window a
// task sits on no list, so a year typo would lose it without a word. The
// frontend mirrors both numbers in format/calendarday.ts.
const (
	DueYearsBack  = 10
	DueYearsAhead = 10
)

// DueInWindow says whether a due moment is inside the window around now. Every
// writer of a task's due date asks it, so the rule has one owner.
func DueInWindow(due, now time.Time) bool {
	return !due.Before(now.AddDate(-DueYearsBack, 0, 0)) && !due.After(now.AddDate(DueYearsAhead, 0, 0))
}
