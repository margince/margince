// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package closerisk says when a deal's close date is at risk. The Worklist's
// deals-at-risk lane and a team's focus list both read it, so the two never
// disagree about one deal.
package closerisk

import (
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/elapsed"
)

// RecoveryHorizonDays is how near a close calls for recovery or
// requalification.
const RecoveryHorizonDays = 14

// Passed reports a close date before the day it is read on.
func Passed(closesInDays int) bool {
	return closesInDays < 0
}

// Overdue reports a close date, a calendar day, before the day now falls on in
// zone. A deal closing today is not late. The deal page, the viability decision
// and the input check all ask this one function.
func Overdue(closeDate, now time.Time, zone *time.Location) bool {
	y, m, d := closeDate.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Before(elapsed.CalendarDay(now, zone))
}

// Near reports a close from today to the recovery horizon, both included.
func Near(closesInDays int) bool {
	return closesInDays >= 0 && closesInDays <= RecoveryHorizonDays
}
