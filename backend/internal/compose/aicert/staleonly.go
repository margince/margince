// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// STALE_ONLY: a run pays only for what is missing or stale. A record current for
// this build describes exactly the requests the run would send, so measuring it
// again buys a second sample, which is a variance question, not a certification.

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
)

// recordMeasures reports whether rec grades binding as a rung of task under
// profile: the same provider, model, profile, thinking level and broker upstream
// preferences, with every site run at the level this binding serves it — a
// record from before a site declared its level measured a different call.
func recordMeasures(rec Record, binding ai.ProviderConfig, profile ai.Profile, task ai.Task) bool {
	return rec.Kind != KindDecision &&
		rec.Task == string(task) &&
		rec.Provider == binding.Provider &&
		rec.ServedModel == binding.Model &&
		rec.EnvClass == string(profile) &&
		rec.ThinkingLevel == binding.ThinkingLevel &&
		maps.Equal(rec.SiteThinking, ai.SiteThinkingLevels(binding, task)) &&
		reflect.DeepEqual(rec.CandidateUpstream, ai.UpstreamPreferencesFor(binding))
}

// currentBindings marks, per task and binding, the candidates whose committed
// record is current for this build on every site the task's scenarios run: the
// same judgement `make e2e-ai-report` prints, so a skip and a report never disagree.
func currentBindings(ctx context.Context, cfg RunnerConfig, byTask map[ai.Task][]Scenario, log *slog.Logger) (map[string]bool, error) {
	records, err := LoadRecords(cfg.RecordDir)
	if err != nil {
		return nil, fmt.Errorf("reading records for STALE_ONLY: %w", err)
	}
	current := map[string]bool{}
	for task, scenarios := range byTask {
		rows, err := readinessOf(ctx, cfg.Census, task, scenarios, records)
		if err != nil {
			// Left to be measured: certifyTask reports the same failure for this
			// task alone, where one broken scenario costs one record.
			log.WarnContext(ctx, "aicert: STALE_ONLY cannot judge this task, so it is measured", "task", string(task), "err", err)
			continue
		}
		cands, _, err := taskCandidates(cfg, task)
		if err != nil {
			continue // certifyAndWrite reports it, per task
		}
		decisions, err := decisionsCurrent(cfg, task, scenarios, records)
		if err != nil {
			log.WarnContext(ctx, "aicert: STALE_ONLY cannot judge this task's decision records, so it is measured", "task", string(task), "err", err)
			decisions = false
		}
		for _, c := range cands {
			// The answering rung carries the decision leg, so it is current only
			// with its decision records.
			if c.Rung == 0 && !decisions {
				continue
			}
			if rungState(rows, scenarios, c.Binding, cfg.recordProfile(), task) == StatusCurrent {
				current[candidateKey(task, c.Binding)] = true
				log.InfoContext(ctx, "aicert: skipped — record current; STALE_ONLY=0 to re-measure",
					"task", string(task), "model", c.Binding.Model)
			}
		}
	}
	return current, nil
}

// readinessOf is task's rows of the readiness report over records.
func readinessOf(ctx context.Context, census *aitasks.Registry, task ai.Task, scenarios []Scenario, records []Record) ([]ReadinessRow, error) {
	taskStamps, perSite, err := CurrentStamps(ctx, scenarios, census)
	if err != nil {
		return nil, fmt.Errorf("task %s: stamping its scenarios: %w", task, err)
	}
	var sites []aitasks.Site
	for _, site := range census.All() {
		if site.Task == task {
			sites = append(sites, site)
		}
	}
	rows, _ := Readiness(Census{Sites: sites, Scopes: census.Scopes()}, taskStamps, perSite, records)
	return rows, nil
}

// decisionsCurrent reports whether every decision site task's scenarios reach
// has a current record from the run's decisions lane; true when none is bound.
func decisionsCurrent(cfg RunnerConfig, task ai.Task, scenarios []Scenario, records []Record) (bool, error) {
	lane := cfg.decisionLane()
	if lane == nil {
		return true, nil
	}
	stamps, err := CurrentDecisionStamps(scenarios, cfg.Census)
	if err != nil {
		return false, fmt.Errorf("task %s: stamping its decision scenarios: %w", task, err)
	}
	rows := DecisionReadiness(stamps, records)
	for site := range stamps {
		if !strings.HasPrefix(site, string(task)+"/") {
			continue
		}
		if !slices.ContainsFunc(rows, func(row DecisionRow) bool {
			return row.Site == site && row.Record.Provider == lane.Provider && row.Record.Model == lane.Model &&
				row.Record.EnvClass == string(cfg.recordProfile()) && row.Status() == StatusCurrent
		}) {
			return false, nil
		}
	}
	return true, nil
}

func scenarioSites(scenarios []Scenario) []string {
	seen := map[string]bool{}
	var sites []string
	for _, sc := range scenarios {
		if !seen[sc.Site] {
			seen[sc.Site] = true
			sites = append(sites, sc.Site)
		}
	}
	return sites
}

func candidateKey(task ai.Task, binding ai.ProviderConfig) string {
	return string(task) + "\x00" + binding.Provider + "\x00" + binding.Model
}
