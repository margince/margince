// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// Task selection and binding: which scenarios a run certifies (corpus filter +
// deterministic order), and how the run's MODEL= binding becomes the config the
// candidate router serves — separately from the judge's, which is a different
// model on purpose.

import (
	"fmt"
	"slices"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/config"
)

// ladderForTask binds every tier in a task's ladder to one binding. role names
// which side it is — "candidate" or "judge" — for the error only.
//
// This replaced an override applied on top of a routing FILE's bindings. With
// the file gone there is no "on top of": the binding a run is given IS what it
// certifies, for every tier the task can fall through, so a record names one
// model rather than whichever rung a file happened to put underneath.
//
// The BaseURL rides along unchanged on every tier. openai_compatible fails
// closed without one, and a ladder that dropped it on the second rung would
// certify the first rung and then fail in a way that reads like the model
// refusing rather than the binding being incomplete.
func ladderForTask(role string, binding ai.ProviderConfig, profile ai.Profile, task ai.Task) (ai.RoutingConfig, error) {
	ladder := ai.TaskLadder(task)
	if len(ladder) == 0 {
		return ai.RoutingConfig{}, fmt.Errorf("aicert: task %s has no routing ladder to bind", task)
	}
	// EVERY tier, not only the task's ladder. The router demotes under budget
	// pressure and the demote target need not be a rung the ladder names — an
	// unbound one surfaces as "no bound tier can serve", which reads like the
	// task being unsupported rather than the binding being partial.
	tiers := make(map[ai.Tier]ai.ProviderConfig, len(ai.AllTiers()))
	for _, tier := range ai.AllTiers() {
		tiers[tier] = binding
	}
	// Validated rather than assembled and run. The profile is a CLAIM about
	// where inference may happen — sovereign forbids a cloud vendor — and the
	// binding arrives from an environment variable, so it has to meet the same
	// rule a parsed config does. Without this a sovereign run would build the
	// client and call the vendor's API, and the record would describe a
	// deployment nobody has.
	//
	// Per TIER rather than the whole config: this lane certifies chat tasks and
	// binds no embeddings lane, which RoutingConfig.Validate requires and would
	// refuse. Binding one to the candidate to satisfy it would put a chat model
	// in the embed lane of every record.
	if err := ai.ValidateTierBinding(profile, ladder[0], binding); err != nil {
		// Named by ROLE, because both the candidate and the judge are validated
		// through here and they are set by different variables: a message that
		// always said MARGINCE_AICERT_MODEL sent a reader to fix the candidate
		// when it was the judge's binding that was incomplete.
		return ai.RoutingConfig{}, fmt.Errorf(
			"aicert: the %s binding %s:%s under profile %s: %w",
			role, binding.Provider, binding.Model, profile, err,
		)
	}
	doc := ladderDocument(binding, profile, tiers)
	// Finalized as a stored document is, so the lanes' hosts and pins resolve
	// from the providers block exactly as they do in production. The keys come
	// from the environment: a binding names providers and never credentials.
	cfg, err := ai.FromStored(doc, config.FromOS)
	if err != nil {
		return ai.RoutingConfig{}, fmt.Errorf("aicert: the %s binding %s:%s under profile %s: %w",
			role, binding.Provider, binding.Model, profile, err)
	}
	return cfg, nil
}

// ladderDocument is the routing a run certifies, written the way the shipped
// presets are: the host and pins sit once in the providers block, and no lane
// repeats them.
//
// The embed lane is bound to the same model because the router requires one,
// not because certification embeds anything. It carries the provider and model
// only: a chat tier's input, thinking level and sort are refused on that lane.
func ladderDocument(binding ai.ProviderConfig, profile ai.Profile, tiers map[ai.Tier]ai.ProviderConfig) ai.RoutingConfig {
	embeddings := ai.ProviderConfig{Provider: binding.Provider, Model: binding.Model, Location: binding.Location}
	return ai.RoutingConfig{
		Profile:    profile,
		Tiers:      tiers,
		Embeddings: ai.EmbeddingsConfig{ProviderConfig: embeddings},
	}.Canonical()
}

// groupByTask buckets scenarios by their Task field, keeping only tasks
// matching filter when filter is non-empty.
func groupByTask(scenarios []Scenario, filter string) map[ai.Task][]Scenario {
	byTask := map[ai.Task][]Scenario{}
	for _, sc := range scenarios {
		if filter != "" && sc.Task != filter {
			continue
		}
		t := ai.Task(sc.Task)
		byTask[t] = append(byTask[t], sc)
	}
	return byTask
}

// sortedTasks returns byTask's keys in deterministic order, so two runs
// over the same corpus process tasks (and therefore emit any errors) in
// the same order.
func sortedTasks(byTask map[ai.Task][]Scenario) []ai.Task {
	tasks := make([]ai.Task, 0, len(byTask))
	for t := range byTask {
		tasks = append(tasks, t)
	}
	slices.Sort(tasks)
	return tasks
}
