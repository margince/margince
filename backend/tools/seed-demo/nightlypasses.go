// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// Asking the nightly passes to run once, at the end of a seed.
//
// Three jobs fill the two surfaces a demo opens on. The close-date corrector
// and the follow-up reconciler stage the worklist's cards: one asks a human to
// confirm the real date on a deal that has gone quiet, the other proposes the
// reply nobody sent. The morning brief ranks the deals themselves. All three
// are nightly and all three read a deal's age — so on a freshly seeded
// installation they have nothing to say for a day, and both surfaces are empty.
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
	if err := requestNightlyPasses(ctx, conn); err != nil {
		return err
	}
	return requestTheMorningBrief(ctx, conn)
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

// requestTheMorningBrief drops today's empty runs and asks for the brief again.
//
// The brief ranks the same deals the worklist passes read, and it opens the
// installation — so a seed that leaves it empty leaves the demo's first screen
// saying the overnight brief found nothing worth the first hour, which is a
// sentence about a database that had no deals rather than about the pipeline.
//
// It cannot simply be added to nightlyWorklistJobs, because it is suppressed
// rather than idempotent. repsWithoutARunFor anti-joins on brief_run and
// uq_brief_run_user_day makes one run per rep per day permanent, so the hourly
// tick that already ran against the empty installation has claimed today for
// every seat. Asking again writes nothing at all until those runs are gone.
//
// Deleting them is safe HERE and only here: a run whose candidate_count is zero
// ranked nothing, so it holds no brief_item rows and no rep can have acted on
// one. That emptiness is the whole filter, and it is deliberately the ONLY one —
// naming a day here would be a second opinion about which day it is. local_day
// is the installation timezone applied to the WORKER's clock, while current_date
// is the database server's date in its own; the two disagree whenever those
// clocks or zones do, and a mismatch would clear nothing while reporting
// success, leaving the suppression in place and the brief empty.
//
// One case this deliberately does NOT cover: a seed run before briefingHour in
// the installation's timezone. repsDueTheirMorning returns nobody until the
// local day reaches that hour, so the requested pass completes having written
// nothing, and the brief fills at the tick after the installation's morning
// arrives. Forcing it would mean holding a second opinion about when morning is,
// which is the engine's to hold — so the seed says what it asked for rather than
// promising cards that a night-time run cannot produce.
func requestTheMorningBrief(ctx context.Context, conn *pgx.Conn) error {
	cleared, err := conn.Exec(ctx, `DELETE FROM brief_run WHERE candidate_count = 0`)
	if err != nil {
		return fmt.Errorf("clearing the empty brief runs this seed outran: %w", err)
	}
	if _, err := conn.Exec(ctx,
		`INSERT INTO river_job (state, kind, queue, priority, max_attempts, args, scheduled_at)
		 VALUES ('available', 'brief_generate', 'default', 1, 3, '{}'::jsonb, now())`); err != nil {
		return fmt.Errorf("requesting the morning brief: %w", err)
	}
	// The hour itself is not repeated here: briefingHour is unexported in the
	// compose package and a copy of the number would be a second answer to when
	// morning is, drifting the moment the engine's changed.
	fmt.Printf("brief:         %d empty run(s) cleared, morning brief requested — "+
		"it fills once the installation's morning hour has arrived\n", cleared.RowsAffected())
	return nil
}
