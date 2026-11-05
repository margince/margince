// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
)

// sessionLocked marks a job kind whose worker takes a session lock for the
// whole run. Its insert names no uniqueness, because the lock does that work.
type sessionLocked interface{ SessionLocked() }

// holdSessionLock tries to take the session advisory lock named key and reports
// whether it got it. A false answer means a live holder owns the work.
//
// Session scope keeps the lock across the transactions of one run. The server
// frees it when the holding connection ends, so a killed worker needs no
// cleanup. It never waits: a queued run would wake to work already done.
// Advisory keys are one namespace for the database, so each caller names its own.
func holdSessionLock(ctx context.Context, pool *pgxpool.Pool, key string, log *slog.Logger) (release func(), held bool, err error) {
	ctx = database.WithSessionBudget(ctx, takeSessionSlot)
	lease, held, err := database.TrySessionLease(ctx, pool, key, log)
	if err != nil || !held {
		return nil, held, err
	}
	return func() { lease.Close(ctx) }, true, nil
}
