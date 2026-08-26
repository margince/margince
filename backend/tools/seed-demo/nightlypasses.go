// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// Asking the nightly passes to run once, at the end of a seed.
//
// The close-date corrector and the follow-up reconciler are the two jobs that
// FILL the worklist: one asks a human to confirm the real date on a deal that
// has gone quiet, the other proposes the reply nobody sent. Both are nightly,
// and both read a deal's age — so on a freshly seeded installation they have
// nothing to say for a day, and the surface a demo opens on is empty.
//
// That empty screen reads as a broken feature rather than a young database, and
// it is not one somebody can wait out: the sweeps did already run, at boot,
// against deals that did not exist yet. This is the same failure the finance
// sync hit (see requestFinanceSync) and it is asked the same way — a row on
// River's queue, which is how the scheduler itself asks. Reaching into the
// deals module instead would be a second way to start a pass that already has
// one.
//
// Ordering is the whole point of running this LAST. The passes read
// last_activity_at, which the mailbox seeding sets when it writes the
// correspondence; asked before that, they would look at deals whose newest
// activity is their own creation and find nothing stale.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// requestNightlyWorklistPasses opens the owner connection and asks for one run
// of each pass. A dry run asks for nothing, and no DSN means this phase is
// skipped entirely — the same two conditions every other SQL phase carries.
func requestNightlyWorklistPasses(dsn string, mode runMode) error {
	if mode == modeDryRun || dsn == "" {
		return nil
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connecting to request the worklist passes: %w", err)
	}
	//craft:ignore swallowed-errors closing a seed connection has no failure the caller can act on
	defer func() { _ = conn.Close(ctx) }()
	return requestNightlyPasses(ctx, conn)
}

// nightlyWorklistJobs are the passes that stage the worklist's cards. Both are
// workspace-scoped fan-outs: the job layer expands them per workspace, which is
// why neither takes a workspace id here.
var nightlyWorklistJobs = []string{"close_date_sweep", "follow_up_reconcile"}

// requestNightlyPasses queues one run of each worklist pass.
//
// Idempotent by intent rather than by key, exactly as the finance sync is: a
// second pass over the same deals stages nothing new — the correctors ask
// whether a proposal is already pending before they raise one — so a duplicate
// row costs one job and writes nothing twice.
//
// A stack with no worker running leaves the rows waiting, which is the right
// outcome: the cards appear when the worker comes up rather than the seed
// failing over a queue nobody is reading.
func requestNightlyPasses(ctx context.Context, conn *pgx.Conn) error {
	for _, kind := range nightlyWorklistJobs {
		if _, err := conn.Exec(ctx,
			`INSERT INTO river_job (state, kind, queue, priority, max_attempts, args, scheduled_at)
			 VALUES ('available', $1, 'default', 1, 3, '{}'::jsonb, now())`, kind); err != nil {
			return fmt.Errorf("requesting the %s pass: %w", kind, err)
		}
	}
	fmt.Println("worklist:      close-date and follow-up passes requested — " +
		"cards arrive with the next worker pass")
	return nil
}
