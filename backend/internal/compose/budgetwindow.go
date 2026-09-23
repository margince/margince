// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"errors"
	"time"

	"github.com/margince/margince/backend/internal/modules/ai"
)

// budgetWindowAt is when a background task deferred for budget may run again.
//
// The router names the boundary on the error and it is the start of the next
// MONTHLY window, so a caller that substitutes its own failure spacing wakes
// the same rows every few minutes until the month turns, reaching no model on
// any of those passes. A bare sentinel names no boundary; only then does the
// caller's spacing stand in.
func budgetWindowAt(err error, now time.Time, fallback time.Duration) time.Time {
	var deferral *ai.BudgetDeferralError
	if errors.As(err, &deferral) && deferral.NextAttemptAt.After(now) {
		return deferral.NextAttemptAt
	}
	return now.Add(fallback)
}

// untilBudgetWindow is budgetWindowAt as the DURATION a store's Defer takes.
//
// Due-ness is stamped with the database's clock, and Defer re-bases this on it:
// only the skew between the two clocks reaches the schedule, never the app
// process's idea of the absolute time.
func untilBudgetWindow(err error, fallback time.Duration) time.Duration {
	now := time.Now()
	return budgetWindowAt(err, now, fallback).Sub(now)
}
