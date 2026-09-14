// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package jobs

// Mutating river_job outside the insert path Runner already owns. The
// package that runs the fleet is the package that writes its own table
// (tableownership_test.go) — a caller with a reason to reprioritize an
// already-queued job reaches through here rather than reissuing the
// UPDATE itself, so river_job's write shape has one home, not one per
// caller with a reason to touch it.

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PromotePriority raises the priority of one queued job of the given kind,
// matched by a single top-level string arg (e.g. a dossier or dedupe id),
// to the given value. It is a raise only: a job already at or above the
// requested priority is left alone, so a caller cannot accidentally demote
// one by calling this with a lower urgency than what is already queued.
//
// Scoped to states a worker has not yet claimed (available, scheduled,
// retryable) — a running or finished job has nothing left to reprioritize,
// and touching it would race the worker executing it.
//
// Reports whether a row was actually changed: false is not an error, and
// covers every reason there was nothing to promote — the job already ran,
// already carries this priority, or the caller's own uniqueness match found
// no row. A caller that only wanted to know a job was already in flight can
// use this as a best-effort nudge without needing to distinguish those cases.
func PromotePriority(ctx context.Context, pool *pgxpool.Pool, kind, argKey, argValue string, priority int) (bool, error) {
	tag, err := pool.Exec(ctx, `
		UPDATE river_job SET priority = $1
		WHERE kind = $2 AND state IN ('available', 'scheduled', 'retryable')
		  AND args ->> $3 = $4 AND priority > $1`,
		priority, kind, argKey, argValue)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}
