// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package closerisk says when a deal's close date is at risk. The Worklist's
// deals-at-risk lane and a team's focus list both read it, so the two never
// disagree about one deal.
package closerisk

// RecoveryHorizonDays is how near a close calls for recovery or
// requalification.
const RecoveryHorizonDays = 14

// Passed reports a close date before the day it is read on.
func Passed(closesInDays int) bool {
	return closesInDays < 0
}

// Near reports a close from today to the recovery horizon, both included.
func Near(closesInDays int) bool {
	return closesInDays >= 0 && closesInDays <= RecoveryHorizonDays
}
