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

// taskDriver is what every run of one task shares: the two routers and their
// recorders, the accumulation the record is built from, and the trace and
// journal each run is filed in.
type taskDriver struct {
	task            ai.Task
	census          *aitasks.Registry
	candidateRouter *ai.Router
	candidateRec    *traceRecorder
	judgeRouter     *ai.Router
	judgeRec        *traceRecorder
	log             *slog.Logger
	acc             *taskAccumulation
	trace           *payloadTrace
	journal         taskJournal
	// maxRuns is the most runs any case reaches: adaptiveMaxRuns in production.
	maxRuns int
}

// runScenarios drives every scenario of the task in adaptive rounds (runRounds)
// and returns their run sets for certifyTask to judge together. The per-run
// degrade gates sit in scoreRun rather than inside certifyTask because they void
// the WHOLE task: a demoted answer or a demoted grader anywhere in the set means
// no record, not a lower band.
func (d taskDriver) runScenarios(ctx context.Context, scenarios []Scenario, stamps map[string]string, repeats int) ([]ScenarioRuns, error) {
	bands := make([]Bands, len(scenarios))
	for i, sc := range scenarios {
		bands[i] = sc.Expect.Bands
	}
	sets, err := runRounds(bands, repeats, d.maxRuns, func(i, run int) (RunResult, error) {
		if run > repeats && (run-repeats-1)%adaptiveRound == 0 {
			d.log.InfoContext(ctx, "aicert: extending a borderline case by another round",
				"task", string(d.task), "scenario", scenarios[i].Name, "runs_so_far", run-1)
		}
		return d.scoreRun(ctx, scenarios[i], stamps[scenarios[i].Name], run)
	})
	if err != nil {
		return nil, err
	}
	for i, sc := range scenarios {
		d.acc.scenarios = append(d.acc.scenarios, scenarioRow(sc, stamps[sc.Name], sets[i]))
	}
	return sets, nil
}

// scoreRun yields one run of one scenario, from the journal when it holds it
// and paid for when it does not, folded into the task's accumulation.
func (d taskDriver) scoreRun(ctx context.Context, sc Scenario, stamp string, run int) (RunResult, error) {
	outcome, replayed := d.journal.lookup(sc, stamp, run)
	if replayed {
		d.log.InfoContext(ctx, "aicert: replaying a journaled run — not paying for it again",
			"task", string(d.task), "scenario", sc.Name, "run", run)
	} else {
		var runErr error
		outcome, runErr = driveRun(ctx, d.candidateRouter, d.candidateRec, d.judgeRouter, d.judgeRec, sc, d.task, d.census, d.log, d.trace, d.journal, run)
		if runErr != nil {
			return RunResult{}, fmt.Errorf("aicert: task %s scenario %s run %d: %w", d.task, sc.Name, run, runErr)
		}
	}
	// Applied to a replayed run too, though only a run that already passed
	// it is ever journaled: one gate over both paths is one answer to
	// "may this run be certified", rather than two that can drift apart.
	if err := degradeGate(d.task, sc, run, outcome); err != nil {
		return RunResult{}, err
	}
	// Journaled only once the accumulation ACCEPTS it, never before. addRun
	// enforces served-identity uniformity across the whole set, which is a
	// property of the set and not of this run: journaling first would store a
	// run that was then rejected, and every restart inside the window would
	// replay it and fail the task again — a transient provider drift made
	// sticky for six hours, escapable only by throwing away the whole
	// journal with RESUME=.
	if err := d.acc.addRun(d.task, sc, run-1, outcome); err != nil {
		return RunResult{}, err
	}
	if !replayed {
		d.journal.append(ctx, sc, stamp, run, outcome, nowFunc(), d.log)
	}
	return outcome.RunResult, nil
}
