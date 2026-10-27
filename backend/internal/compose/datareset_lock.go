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
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// resetLockKey names the advisory lock. The string is what a reader greps for.
const resetLockKey = "margince:data-reset"

// releaseReset drops a held reset lock. Always safe to call, including on the
// no-op a composition without a pool produces, so the caller defers it once
// without asking whether there was a lock.
type releaseReset func()

// takeResetLock acquires the installation-wide reset lock, or reports that
// somebody else holds it.
//
// An operator who fires a second reset while one is running is told so at once.
// A queued one would hold an HTTP request open for a full sweep and then wipe an
// installation the operator has stopped thinking about.
func takeResetLock(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) (releaseReset, error) {
	// No pool is a composition that cannot serialize, the unit harness. With
	// one reset there is nothing to keep apart.
	if pool == nil {
		return func() {}, nil
	}
	release, held, err := holdSessionLock(ctx, pool, resetLockKey, log)
	if err != nil {
		return nil, fmt.Errorf("data reset: %w", err)
	}
	if !held {
		return nil, fmt.Errorf("another data reset is already running on this installation: %w",
			apperrors.ErrConflict)
	}
	return release, nil
}
