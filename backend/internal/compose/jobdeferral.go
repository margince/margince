// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"time"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/ai"
)

// snoozeWhileDeferred turns the router's "not now" — the budget stop or a
// blocked provider — into a snooze until the moment the router named. River
// does not count a snooze as an attempt, so the wait costs the job no try and
// the work is done once the budget or the provider is back.
func snoozeWhileDeferred(err error, now time.Time) error {
	until, deferred := ai.DeferredUntil(err)
	if !deferred {
		return err
	}
	return river.JobSnooze(max(until.Sub(now), 0))
}

// snoozeWhileHeld is snoozeWhileDeferred and snoozeWhileRestricted together:
// what a reading worker hands its fault to before the fault is classified, so
// a wait is never logged as a failure.
func snoozeWhileHeld(err error, now time.Time) error {
	return snoozeWhileRestricted(snoozeWhileDeferred(err, now))
}
