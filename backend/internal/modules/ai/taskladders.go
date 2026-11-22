// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// A routing config may reorder one task's ladder: the same rungs the contract
// gives the task, walked in another order. A preset uses it to put a task on a
// stronger rung first without rebinding a tier every other task also reads.

import (
	"fmt"
	"maps"
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
