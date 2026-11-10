// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
)

// taskDriver is what every run of one task shares: its lanes, the record's
// accumulation, and the trace and journal.
type taskDriver struct {
	task   ai.Task
	census *aitasks.Registry
	// lanes holds one router pair per worker; the task's runs are in flight on
	// at most len(lanes) of them at once.
	lanes   []runLane
	log     *slog.Logger
	acc     *taskAccumulation
	trace   *payloadTrace
	journal taskJournal
	// maxRuns is the most runs any case reaches: adaptiveMaxRuns in production.
	maxRuns int
}

// pendingRun is a run measured or replayed but not yet folded into the task.
type pendingRun struct {
	measured measuredRun
	replayed bool
}

// runScenarios drives every scenario of the task in adaptive rounds (runRounds)
// and returns their run sets for certifyTask to judge together. The per-run
// degrade gates sit in measureRun because they void the whole task. A demoted
// answer or grader anywhere in the set means no record, not a lower band.
//
// One lane scores each run and folds it before the next begins. More lanes
// measure a round's runs concurrently and fold them in run order, so the
// record is the one a single lane would build.
func (d taskDriver) runScenarios(ctx context.Context, scenarios []Scenario, stamps map[string]string, repeats int) ([]ScenarioRuns, error) {
	seeds := caseSeeds(scenarios)
	first := firstRunCounts(seeds, repeats)
	measure := func(ctx context.Context, worker int, job runJob) (pendingRun, error) {
		sc := scenarios[job.caseIndex]
		if firstRun := first[job.caseIndex]; job.run > firstRun && (job.run-firstRun-1)%adaptiveRound == 0 {
			d.log.InfoContext(ctx, "aicert: extending a borderline case by another round",
				"task", string(d.task), "scenario", sc.Name, "runs_so_far", job.run-1)
		}
		return d.measureRun(ctx, d.lanes[worker], sc, stamps[sc.Name], job.run)
	}
	fold := func(job runJob, pending pendingRun) (RunResult, error) {
		sc := scenarios[job.caseIndex]
		return d.foldRun(ctx, sc, stamps[sc.Name], job.run, pending)
	}
	var sets []ScenarioRuns
	var err error
	if len(d.lanes) == 1 {
		sets, err = runRounds(seeds, repeats, d.maxRuns, func(i, run int) (RunResult, error) {
			job := runJob{caseIndex: i, run: run}
			pending, err := measure(ctx, 0, job)
			if err != nil {
				return RunResult{}, err
			}
			return fold(job, pending)
		})
	} else {
		sets, err = roundsOf(seeds, repeats, d.maxRuns, func(jobs []runJob) ([]RunResult, error) {
			return scoreConcurrently(ctx, jobs, len(d.lanes), measure, fold)
		})
	}
	if err != nil {
		return nil, err
	}
	for i, sc := range scenarios {
		d.acc.scenarios = append(d.acc.scenarios, scenarioRow(sc, stamps[sc.Name], sets[i]))
	}
	return sets, nil
}

// measureRun yields one run of one scenario, replayed from the journal when it
// can be and paid for on lane when it cannot.
func (d taskDriver) measureRun(ctx context.Context, lane runLane, sc Scenario, stamp string, run int) (pendingRun, error) {
	pending := pendingRun{}
	pending.measured.outcome, pending.replayed = d.journal.replay(ctx, d.census, sc, stamp, run, d.log)
	if pending.replayed {
		d.log.InfoContext(ctx, "aicert: replaying a journaled run — not paying for it again",
			"task", string(d.task), "scenario", sc.Name, "run", run)
	} else {
		measured, runErr := driveRun(ctx, lane, sc, d.task, d.census, d.log, d.trace, d.journal, run)
		if runErr != nil {
			return pendingRun{}, fmt.Errorf("aicert: task %s scenario %s run %d: %w", d.task, sc.Name, run, runErr)
		}
		pending.measured = measured
	}
	// Applied to a replayed run too, though only a run that already passed
	// it is ever journaled: one gate over both paths is one answer to
	// "may this run be certified", rather than two that can drift apart.
	if err := degradeGate(d.task, sc, run, pending.measured.outcome); err != nil {
		return pendingRun{}, err
	}
	return pending, nil
}

// foldRun adds one measured run to the task's accumulation, then journals it.
func (d taskDriver) foldRun(ctx context.Context, sc Scenario, stamp string, run int, pending pendingRun) (RunResult, error) {
	// Journaled only once the accumulation ACCEPTS it, never before. addRun
	// enforces served-identity uniformity across the whole set, which is a
	// property of the set and not of this run: journaling first would store a
	// run that was then rejected, and every restart inside the window would
	// replay it and fail the task again — a transient provider drift made
	// sticky for six hours, escapable only by throwing away the whole
	// journal with RESUME=.
	if err := d.acc.addRun(d.task, sc, run-1, pending.measured.outcome); err != nil {
		return RunResult{}, err
	}
	if !pending.replayed {
		d.journal.append(ctx, sc, stamp, run, pending.measured, nowFunc(), d.log)
	}
	return pending.measured.outcome.RunResult, nil
}

// certifyHooks is the injection seam for certifyTask's two router
// constructions and the per-run payload trace both routers feed. The
// candidate/judge LocalOption lists let this package's own tests reach in —
// a scripted *ai.FakeClient via ai.WithFakeClient, a starved
// ai.WithMonthlyBudget to force a deterministic degrade — none of which
// RunnerConfig's pinned shape has room for. trace is the one field a real
// run sets: Run passes &certifyHooks{trace: t} (t nil unless
// MARGINCE_AICERT_TRACE named a directory), the tests leave it nil. This
// mirrors ai.assembleRouter: "the seam unit tests inject fakes through."
type certifyHooks struct {
	candidateOpts []ai.LocalOption
	judgeOpts     []ai.LocalOption
	// decisionOpts reach the decision leg's router alone — a scripted
	// decision model via ai.WithFakeDecider.
	decisionOpts []ai.LocalOption
	trace        *payloadTrace
	// journal is this task's view of the resume journal. Its zero value is the
	// disabled one, so a caller that resumes nothing leaves it alone.
	journal taskJournal
	// maxRuns, when set, caps each case's adaptive runs below adaptiveMaxRuns:
	// a test pinning an exact run count sets it to the repeats it asks for.
	maxRuns int
	// parallel is how many runs are in flight at once; zero is one.
	parallel int
}

// caseSeeds is each scenario's run set before its first run: what the verdict
// rule and the adaptive rounds read of the case beyond its runs.
func caseSeeds(scenarios []Scenario) []ScenarioRuns {
	seeds := make([]ScenarioRuns, len(scenarios))
	for i, sc := range scenarios {
		seeds[i] = ScenarioRuns{Bands: sc.Expect.Bands, Mechanical: !sc.Expect.Judged(), Privacy: sc.Expect.Privacy}
	}
	return seeds
}
