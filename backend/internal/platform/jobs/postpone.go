// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package jobs

import (
	"context"
	"errors"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// Postponement is a refusal that names when the same work may be tried
// again. A dependency said "not now", such as the AI budget stop or a blocked
// provider. It is no failure, so the job waits rather than spends a
// try. The error types satisfy it by method, so this package imports none of
// their owners.
type Postponement interface {
	error
	// RetryAt is when to ask again. It answers false for a call that was
	// made and failed, which is a failure however it is typed.
	RetryAt() (time.Time, bool)
}

// postponementClock is when a postponement's wait is measured from. A test in
// this package replaces it.
var postponementClock = time.Now

// postponed reschedules a postponement through rescheduleFor, which floors the
// wait at a second and caps it at maxRescheduleDelay. A longer wait wakes,
// asks again and sleeps again, so it stays visible to the health readings. It
// answers nil for any other error.
func postponed(ctx context.Context, kind string, err error) error {
	until, ok := waitUntil(err)
	if !ok {
		return nil
	}
	return rescheduleFor(ctx, kind, "postponed", until.Sub(postponementClock()), err)
}

// Exhausted reports whether err ends the job for good. River will not run it
// again, and the error is no snooze or postponement that brings it back. A
// worker that handed its item back for a retry closes it instead.
func Exhausted(row *rivertype.JobRow, err error) bool {
	if err == nil || row == nil || row.Attempt < row.MaxAttempts {
		return false
	}
	if _, snoozed := errors.AsType[*river.JobSnoozeError](err); snoozed {
		return false
	}
	_, waits := waitUntil(err)
	return !waits
}

// waitUntil is when a postponement asks again, and false for any other error.
func waitUntil(err error) (time.Time, bool) {
	postponement, ok := errors.AsType[Postponement](err)
	if !ok {
		return time.Time{}, false
	}
	return postponement.RetryAt()
}
