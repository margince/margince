// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// A routing config may reorder one task's ladder: the same rungs the contract
// gives the task, walked in another order. A preset uses it to put a task on a
// stronger rung first without rebinding a tier every other task also reads.

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
)

// Ladder is the rungs task walks under cfg: the config's own order when it
// reorders the task, the contract's otherwise.
func (cfg RoutingConfig) Ladder(task Task) []Tier {
	if order, reordered := cfg.TaskLadders[task]; reordered {
		return slices.Clone(order)
	}
	return TaskLadder(task)
}

// validateTaskLadders refuses an order that names a task the contract does not
// declare, or that is not a reordering of that task's own rungs.
func (cfg RoutingConfig) validateTaskLadders() error {
	for _, task := range slices.Sorted(maps.Keys(cfg.TaskLadders)) {
		contract, known := taskLadders[task]
		if !known {
			return fmt.Errorf("ai: routing config: task_ladders names %q, which is no task", task)
		}
		order := cfg.TaskLadders[task]
		if !slices.Equal(slices.Sorted(slices.Values(order)), slices.Sorted(slices.Values(contract))) {
			return fmt.Errorf("ai: routing config: task_ladders.%s is %v; it must reorder the task's own rungs %v", task, order, contract)
		}
	}
	return nil
}

// retiredTasks are tasks a release removed. A binding written before then, in
// the store or the environment, may still order one, and refusing it would
// leave the installation with no models. Any other unknown name is refused.
var retiredTasks = []Task{"extension_task"}

// RetiredTasks lists them for the activity contract, whose filter keeps a
// retired name while rows written under it are still served.
func RetiredTasks() []Task { return slices.Clone(retiredTasks) }

// withoutRetiredTaskLadders drops the order of a retired task.
func (cfg RoutingConfig) withoutRetiredTaskLadders() RoutingConfig {
	kept := maps.Clone(cfg.TaskLadders)
	maps.DeleteFunc(kept, func(task Task, _ []Tier) bool { return slices.Contains(retiredTasks, task) })
	if len(kept) < len(cfg.TaskLadders) {
		cfg.TaskLadders = kept
	}
	return cfg
}

// WithStockTaskLadders gives a stored binding that reorders no task the task
// order of the committed preset it binds tier for tier. Tier for tier means the
// provider, model and thinking level match on every tier; location may differ. A binding an
// operator chose for themselves answers false and is left as it is.
func (cfg RoutingConfig) WithStockTaskLadders() (RoutingConfig, bool) {
	if len(cfg.TaskLadders) > 0 {
		return cfg, false
	}
	var adopt map[Task][]Tier
	matched := false
	for _, name := range slices.Sorted(maps.Keys(stockPresets)) {
		stock, err := ParsePreset([]byte(stockPresets[name]))
		if err != nil || !sameTierModels(cfg.Tiers, stock.Tiers) {
			continue
		}
		// Two committed presets that bind these tiers and order tasks
		// differently, one of them not at all, leave the install's choice unknown.
		if matched && !reflect.DeepEqual(adopt, stock.TaskLadders) {
			return cfg, false
		}
		adopt, matched = stock.TaskLadders, true
	}
	if len(adopt) == 0 {
		return cfg, false
	}
	cfg.TaskLadders = maps.Clone(adopt)
	return cfg, true
}

func sameTierModels(a, b map[Tier]ProviderConfig) bool {
	return maps.EqualFunc(a, b, func(x, y ProviderConfig) bool {
		return x.Provider == y.Provider && x.Model == y.Model && x.ThinkingLevel == y.ThinkingLevel
	})
}
