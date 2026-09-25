// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// The decision leg: when a routed run's config binds a decisions lane, every
// scenario whose case has a decision form is also asked of that lane, and each
// site gets a decision record beside the task's LLM record.
//
// It runs on a router of its own, built from the candidate's config plus the
// lane, so the LLM leg's router, calls and record are exactly what a run
// without a lane produces. No judge: a decision answer is a closed label, and
// the site's own gate plus the fixture's expected label grade it.

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
)

// decisionLane is the lane a run certifies decisions on: a routed run's own
// `decisions:` block. A MODEL= run names one chat model and no lane.
func (c RunnerConfig) decisionLane() *ai.DecisionsConfig {
	if c.Routing == nil {
		return nil
	}
	return c.Routing.Decisions
}

// refuseInvalidDecisionLane holds the lane to the rule a parsed config meets,
// under the profile its records are filed under, before a paid call.
func refuseInvalidDecisionLane(cfg RunnerConfig) error {
	lane := cfg.decisionLane()
	if lane == nil {
		return nil
	}
	if err := ai.ValidateDecisionsLane(cfg.recordProfile(), *lane); err != nil {
		return fmt.Errorf("the routing's decisions lane cannot be certified: %w", err)
	}
	return nil
}

// decisionLeg is one task's decision certification: the scenarios, the chat
// binding its router is built on, the lane, and the LLM record whose pass rate
// the fallen-back runs are credited with.
type decisionLeg struct {
	task      ai.Task
	scenarios []Scenario
	census    *aitasks.Registry
	candidate ai.ProviderConfig
	lane      ai.DecisionsConfig
	profile   ai.Profile
	repeats   int
	llm       Record
	hooks     *certifyHooks
}

// decisionRouter is the router the leg asks: the candidate's config with the
// lane bound, and every site treated as certified — the table a production
// router reads is built from the records this leg writes.
func (leg decisionLeg) decisionRouter() (*ai.Router, *traceRecorder, error) {
	cfg, err := ladderForTask("candidate (the rung MARGINCE_AICERT_ROUTING resolved)", leg.candidate,
		candidateRole.profileFor(leg.profile), leg.task)
	if err != nil {
		return nil, nil, err
	}
	lane := leg.lane
	cfg.Decisions = &lane
	rec := newTraceRecorder()
	opts := []ai.LocalOption{ai.WithoutResultCache(), ai.WithCallStore(rec), ai.WithEveryDecisionCertified()}
	if leg.hooks != nil {
		opts = append(opts, leg.hooks.decisionOpts...)
		if leg.hooks.trace != nil {
			opts = append(opts, ai.WithPayloadCapture())
		}
	}
	router, err := compose.NewLocalRouterForCert(cfg, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("aicert: task %s: decision router: %w", leg.task, err)
	}
	return router, rec, nil
}

// decisionRun is one decision run: what the probe said, the call it made when
// it made one, and how the fixture grades a kept answer.
type decisionRun struct {
	probe   ai.DecisionProbe
	calls   []ai.Call
	outcome aitasks.Outcome
}

// certify asks every decision scenario repeats times and folds each site's
// runs into its own record. A task with no decision form has none.
func (leg decisionLeg) certify(ctx context.Context, log *slog.Logger) ([]Record, error) {
	stamps, err := DecisionScenarioStamps(leg.scenarios, leg.census)
	if err != nil || len(stamps) == 0 {
		return nil, err
	}
	router, rec, err := leg.decisionRouter()
	if err != nil {
		return nil, err
	}
	bySite := map[string][]scenarioDecisionRuns{}
	var sites []string
	for _, sc := range leg.scenarios {
		if _, stamped := stamps[sc.Name]; !stamped {
			continue
		}
		runs, err := leg.askScenario(ctx, router, rec, sc, log)
		if err != nil {
			return nil, fmt.Errorf("aicert: task %s scenario %s: decision leg: %w", leg.task, sc.Name, err)
		}
		if _, seen := bySite[sc.Site]; !seen {
			sites = append(sites, sc.Site)
		}
		bySite[sc.Site] = append(bySite[sc.Site], scenarioDecisionRuns{scenario: sc, stamp: stamps[sc.Name], runs: runs})
	}
	records := make([]Record, 0, len(sites))
	for _, site := range sites {
		built, err := leg.buildDecisionRecord(site, bySite[site])
		if err != nil {
			return nil, fmt.Errorf("aicert: task %s site %s: decision leg: %w", leg.task, site, err)
		}
		records = append(records, built)
	}
	return records, nil
}

// askScenario asks one scenario's decision question repeats times. A probe
// error — no workspace, a budget the router cannot read, metering that failed
// — is the harness failing, not the lane answering, and voids the leg.
func (leg decisionLeg) askScenario(ctx context.Context, router *ai.Router, rec *traceRecorder, sc Scenario,
	log *slog.Logger,
) ([]decisionRun, error) {
	dc, _, err := decisionCaseFor(sc, leg.census)
	if err != nil {
		return nil, err
	}
	var trace *payloadTrace
	if leg.hooks != nil {
		trace = leg.hooks.trace
	}
	runs := make([]decisionRun, 0, leg.repeats)
	for i := 0; i < leg.repeats; i++ {
		mark := rec.mark()
		probe, err := router.DecideProbe(ctx, leg.task, dc.DecisionSite(), dc.DecisionRequest(), dc.GateDecision)
		if err != nil {
			return nil, fmt.Errorf("run %d: %w", i+1, err)
		}
		calls, err := rec.terminalsSince(mark)
		if err != nil {
			return nil, fmt.Errorf("run %d: %w", i+1, err)
		}
		traceCalls(ctx, trace, "decision", leg.task, sc, i+1, 1, calls, log)
		run := decisionRun{probe: probe, calls: calls}
		if probe.Decided {
			run.outcome = dc.EvaluateDecision(probe.Answer)
		}
		runs = append(runs, run)
	}
	return runs, nil
}

// certifyDecisionsFor runs task's decision leg when the run binds a lane, and
// writes each site's record beside the LLM record llm. The LLM record is
// already written: a decision leg that fails costs its own records only.
func certifyDecisionsFor(ctx context.Context, cfg RunnerConfig, task ai.Task, scenarios []Scenario, candidate ai.ProviderConfig,
	llm Record, repeats int, hooks *certifyHooks, log *slog.Logger,
) ([]Record, error) {
	lane := cfg.decisionLane()
	if lane == nil {
		return nil, nil
	}
	leg := decisionLeg{
		task: task, scenarios: scenarios, census: cfg.Census, candidate: candidate, lane: *lane,
		profile: cfg.recordProfile(), repeats: repeats, llm: llm, hooks: hooks,
	}
	records, err := leg.certify(ctx, log)
	if err != nil {
		return nil, err
	}
	for _, rec := range records {
		if err := WriteRecord(cfg.RecordDir, rec); err != nil {
			return nil, fmt.Errorf("task %s: writing the decision record for site %s: %w", task, rec.Site, err)
		}
		log.InfoContext(ctx, "aicert: decision record", "task", string(task), "site", rec.Site,
			"verdict", rec.Verdict, "kept_wrong", rec.Decision.KeptWrong, "fallback_rate", rec.Decision.FallbackRate)
	}
	return records, nil
}
