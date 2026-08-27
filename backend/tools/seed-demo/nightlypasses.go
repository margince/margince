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
// of each pass, in one transaction. A dry run asks for nothing, and no DSN means
// this phase is skipped entirely — the same two conditions every other SQL phase
// carries.
func requestNightlyWorklistPasses(dsn string, mode runMode) (err error) {
	if mode == modeDryRun || dsn == "" {
		return nil
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connecting to request the worklist passes: %w", err)
	}
	// A close that fails after the inserts committed still matters: it is the
	// one signal that the connection was not in the state the writes assumed.
	// It must not mask them, so it only becomes the error when they succeeded.
	defer func() {
		if closeErr := conn.Close(ctx); closeErr != nil && err == nil {
			err = fmt.Errorf("closing the connection that requested the worklist passes: %w", closeErr)
		}
	}()
	// One transaction over all three requests. Half a worklist is worse than
	// none: the surface fills with close-date cards and silently lacks the
	// follow-ups, which reads as a working feature with nothing to say rather
	// than as a seed that failed. The brief's delete joins it for the same
	// reason — it must not survive an insert that never happened.
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("opening the transaction for the worklist passes: %w", err)
	}
	//craft:ignore swallowed-errors a rollback after a failed commit path has no outcome the caller can act on
	defer func() { _ = tx.Rollback(ctx) }()
	if err := requestNightlyPasses(ctx, tx); err != nil {
		return err
	}
	cleared, err := requestTheMorningBrief(ctx, tx)
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing the worklist passes: %w", err)
	}
	reportRequestedPasses(cleared)
	return nil
}

// reportRequestedPasses says what was asked for, after the commit that makes it
// true. Printing inside the transaction would announce passes that a failed
// commit then threw away.
func reportRequestedPasses(cleared int64) {
	fmt.Println("worklist:      close-date and follow-up passes requested — " +
		"cards arrive with the next worker pass")
	fmt.Printf("brief:         %d empty run(s) cleared, morning brief requested — "+
		"it fills once the installation's morning hour has arrived\n", cleared)
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
func requestNightlyPasses(ctx context.Context, tx pgx.Tx) error {
	for _, kind := range nightlyWorklistJobs {
		if _, err := tx.Exec(ctx,
			`INSERT INTO river_job (state, kind, queue, priority, max_attempts, args, scheduled_at)
			 VALUES ('available', $1, 'default', 1, 3, '{}'::jsonb, now())`, kind); err != nil {
			return fmt.Errorf("requesting the %s pass: %w", kind, err)
		}
	}
	return nil
}

// requestTheMorningBrief drops the empty runs and asks for the brief again,
// reporting how many it cleared so the caller can say so after the commit.
//
// It cannot simply be added to nightlyWorklistJobs, because it is suppressed
// rather than idempotent. repsWithoutARunFor anti-joins on brief_run and
// uq_brief_run_user_day makes one run per rep per day permanent, so the hourly
// tick that already ran against the empty installation has claimed today for
// every seat. Asking again writes nothing at all until those runs are gone.
//
// A run whose candidate_count is zero ranked nothing, so it holds no brief_item
// rows and no rep can have acted on one. The emptiness test is spelled out in
// the statement rather than argued here: candidate_count is computed in
// compose/briefs, and a delete that trusted it would rest on another package's
// arithmetic staying true, with nothing in this file failing if it stopped.
//
// It clears every rep's empty run rather than today's. Nothing reads a
// historical run — the brief read resolves today's local day — so the only
// effect is on the next rank's evidence window, which opens to all time. For a
// seed filling an installation from empty that is the intent.
//
// No day filter for a second reason as well: local_day is the worker's clock in
// the installation timezone and current_date is the database server's date in
// its own, so the two can disagree and a day-filtered delete would clear
// nothing while reporting success.
//
// A seed before the installation's briefing hour still fills nothing:
// repsDueTheirMorning finds nobody due, and the brief arrives at the first tick
// after morning. The hour is not repeated here — briefingHour is unexported in
// compose, and a copy of the number would drift the moment the engine's changed.
func requestTheMorningBrief(ctx context.Context, tx pgx.Tx) (int64, error) {
	cleared, err := tx.Exec(ctx, `
		DELETE FROM brief_run br
		WHERE br.candidate_count = 0
		  AND NOT EXISTS (SELECT 1 FROM brief_item bi WHERE bi.brief_run_id = br.id)`)
	if err != nil {
		return 0, fmt.Errorf("clearing the empty brief runs this seed outran: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO river_job (state, kind, queue, priority, max_attempts, args, scheduled_at)
		 VALUES ('available', 'brief_generate', 'default', 1, 3, '{}'::jsonb, now())`); err != nil {
		return 0, fmt.Errorf("requesting the morning brief: %w", err)
	}
	return cleared.RowsAffected(), nil
}
