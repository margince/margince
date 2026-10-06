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
	for task, want := range map[string]string{
		"foo": "| `foo` | Test task foo | background | `alpha` → `beta` | waits for the provider's next check; the attempt is not spent | — |",
		"bar": "| `bar` | Test task bar | interactive | `beta` → `alpha` | fails at once with a 503 naming the cause | — |",
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
		if task, _, ok := strings.Cut(strings.TrimPrefix(line, "| `"), "` |"); ok && strings.HasPrefix(line, "| `") {
			rows[task] = line
		}
	}
	for _, task := range []string{"abe", "zed"} {
		if !strings.HasSuffix(rows[task], "| a failed decision call hands the task to its ladder |") {
			t.Errorf("the outage row for decision task %s does not name its fallback: %q", task, rows[task])
		}
	}
}
