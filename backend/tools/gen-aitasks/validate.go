// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// What the contract must say before anything is generated from it.
//
// Its own file because the refusals are the product here: each one names a
// declaration that would otherwise compile into a binary and be discovered by
// a person instead — an unknown tier, a mode paired with the wrong budget
// posture, a task nobody can name.

import (
	"fmt"
)

// validate enforces the contract's own invariants: every tier a ladder
// or degrade_to entry names must be declared in tiers, every task name
// is a valid Go-identifier source, and on_budget_exhausted is one of the
// two policies the runtime understands. Execution mode and exhaustion policy
// are a closed pair: interactive tasks degrade, background tasks queue.
// Status and sites are a closed pair too: a shipped task owes at least one
// uniquely named site of a known kind, a planned task owes none. A declared
// company-context policy must be internally coherent.
func (c contract) validate() error {
	if len(c.Tiers) == 0 {
		return fmt.Errorf("contract declares no tiers")
	}
	tierSet := make(map[string]bool, len(c.Tiers))
	for _, t := range c.Tiers {
		tierSet[t] = true
	}
	if len(c.Tasks) == 0 {
		return fmt.Errorf("contract declares no tasks")
	}
	for name, def := range c.Tasks {
		if err := validateTask(name, def, tierSet); err != nil {
			return err
		}
	}
	for from, to := range c.DegradeTo {
		if !tierSet[from] {
			return fmt.Errorf("degrade_to: unknown tier %q", from)
		}
		if !tierSet[to] {
			return fmt.Errorf("degrade_to: tier %q degrades to unknown tier %q", from, to)
		}
	}
	return nil
}

// validateTask refuses one task's declaration.
//
// Split from validate for the length ceiling, and it reads better for it:
// the outer function is about the contract as a whole — its tiers, its
// degrade ladder — and this one is entirely about a single task's fields.
func validateTask(name string, def taskDef, tierSet map[string]bool) error {
	if !taskNameRE.MatchString(name) {
		return fmt.Errorf("task %q: name must match %s", name, taskNameRE.String())
	}
	if err := checkDisplayName(name, def.DisplayName); err != nil {
		return err
	}
	if len(def.Ladder) == 0 {
		return fmt.Errorf("task %q: ladder is empty", name)
	}
	for _, tier := range def.Ladder {
		if !tierSet[tier] {
			return fmt.Errorf("task %q: ladder names unknown tier %q", name, tier)
		}
	}
	switch def.OnBudgetExhausted {
	case "queue", "degrade":
	default:
		return fmt.Errorf("task %q: on_budget_exhausted must be \"queue\" or \"degrade\", got %q", name, def.OnBudgetExhausted)
	}
	switch def.ExecutionMode {
	case "interactive":
		if def.OnBudgetExhausted != "degrade" {
			return fmt.Errorf("task %q: interactive execution_mode requires on_budget_exhausted \"degrade\"", name)
		}
	case "background":
		if def.OnBudgetExhausted != "queue" {
			return fmt.Errorf("task %q: background execution_mode requires on_budget_exhausted \"queue\"", name)
		}
	default:
		return fmt.Errorf("task %q: execution_mode must be \"interactive\" or \"background\", got %q", name, def.ExecutionMode)
	}
	switch def.Status {
	case statusShipped:
		if len(def.Sites) == 0 {
			return fmt.Errorf("task %q: status shipped requires at least one site — a shipped task with no site cannot be certified, and would present as covered", name)
		}
	case statusPlanned:
		if len(def.Sites) > 0 {
			return fmt.Errorf("task %q: status planned declares %d site(s); a planned task has no implementation", name, len(def.Sites))
		}
	default:
		return fmt.Errorf("task %q: status must be %q or %q, got %q", name, statusShipped, statusPlanned, def.Status)
	}
	seenSite := make(map[string]bool, len(def.Sites))
	for _, s := range def.Sites {
		if !siteNameRE.MatchString(s.Name) {
			return fmt.Errorf("task %q: site name %q must match %s", name, s.Name, siteNameRE.String())
		}
		if seenSite[s.Name] {
			return fmt.Errorf("task %q: site %q is declared twice", name, s.Name)
		}
		seenSite[s.Name] = true
		if !siteKinds[s.Kind] {
			return fmt.Errorf("task %q: site %q has unknown kind %q", name, s.Name, s.Kind)
		}
	}
	if err := validateAgents(name, def); err != nil {
		return err
	}
	if err := def.CompanyContext.validate(name); err != nil {
		return err
	}
	return nil
}
