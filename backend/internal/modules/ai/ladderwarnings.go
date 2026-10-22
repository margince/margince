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
// that all reach one API, grouped by that API so a preset that puts every tier
// on one model costs one finding, not one per task.
//
// The rungs are the ones a call walks (boundPlan: sovereign remapping applied,
// providers resolved), and "one API" is sameRoute, the test the router uses to
// stop retrying a rejected request at the same place. Upstream pins and the
// thinking level are not part of it: they change how the request is served or
// asked, not which API answers.
//
// A ladder with a single bound rung is not reported. Several tasks bind one
// tier by design (document_extract and corpus_ask, for example).
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
// onto. It is a warning, not a refusal, because one model on several tiers is
// legal and sometimes deliberate.
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

// LogLadderWarnings is what a boot reports about cfg's ladders: tasks with no
// bound rung, which a call refuses outright, and tasks whose rungs are one API.
func LogLadderWarnings(log *slog.Logger, cfg RoutingConfig) {
	for _, w := range cfg.UnboundLadderWarnings() {
		log.Warn(w)
	}
	LogSingleRungLadders(log, cfg)
}
