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

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
)

// recordMeasures reports whether rec grades binding as a rung of task under
// profile: the same provider, model, profile and thinking level, with every
// site run at the level this binding serves it — a record from before a site
// declared its level measured a different call.
func recordMeasures(rec Record, binding ai.ProviderConfig, profile ai.Profile, task ai.Task) bool {
	return rec.Kind != KindDecision &&
		rec.Task == string(task) &&
		rec.Provider == binding.Provider &&
		rec.ServedModel == binding.Model &&
		rec.EnvClass == string(profile) &&
		rec.ThinkingLevel == binding.ThinkingLevel &&
		maps.Equal(rec.SiteThinking, ai.SiteThinkingLevels(binding, task))
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
			return nil, err
		}
		cands, _, err := taskCandidates(cfg, task)
		if err != nil {
			continue // certifyAndWrite reports it, per task
		}
		for _, c := range cands {
			if everySiteCurrent(rows, scenarios, c.Binding, cfg.recordProfile(), task) {
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
		return nil, fmt.Errorf("task %s: stamping its scenarios for STALE_ONLY: %w", task, err)
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

// everySiteCurrent is true when a record grading binding is current on every
// site task's scenarios run on; a site without its row is not.
func everySiteCurrent(rows []ReadinessRow, scenarios []Scenario, binding ai.ProviderConfig, profile ai.Profile, task ai.Task) bool {
	for _, site := range scenarioSites(scenarios) {
		found := false
		for _, row := range rows {
			if row.Site.Variant == site && row.Certified && recordMeasures(row.Record, binding, profile, task) {
				found = row.Status() == StatusCurrent
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
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
