// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"log/slog"
	"strings"
)

// SingleRungLadder is one API that every bound rung of the listed tasks'
// ladders reaches: a fallback in name only.
type SingleRungLadder struct {
	Provider, Model string
	Tasks           []Task
}

// SingleRungLadders names the tasks whose ladder has two or more bound rungs
// that all reach one API. They are grouped by API, so a preset with one model on
// every tier costs one finding.
//
// The rungs are the ones a call walks, with sovereign remapping applied. "One
// API" is sameRoute, so upstream pins and thinking level are not compared.
// A ladder with one bound rung draws no finding.
func (cfg RoutingConfig) SingleRungLadders() []SingleRungLadder {
	var found []SingleRungLadder
	index := map[routeMeta]int{}
	for _, task := range AllTasks() {
		plan, blocked := boundPlan(cfg, task, "")
		if blocked || len(plan) < 2 {
			continue
		}
		first := metaOf(plan[0].config)
		collapsed := true
		for _, rung := range plan[1:] {
			collapsed = collapsed && sameRoute(first, metaOf(rung.config))
		}
		if !collapsed {
			continue
		}
		at, seen := index[first]
		if !seen {
			at = len(found)
			index[first] = at
			found = append(found, SingleRungLadder{Provider: first.provider, Model: first.model})
		}
		found[at].Tasks = append(found[at].Tasks, task)
	}
	return found
}

func metaOf(binding ProviderConfig) routeMeta {
	return routeMeta{provider: binding.Provider, model: binding.Model, baseURL: binding.BaseURL}
}

// LogSingleRungLadders writes one warning per API that cfg's ladders collapse
// onto. It warns instead of refusing, because one model on several tiers is
// legal.
func LogSingleRungLadders(log *slog.Logger, cfg RoutingConfig) {
	for _, ladder := range cfg.SingleRungLadders() {
		tasks := make([]string, len(ladder.Tasks))
		for i, task := range ladder.Tasks {
			tasks[i] = string(task)
		}
		log.Warn("ai: routing: every bound rung of these tasks' ladders reaches one API, so a refusal there has no fallback; "+
			"bind a different model on a higher tier",
			"provider", ladder.Provider, "model", ladder.Model, "tasks", strings.Join(tasks, ","))
	}
}

// LogLadderWarnings is what a boot reports about cfg's ladders. Tasks with no
// bound rung are refused outright. Tasks whose rungs reach one API have no fallback.
func LogLadderWarnings(log *slog.Logger, cfg RoutingConfig) {
	for _, w := range cfg.UnboundLadderWarnings() {
		log.Warn(w)
	}
	LogSingleRungLadders(log, cfg)
}
