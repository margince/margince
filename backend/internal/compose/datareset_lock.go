// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// One reset at a time, installation-wide.
//
// A reset pauses the job fleet, sweeps Postgres and purges four surfaces no
// transaction reaches. None of that is safe to interleave with a SECOND reset,
// and the sharpest way it fails is the pause: the resume is deferred on every
// exit — deliberately, because a pause with nobody left to lift it wedges every
// queue — so two overlapping resets have two deferred resumes, and whichever
// finishes first lifts the pause the other is still sweeping under. The fleet
// then starts writing into a half-swept installation.
//
// A LOCK, not a status column. The state to protect is "a reset is running in
// this process, right now", and a row saying so outlives the process that wrote
// it: a reset killed mid-sweep would leave the installation refusing every
// later reset until somebody cleared the row by hand — turning a crash into a
// second outage. A session-level advisory lock is released by Postgres when the
// connection goes, whatever happened to the caller.
//
// It is installation-wide rather than per workspace, and that is not laziness.
// The surfaces a reset purges past the sweep are not workspace-scoped in the
// same transaction: the queue, the bus, the object prefix and the vault each
// have their own boundary, and the fleet pause is global by construction. A
// per-workspace lock would let two resets serialize on nothing while both held
// the fleet down.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// resetLockKey names the advisory lock. hashtextextended folds it to the bigint
// the lock space takes; the string is what a reader greps for.
const resetLockKey = "margince:data-reset"

// resetUnlockTimeout bounds the release. Short: it is one statement on a
// connection this process already holds.
const resetUnlockTimeout = 10 * time.Second

// releaseReset drops a held reset lock. Always safe to call, including on the
// no-op a composition without a pool produces, so the caller defers it once
// without asking whether there was a lock.
type releaseReset func()

// takeResetLock acquires the installation-wide reset lock, or reports that
// somebody else holds it.
//
// SESSION-level (pg_try_advisory_lock, not the _xact_ variant) because a reset
// is not one transaction: the sweep commits and the purges, the announcement
// and the resume all run after it, and a transaction-scoped lock would drop at
// the commit — leaving the whole detached tail unprotected, which is the half
// where the object store and the vault are touched.
//
// TRY, never a wait. An operator who fires a second reset while one is running
// gets told so, immediately; a queued one would sit holding an HTTP request
// open for as long as a full sweep takes and then wipe an installation the
// operator has by then stopped thinking about.
func takeResetLock(ctx context.Context, pool *pgxpool.Pool) (releaseReset, error) {
	// No pool is a composition that cannot serialize — the unit harness. It
	// gets a no-op rather than a refusal: this function's job is to keep two
	// resets apart, and where there is only one there is nothing to keep apart.
	if pool == nil {
		return func() {}, nil
	}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("data reset: taking a connection for the reset lock: %w", err)
	}
	var held bool
	if err := conn.QueryRow(ctx,
		`SELECT pg_try_advisory_lock(hashtextextended($1, 0))`, resetLockKey).Scan(&held); err != nil {
		conn.Release()
		return nil, fmt.Errorf("data reset: taking the reset lock: %w", err)
	}
	if !held {
		conn.Release()
		return nil, fmt.Errorf("another data reset is already running on this installation: %w",
			apperrors.ErrConflict)
	}
	return func() {
		// Released on a context of its own, and the connection goes back
		// either way. The reset's own context is routinely cancelled by the
		// time this runs — an operator's client abandons the request, which is
		// exactly when the lock most needs dropping — and an unlock that never
		// ran would hold this installation's resets out until the pool recycled
		// the connection.
		unlockCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), resetUnlockTimeout)
		defer cancel()
		_, err := conn.Exec(unlockCtx, `SELECT pg_advisory_unlock(hashtextextended($1, 0))`, resetLockKey)
		conn.Release()
		if err != nil {
			// Not returned: the reset itself has already succeeded or failed on
			// its own terms, and a failed unlock must not turn a completed
			// wipe into a reported failure. The lock goes with the connection.
			return
		}
	}, nil
}
