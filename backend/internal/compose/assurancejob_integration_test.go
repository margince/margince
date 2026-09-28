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

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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

	// A deal the rules FAULT, and fault more than once: its expected close is in
	// the past (close_past, high) and it is committed with no next step
	// (no_next_step, low). The healthy deal above is what keeps the eligible
	// count honest; this one is what gives the bundling suite something to
	// bundle. Two findings on ONE deal is the shape #4004 is about — a rep
	// clearing them should clear one task, not two.
	if _, err := owner.Exec(context.Background(), `
		INSERT INTO deal (pipeline_id, stage_id, name, owner_id, status, source, captured_by,
		                  amount_minor, currency, expected_close_date, forecast_category)
		VALUES ($1, $2, 'Assurance Faulted', $3, 'open', 'manual', 'test', 3100000, 'EUR', $4, 'commit')`,
		pipeline, open, e.Rep1, at.AddDate(0, 0, -7)); err != nil {
		t.Fatalf("seeding the deal the check faults: %v", err)
	}

	return &assuranceJobEnv{
		Env: e,
		worker: &assuranceSweepWorker{
			pool: e.Pool,
			now:  func() time.Time { return at },
			log:  slog.New(slog.DiscardHandler),
			// The REAL activities store, not a double. The task a finding is
			// bundled onto has to be one the rest of the product renders, so a
			// stub here would prove the seam calls something rather than that a
			// rep is handed a task.
			activities: activities.NewStore(InstallationDB(e.Pool)),
		},
		at: at,
	}
}

// run drives the pass a human asked for, which is also the only thing that
// STARTS a workspace: the nightly sweep skips one nobody has started, so a
// suite that reached for the sweep here would be asserting against a pass that
// never ran. Work rather than the inner call because these args carry their
// workspace, so there is no fleet to enumerate.
func (e *assuranceJobEnv) run(t *testing.T) error {
	t.Helper()
	worker := &assuranceRunWorker{sweep: e.worker}
	return worker.Work(context.Background(), &river.Job[AssuranceRunArgs]{
		Args: AssuranceRunArgs{Workspace: e.WS, RequestedBy: e.Rep1.String()},
	})
}

// sweep drives the NIGHTLY turn, enrolment gate and all — the same code Work
// calls per tenant (the sweep's args carry no workspace, ADR-0103, so its own
// Work would enumerate the fleet and lose the one this suite is about).
func (e *assuranceJobEnv) sweep(t *testing.T) error {
	t.Helper()
	return e.worker.assureWorkspace(context.Background(), e.WS)
}

// The nightly sweep does not check a workspace nobody has started.
//
// This is the whole of the first-use guarantee. The first pass over a backlog
// raises every exception it can see at once and mints a task per faulted deal,
// and a queue that arrives on the calendar's schedule is one nobody chose.
func TestTheNightlySweepSkipsAWorkspaceNobodyHasStarted(t *testing.T) {
	e := setupAssuranceJob(t)

	if err := e.sweep(t); err != nil {
		t.Fatalf("the nightly sweep: %v", err)
	}

	if _, err := e.latestRun(t); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("the sweep checked a workspace nobody started (read answered %v) — "+
			"a fresh installation wakes up to a wall of exceptions it never asked for", err)
	}
}

// Once somebody has started it, the sweep checks it every night as before.
//
// This is the direction that can fail SHORT: a workspace wrongly read as
// unstarted simply stops being checked, the read keeps answering the last run,
// and nothing asserts. An installation already being checked must never enter
// that branch.
func TestTheNightlySweepChecksAWorkspaceSomebodyStarted(t *testing.T) {
	e := setupAssuranceJob(t)

	if err := e.run(t); err != nil {
		t.Fatalf("starting the workspace: %v", err)
	}
	first, err := e.latestRun(t)
	if err != nil {
		t.Fatalf("reading the run that started the workspace: %v", err)
	}

	if err := e.sweep(t); err != nil {
		t.Fatalf("the nightly sweep: %v", err)
	}

	second, err := e.latestRun(t)
	if err != nil {
		t.Fatalf("reading the run the sweep wrote: %v", err)
	}
	if second.ID == first.ID {
		t.Error("the sweep skipped a workspace that has already been checked — it " +
			"would quietly stop checking an installation that was working, and the " +
			"panel would keep showing the last run as though it were current")
	}
}

// A pass somebody asked for names them on the run; the nightly one names nobody.
func TestTheRequestedPassRecordsWhoAskedAndTheSweepDoesNot(t *testing.T) {
	e := setupAssuranceJob(t)

	if err := e.run(t); err != nil {
		t.Fatalf("the requested pass: %v", err)
	}
	requested, err := e.latestRun(t)
	if err != nil {
		t.Fatal(err)
	}
	if got := e.requesterOf(t, requested.ID); got != e.Rep1.String() {
		t.Errorf("the requested run says %q asked for it, want %q", got, e.Rep1)
	}

	if err := e.sweep(t); err != nil {
		t.Fatalf("the nightly sweep: %v", err)
	}
	nightly, err := e.latestRun(t)
	if err != nil {
		t.Fatal(err)
	}
	if got := e.requesterOf(t, nightly.ID); got != "" {
		t.Errorf("the nightly run says %q asked for it; nobody did", got)
	}
}

// requesterOf reads who asked for one run, empty for the nightly cadence.
func (e *assuranceJobEnv) requesterOf(t *testing.T, run ids.UUID) string {
	t.Helper()
	var out *string
	if err := integration.OwnerConn(t).QueryRow(context.Background(),
		`SELECT requested_by FROM assurance_run WHERE id = $1`, run).Scan(&out); err != nil {
		t.Fatalf("reading the run's requester: %v", err)
	}
	if out == nil {
		return ""
	}
	return *out
}

// latestRun is the read the Forecast tab opens on, taken as an ordinary seat
// rather than as the fleet: what this ticket is about is what a CONTACT gets.
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
