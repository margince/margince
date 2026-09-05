// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// Request the scheduled readers after the seed has finished writing their inputs.
// The scheduler may already have run against the empty installation at boot.
// Queueing its declared jobs lets the production workers fill the demo surfaces.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/jobs"
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
	// One transaction over all requests. Half a worklist is worse than
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
	fmt.Println("weekly review: requested — existing frozen reviews are retained; new reviews use the seeded records")
	fmt.Println("finance sync:  requested — invoices and payments arrive with the next worker pass")
	fmt.Println("worklist:      close-date and follow-up passes requested — " +
		"cards arrive with the next worker pass")
	fmt.Printf("brief:         %d empty run(s) cleared, morning brief requested — "+
		"it fills once the installation's morning hour has arrived\n", cleared)
}

// nightlyWorklistJobs populate finance and the worklist after all records and
// customer links exist. Each dispatcher resolves its own workspace population.
var nightlyWorklistJobs = []string{"finance_sync_sweep", "close_date_sweep", "follow_up_reconcile", "weekly_review_generate"}

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
		if err := requestSeedPass(ctx, tx, kind); err != nil {
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
	if err := requestSeedPass(ctx, tx, "brief_generate"); err != nil {
		return 0, fmt.Errorf("requesting the morning brief: %w", err)
	}
	return cleared.RowsAffected(), nil
}

// requestSeedPass refuses stale job names before the seed can announce success.
// The declaration owns the queue and whether an empty payload is meaningful.
func requestSeedPass(ctx context.Context, tx pgx.Tx, kind string) error {
	spec, ok := jobs.SpecFor(kind)
	if !ok || len(spec.Args) != 0 {
		return fmt.Errorf("seed pass %q is not a declared, argument-free job", kind)
	}
	_, err := tx.Exec(ctx,
		`INSERT INTO river_job (state, kind, queue, priority, max_attempts, args, scheduled_at)
		 VALUES ('available', @kind, @queue, 1, 3, '{}'::jsonb, now())`,
		pgx.NamedArgs{"kind": kind, "queue": spec.Queue})
	return err
}
