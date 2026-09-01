// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integrations

// What only a real database can prove about the catch-up sweep.
//
// The failure this lane exists to catch is the one that costs money quietly: a
// selection predicate that keeps re-choosing the same contact. It reads as a
// working sweep — runs are queued, the log says so — while the same twenty-five
// people are bought over and over and the backlog never moves. A unit test
// cannot see it, because the predicate IS SQL and the defect is in what the
// rows say rather than in what the Go does.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

// TestTheSweepNeverAsksTwiceAboutTheSameAnswer runs the sweep twice over a
// contact in every terminal state and asserts what the second pass picks up.
//
// The two halves are one guarantee. A contact the provider ANSWERED — bought,
// or matched nobody — must never be re-selected: that is the loop, and it
// spends real credits every minute forever. A contact the platform REFUSED
// must come back once its cooldown passes: that is the opposite defect, where
// one bad afternoon excludes somebody permanently and the backlog count reports
// them as done.
func TestTheSweepNeverAsksTwiceAboutTheSameAnswer(t *testing.T) {
	e := setupRuns(t, runsConfig{})
	ctx := context.Background()

	// A contact per state, plus one nobody has asked about at all.
	answered := map[string]ids.PersonID{}
	for _, state := range []string{"completed", "no_match", "queued", "in_progress"} {
		answered[state] = e.plantSubjectWithRun(t, state)
	}
	refused := map[string]ids.PersonID{}
	for _, state := range []string{"skipped", "failed", "cancelled", "submission_unknown"} {
		refused[state] = e.plantSubjectWithRun(t, state)
	}

	selected := e.sweepSelects(t)
	for state, id := range answered {
		if selected[id] {
			t.Errorf("a %s run was re-selected — the provider already answered for this contact, "+
				"and asking again spends a credit every tick forever", state)
		}
	}
	for state, id := range refused {
		if selected[id] {
			t.Errorf("a %s run was re-selected inside its cooldown; the contact should wait", state)
		}
	}

	// Age every refusal past the cooldown. What was declined comes back; what
	// was answered still does not.
	if _, err := e.owner.Exec(ctx, `
		UPDATE provider_run SET created_at = now() - interval '2 days'
		 WHERE state IN ('skipped', 'failed', 'cancelled', 'submission_unknown')`); err != nil {
		t.Fatal(err)
	}

	selected = e.sweepSelects(t)
	for state, id := range refused {
		if !selected[id] {
			t.Errorf("a %s run older than the cooldown was NOT re-selected — the contact is "+
				"excluded for good, and the backlog reports them as covered", state)
		}
	}
	for state, id := range answered {
		if selected[id] {
			t.Errorf("a %s run was re-selected after the cooldown aged the refusals; "+
				"an answer does not expire", state)
		}
	}
}

// TestTheSweepSpendsNoMoreThanTheDayAllows proves the budget binds.
//
// The ceiling is the customer's stated limit on what a day may cost, and the
// sweep is the only thing in this system that queues work nobody asked for —
// so a budget that leaked would spend money on a standing instruction the
// customer capped.
func TestTheSweepSpendsNoMoreThanTheDayAllows(t *testing.T) {
	e := setupRuns(t, runsConfig{})
	ctx := context.Background()

	// Count what the fixture has already set in motion today: the ceiling is
	// about the DAY, not about this test, so the assertion is relative to
	// whatever the environment starts with.
	var already int
	if err := e.owner.QueryRow(ctx, `
		SELECT count(*) FROM provider_run
		 WHERE provider = $1 AND state <> 'skipped' AND state <> 'cancelled'
		   AND created_at >= date_trunc('day', now() AT TIME ZONE 'UTC')`,
		e.provider).Scan(&already); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(ctx,
		`UPDATE provider_connection SET daily_run_limit = $2 WHERE provider = $1`,
		e.provider, already+2); err != nil {
		t.Fatal(err)
	}
	for range 5 {
		e.plantSubject(t)
	}

	budget, err := e.sweepBudgetNow(t)
	if err != nil {
		t.Fatal(err)
	}
	if budget != 2 {
		t.Fatalf("budget %d, want 2 — the day's remaining allowance is what a tick may queue", budget)
	}

	// Two runs already set in motion spend the whole allowance, queued
	// included: they have not been submitted yet, but they will be.
	e.plantSubjectWithRun(t, "queued")
	e.plantSubjectWithRun(t, "queued")
	budget, err = e.sweepBudgetNow(t)
	if err != nil {
		t.Fatal(err)
	}
	if budget != 0 {
		t.Errorf("budget %d with the day's limit already queued, want 0 — a queued run is "+
			"work the sweep has already committed the customer to", budget)
	}
}

// sweepSelects runs the predicate and returns who it chose.
func (e *runsEnv) sweepSelects(t *testing.T) map[ids.PersonID]bool {
	t.Helper()
	var chosen []string
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		var err error
		chosen, err = e.store.uncoveredSubjects(e.ctx, tx, e.provider, sweepTickBudget)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	out := map[ids.PersonID]bool{}
	for _, id := range chosen {
		parsed, err := ids.Parse(id)
		if err != nil {
			t.Fatal(err)
		}
		out[ids.PersonID{UUID: parsed}] = true
	}
	return out
}

// sweepBudgetNow reads what this tick may queue.
func (e *runsEnv) sweepBudgetNow(t *testing.T) (int, error) {
	t.Helper()
	var budget int
	err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		var err error
		budget, err = e.store.sweepBudget(e.ctx, tx, e.provider)
		return err
	})
	return budget, err
}

// plantSubject adds a contact nobody has asked the provider about.
func (e *runsEnv) plantSubject(t *testing.T) ids.PersonID {
	t.Helper()
	var id ids.PersonID
	if err := e.owner.QueryRow(context.Background(), `
		INSERT INTO person (full_name, first_name, source, captured_by)
		VALUES ('Sweep Subject', 'Sweep', 'test', 'test:sweep')
		RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// plantSubjectWithRun adds a contact whose one run is in the given state.
func (e *runsEnv) plantSubjectWithRun(t *testing.T, state string) ids.PersonID {
	t.Helper()
	id := e.plantSubject(t)
	var skipReason *string
	if state == string(provider.RunSkipped) {
		reason := string(provider.SkipRateLimited)
		skipReason = &reason
	}
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO provider_run
		       (person_id, subject_kind, provider, trigger, state, skip_reason,
		        connection_version, connection_epoch, configuration_snapshot,
		        requested_categories, input_fingerprint, external_correlation_id)
		VALUES ($1, 'person', $2, 'automatic_create', $3, $4, 1, 1, '{}'::jsonb,
		        ARRAY['linkedin_profile'], $5, gen_random_uuid())`,
		id, e.provider, state, skipReason, "fp-"+state+"-"+id.String()); err != nil {
		t.Fatal(err)
	}
	return id
}

// TestAHumanCanProbeADegradedConnection is the escape from a trap that shipped.
//
// A vendor error flips the connection to provider_error. Execution treats that
// as recoverable and says so — it authorizes egress precisely because a later
// success restores `connected`. Admission used to refuse to CREATE that run, so
// the sentence was unreachable: one bad minute from the vendor left the
// connection refusing every lookup, and the only way out was a human noticing
// and re-entering a key that was never at fault.
//
// A person pressing the button is the deliberate probe. A sweep is not: nobody
// is watching it, and retrying a rate limit every minute is how a transient
// limit becomes a sustained one.
func TestAHumanCanProbeADegradedConnection(t *testing.T) {
	e := setupRuns(t, runsConfig{})
	ctx := context.Background()

	for _, status := range []string{"rate_limited", "insufficient_credits", "provider_error"} {
		if _, err := e.owner.Exec(ctx,
			`UPDATE provider_connection SET status = $2 WHERE provider = $1`,
			e.provider, status); err != nil {
			t.Fatal(err)
		}

		if _, err := e.store.QueueRun(e.ctx, provider.QueueInput{
			PersonID: e.mine.String(), Provider: e.provider, Trigger: provider.TriggerManual,
		}); err != nil {
			t.Errorf("a human's run was refused on a %s connection: %v — the only path back to "+
				"connected is a run that succeeds, so refusing them all makes the state permanent",
				status, err)
		}

		_, err := e.store.QueueRun(e.ctx, provider.QueueInput{
			PersonID: e.theirsInBook.String(), Provider: e.provider,
			Trigger: provider.TriggerAutomaticBackfill,
		})
		if err == nil {
			t.Errorf("a sweep queued work on a %s connection; automatic retries are how a "+
				"transient vendor condition becomes a sustained one", status)
		}
	}
}

// A completed run with no claims stored yet must not be recorded as applied.
//
// The terminal state commits in one transaction and the hand-off writes the
// claims in the next, so a sweep tick landing between them finds a completed
// run with nothing to fold. Treating that as success stamps applied_at over a
// purchase the record never received — and applied_at is what a waiting client
// reads, so it stops one step before the values exist and nothing comes back to
// say they arrived.
func TestASweepDoesNotCallAnEmptyRunApplied(t *testing.T) {
	e := setupRuns(t, runsConfig{})
	runID := seedCompletedRunWithoutClaims(t, e)

	var applied int
	e.store.WithStoredClaimApplier(
		func(context.Context, pgx.Tx, string, string) (bool, error) {
			applied++
			// What the real applier answers with nothing stored.
			return false, nil
		})
	e.store.WithSubjectHold(
		func(context.Context, pgx.Tx, string) (FenceVerdict, error) {
			return FenceVerdict{Allowed: true}, nil
		})

	if err := e.store.applyStoredPurchases(e.ctx, "surfe"); err != nil {
		t.Fatal(err)
	}
	if applied == 0 {
		t.Fatal("the sweep never reached the run, so this case proves nothing about the stamp")
	}

	var stamped *time.Time
	if err := e.owner.QueryRow(context.Background(),
		`SELECT applied_at FROM provider_run WHERE id = $1`, runID).Scan(&stamped); err != nil {
		t.Fatal(err)
	}
	if stamped != nil {
		t.Error("a run with no stored claims was stamped as applied: the page stops waiting " +
			"on that stamp, so the values land after nothing is watching for them")
	}
}

// seedCompletedRunWithoutClaims is the window between the terminal commit and
// the hand-off: completed, not exhausted, and holding nothing.
func seedCompletedRunWithoutClaims(t *testing.T, e *runsEnv) string {
	t.Helper()
	var runID string
	if err := e.owner.QueryRow(context.Background(), `
		INSERT INTO provider_run
		  (subject_kind, person_id, provider, trigger, state, input_fingerprint,
		   external_correlation_id, connection_version, connection_epoch,
		   configuration_snapshot, requested_categories, completed_at)
		VALUES ('person', $1, 'surfe', 'manual', 'completed', $2,
		        gen_random_uuid(), 1, 1, '{}'::jsonb, ARRAY['linkedin_profile'], now())
		RETURNING id::text`, e.mine, "fp-unstored-"+e.mine.String()).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	return runID
}
