// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/agentloope2e"
	"github.com/margince/margince/backend/internal/compose/agentloope2e/live"
)

func TestTheAgentLoopSectionSaysWhenNoVerdictIsFiled(t *testing.T) {
	out := renderAgentLoop(nil)
	if !strings.Contains(out, "no verdict is filed yet") || !strings.Contains(out, "never change a site's grade") {
		t.Fatalf("an empty section reads:\n%s", out)
	}
}

func TestTheAgentLoopSectionNamesResultRunsAndWhatWentStale(t *testing.T) {
	binding := agentloope2e.Binding{Provider: "gemini", Model: "gemini-3.8-flash", Tier: "premium"}
	verdict := agentloope2e.Verdict{
		Scenario: "morning_brief_annotates_its_queue_from_the_evidence", Agent: agentloope2e.MorningBrief, Binding: binding,
		Outcome: agentloope2e.Outcome{Runs: 3, Passed: 3, Pass: true, Smoke: true},
	}
	failed := verdict
	failed.Scenario = "overnight_sweep_logs_one_note_per_slipping_deal"
	failed.Agent = agentloope2e.OvernightSweep
	failed.Outcome = agentloope2e.Outcome{Runs: 12, Passed: 7}
	out := renderAgentLoop([]live.Line{
		{Verdict: verdict},
		{Verdict: failed, Moved: []string{"scenario", "harness"}},
	})
	var current, stale string
	for line := range strings.SplitSeq(out, "\n") {
		switch {
		case strings.HasPrefix(line, verdict.Scenario):
			current = line
		case strings.HasPrefix(line, failed.Scenario):
			stale = line
		}
	}
	for _, want := range []string{"morning_brief", "gemini:gemini-3.8-flash (premium)", "pass (smoke)", "3/3", "current"} {
		if !strings.Contains(current, want) {
			t.Errorf("the current row lacks %q:\n%s", want, current)
		}
	}
	for _, want := range []string{"overnight_at_risk_sweep", "fail", "7/12", "stale: scenario, harness"} {
		if !strings.Contains(stale, want) {
			t.Errorf("the stale row lacks %q:\n%s", want, stale)
		}
	}
	if strings.Contains(stale, "smoke") {
		t.Errorf("a 12-run verdict is no smoke result:\n%s", stale)
	}
	if !strings.HasPrefix(strings.Fields(out)[0], "Agent") {
		t.Errorf("the section opens with %q", strings.Fields(out)[0])
	}
}
