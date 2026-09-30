// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package database

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

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

// TxRetryingLockCycles is Tx for a write whose whole transaction may be run
// again when Postgres aborts it as a deadlock victim (40P01) or a
// serialization failure (40001). Nothing an aborted transaction wrote
// survives, so a retry is safe exactly when fn has no effect OUTSIDE the
// transaction — which is the condition a caller opts into by choosing this
// over Tx. The last attempt's error is returned as it came.
//
// A transaction joined from an ambient snapshot is never retried: it is the
// caller's, and a cycle has already aborted it for them too.
func (d *DB) TxRetryingLockCycles(ctx context.Context, fn func(pgx.Tx) error) error {
	if _, joined := snapshotOf(ctx); joined {
		return d.Tx(ctx, fn)
	}
	return retryLockCycles(ctx, lockCycleAttempts, func() error { return d.Tx(ctx, fn) })
}

func retryLockCycles(ctx context.Context, attempts int, run func() error) error {
	var err error
	for attempt := range attempts {
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

// IsLockCycle reports the two refusals a whole-transaction retry clears:
// 40P01, Postgres breaking a deadlock by aborting this transaction, and 40001,
// a serialization failure. Neither says anything about the request.
func IsLockCycle(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && (pgErr.Code == "40P01" || pgErr.Code == "40001")
}
