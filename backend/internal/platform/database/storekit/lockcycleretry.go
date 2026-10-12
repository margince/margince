// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/backoff"
)

// The ladder a transaction Postgres aborted to break a lock cycle is run on
// again. The other side of a cycle is one short write that has usually
// committed within milliseconds, so the pauses are short; the jitter keeps two
// victims of one cycle from meeting again on the same schedule.
const (
	lockCycleAttempts = 3
	lockCycleBase     = 20 * time.Millisecond
	lockCycleCeiling  = 200 * time.Millisecond
)

// RetryLockCycles runs a whole transaction again when Postgres aborts it as a
// deadlock victim or a serialization failure (IsLockCycle). Nothing an aborted
// transaction wrote survives, so this is safe exactly when run opens its own
// transaction and has no effect outside it — the condition a caller opts into
// by wrapping its Tx here. The last attempt's error is returned as it came.
func RetryLockCycles(ctx context.Context, run func() error) error {
	var err error
	for attempt := range lockCycleAttempts {
		if attempt > 0 {
			pause := time.NewTimer(backoff.Jittered(attempt-1, lockCycleBase, lockCycleCeiling))
			select {
			case <-ctx.Done():
				pause.Stop()
				return err
			case <-pause.C:
			}
		}
		if err = run(); !IsLockCycle(err) {
			return err
		}
	}
	return err
}
