// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The nightly input check against a real database.
//
// It has to be an integration test rather than a unit one, because what was
// missing was never a function — Scan, every rule and both seams have been here
// and covered all along — but a CALLER wired to them. What is asserted here is
// the thing whose absence nothing could fail on: that after the pass runs,
// LatestRun answers a run.
//
// That read is what the Forecast tab opens on. Before this job existed it
// returned ErrNotFound on every installation for ever, and the tab rendered it
// as "Couldn't load this view." above a dead Retry — the one element on the
// page that looks like the application is broken, on every load, for every
// seat.

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// assuranceJobEnv is one workspace holding an open deal for the pass to examine.
type assuranceJobEnv struct {
	*integration.Env
	worker *assuranceSweepWorker
	at     time.Time
}

func setupAssuranceJob(t *testing.T) *assuranceJobEnv {
	t.Helper()
	e := integration.Setup(t)
	pipeline, open, _ := integration.DealFixture(t, e)
	at := time.Now().UTC()

	// One live open deal, so the run has a subject and its eligible count is
	// not zero. A pass over an empty pipeline writes a run too, and this suite
	// could not tell that apart from a pass that read nothing at all.
	owner := integration.OwnerConn(t)
	if _, err := owner.Exec(context.Background(), `
		INSERT INTO deal (pipeline_id, stage_id, name, owner_id, status, source, captured_by,
		                  amount_minor, currency, expected_close_date)
		VALUES ($1, $2, 'Assurance Fixture', $3, 'open', 'manual', 'test', 4200000, 'EUR', $4)`,
		pipeline, open, e.Rep1, at.AddDate(0, 0, 3)); err != nil {
		t.Fatalf("seeding the deal the check examines: %v", err)
	}
	return &assuranceJobEnv{
		Env: e,
		worker: &assuranceSweepWorker{
			pool: e.Pool,
			now:  func() time.Time { return at },
			log:  slog.New(slog.DiscardHandler),
		},
		at: at,
	}
}

// run drives the worker's per-workspace turn, which is what River's row now
// walks rather than what it carries: the pass takes no workspace in its args
// (ADR-0103), so Work would enumerate the fleet and lose the one this suite is
// about. It is the same code Work calls per tenant.
func (e *assuranceJobEnv) run(t *testing.T) error {
	t.Helper()
	return e.worker.assureWorkspace(context.Background(), e.WS)
}

// latestRun is the read the Forecast tab opens on, taken as an ordinary seat
// rather than as the fleet: what this ticket is about is what a PERSON gets.
func (e *assuranceJobEnv) latestRun(t *testing.T) (assurance.Run, error) {
	t.Helper()
	return assurance.NewStore(InstallationDB(e.Pool)).LatestRun(e.Admin())
}

// Before the pass exists there is nothing to read, and that is the state the
// whole ticket is about — asserted here so the case below is a change rather
// than a coincidence.
func TestTheForecastReviewHasNothingToReadUntilTheCheckHasRun(t *testing.T) {
	e := setupAssuranceJob(t)

	if _, err := e.latestRun(t); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("a workspace whose inputs have never been checked answered %v, "+
			"wanted ErrNotFound — the rest of this suite proves nothing if the read "+
			"was already satisfied before the pass ran", err)
	}
}

func TestTheNightlyCheckLeavesARunTheForecastReviewCanRead(t *testing.T) {
	e := setupAssuranceJob(t)

	if err := e.run(t); err != nil {
		t.Fatalf("the nightly check: %v", err)
	}

	run, err := e.latestRun(t)
	if err != nil {
		t.Fatalf("reading the run the check just wrote: %v — this is the 404 the "+
			"Forecast tab renders as a failed panel", err)
	}
	if run.Status == assurance.StatusRunning {
		t.Error("the pass left its run at `running`, which LatestRun skips — the tab " +
			"would read 404 with a finished-looking row sitting in the table")
	}
	if run.Readiness == nil || *run.Readiness == "" {
		t.Error("the run carries no readiness verdict, which is the whole of what the " +
			"panel reads it for")
	}
	if run.EligibleDeals == 0 {
		t.Error("the run examined no deals, so it was written by a pass that read " +
			"nothing — the fixture seeds one live open deal for it to find")
	}
}

// A second pass in one day is ordinary: the dispatcher ticks more than once so
// a worker that was down still backfills, and River retries a failed attempt.
// Nothing arbitrates assurance_run to one row per day, and nothing should —
// each pass is a fresh answer and the reader takes the newest — so the second
// run must succeed and must be the one the tab then reads.
func TestASecondCheckSupersedesTheFirstRatherThanFailing(t *testing.T) {
	e := setupAssuranceJob(t)

	if err := e.run(t); err != nil {
		t.Fatalf("the first check: %v", err)
	}
	first, err := e.latestRun(t)
	if err != nil {
		t.Fatalf("reading the first run: %v", err)
	}

	e.worker.now = func() time.Time { return e.at.Add(time.Hour) }
	if err := e.run(t); err != nil {
		t.Fatalf("the second check: %v", err)
	}

	second, err := e.latestRun(t)
	if err != nil {
		t.Fatalf("reading the second run: %v", err)
	}
	if second.ID == first.ID {
		t.Error("the second pass wrote no run of its own, so the tab keeps reading " +
			"a verdict from a check that ran earlier than the one that just finished")
	}
}
