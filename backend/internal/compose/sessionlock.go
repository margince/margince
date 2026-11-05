// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// sessionLocked marks a job kind whose worker takes a session lock for the
// whole run. Its insert names no uniqueness, because the lock does that work.
type sessionLocked interface{ SessionLocked() }

// sessionUnlockWait bounds the unlock that ends a hold, which has to happen
// after the holder's own context may already be cancelled.
const sessionUnlockWait = 5 * time.Second

// holdSessionLock tries to take the session advisory lock named key and reports
// whether it got it. A false answer means a live holder owns the work.
//
// Session scope keeps the lock across the transactions of one run. The server
// frees it when the holding connection ends, so a killed worker needs no
// cleanup. It never waits: a queued run would wake to work already done.
// Advisory keys are one namespace for the database, so each caller names its own.
func holdSessionLock(ctx context.Context, pool *pgxpool.Pool, key string, log *slog.Logger) (release func(), held bool, err error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("taking a connection for the %s lock: %w", key, err)
	}
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtextextended($1, 0))`, key).Scan(&held); err != nil {
		conn.Release()
		return nil, false, fmt.Errorf("taking the %s lock: %w", key, err)
	}
	if !held {
		conn.Release()
		return nil, false, nil
	}
	return func() {
		unlockCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sessionUnlockWait)
		defer cancel()
		var unlocked bool
		if err := conn.QueryRow(unlockCtx, `SELECT pg_advisory_unlock(hashtextextended($1, 0))`, key).Scan(&unlocked); err != nil || !unlocked {
			// A connection that may still hold the lock is closed, never
			// returned to the pool: the next borrower would carry the lock.
			log.WarnContext(ctx, "the session lock could not be released; closing its connection", "key", key, "err", err)
			if cerr := conn.Hijack().Close(unlockCtx); cerr != nil {
				log.WarnContext(ctx, "closing the connection that held the session lock", "key", key, "err", cerr)
			}
			return
		}
		conn.Release()
	}, true, nil
}
