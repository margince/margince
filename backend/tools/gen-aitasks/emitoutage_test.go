// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"strings"
	"testing"
)

// A background task waits for its provider and an interactive one fails fast:
// the row follows the task's declared posture, so a task moved between the two
// changes its outage row with it.
func TestTheOutagePageFollowsEachTasksPosture(t *testing.T) {
	c, err := parseContract([]byte(minimalContract))
	if err != nil {
		t.Fatalf("parseContract: %v", err)
	}
	page := string(emitOutageDoc(c))
	shipped := strings.Replace(minimalContract, "status: planned", "status: shipped, sites: [ask]", 1)
	if c, err = parseContract([]byte(shipped)); err != nil {
		t.Fatalf("parseContract: %v", err)
	}
	if want := "| `bar` | Test task bar | interactive | `beta` → `alpha` | fails at once with a 503 naming the cause | — |"; !strings.Contains(string(emitOutageDoc(c)), want) {
		t.Errorf("a shipped interactive task's outage row is not %q", want)
	}
	for task, want := range map[string]string{
		"foo": "| `foo` | Test task foo | background | `alpha` → `beta` | waits for the provider's next check; the attempt is not spent | — |",
		"bar": "| `bar` | Test task bar | interactive | `beta` → `alpha` | not in use yet | — |",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the outage row for %s is not %q:\n%s", task, want, page)
		}
	}
}

// A decision task says its decision model is never waited on.
func TestTheOutagePageSaysADecisionTaskFallsToItsLadder(t *testing.T) {
	c, err := parseContract([]byte(decisionContract))
	if err != nil {
		t.Fatalf("parseContract: %v", err)
	}
	rows := map[string]string{}
	for _, line := range strings.Split(string(emitOutageDoc(c)), "\n") {
		if rest, ok := strings.CutPrefix(line, "| `"); ok {
			task, _, _ := strings.Cut(rest, "`")
			rows[task] = line
		}
	}
	const fallback = "| a failed decision call hands the task to its ladder |"
	for _, task := range []string{"abe", "zed"} {
		if !strings.HasSuffix(rows[task], fallback) {
			t.Errorf("the outage row for decision task %s does not name its fallback: %q", task, rows[task])
		}
	}
	if !strings.HasSuffix(rows["foo"], "| — |") {
		t.Errorf("task foo declares no decision form, yet its outage row names one: %q", rows["foo"])
	}
}

// An interactive task that answers from its own facts when no model can is not
// one that fails: its row says what it does, and a task that does not declare
// it keeps the failure sentence.
func TestTheOutagePageSaysATaskThatDegradesDegrades(t *testing.T) {
	declared := strings.Replace(minimalContract,
		"execution_mode: interactive, on_budget_exhausted: degrade, status: planned",
		"execution_mode: interactive, on_budget_exhausted: degrade, on_outage: degrades, status: shipped, sites: [ask]", 1)
	c, err := parseContract([]byte(declared))
	if err != nil {
		t.Fatalf("parseContract: %v", err)
	}
	page := string(emitOutageDoc(c))
	if strings.Contains(page, "| `bar` | Test task bar | interactive | `beta` → `alpha` | fails at once") {
		t.Errorf("a task declared to degrade still reads as failing:\n%s", page)
	}
	if !strings.Contains(page, "| `bar` | Test task bar | interactive | `beta` → `alpha` | answers from its own facts instead of failing |") {
		t.Errorf("the degrading task's row does not say what it does:\n%s", page)
	}
}

// Only a task that answers while a request is waiting has anything to degrade
// to; a background task queues. And the value is closed.
func TestOnlyAnInteractiveTaskMayDeclareItDegrades(t *testing.T) {
	for name, c := range map[string]struct{ from, to, want string }{
		"a background task":  {"execution_mode: background", "execution_mode: background, on_outage: degrades", "on_outage"},
		"an unknown posture": {"execution_mode: background", "execution_mode: background, on_outage: degrade", "on_outage"},
	} {
		t.Run(name, func(t *testing.T) {
			contract, err := parseContract([]byte(strings.Replace(minimalContract, c.from, c.to, 1)))
			if err == nil {
				err = contract.validate()
			}
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("answered %v, want a refusal naming %s", err, c.want)
			}
		})
	}
}
