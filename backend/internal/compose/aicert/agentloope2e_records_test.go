// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import (
	"path/filepath"
	"testing"

	"github.com/margince/margince/backend/internal/compose/agentloope2e"
)

// The agent_loop lane files its verdicts beside the certification records.
// They must stay out of them. A verdict read as a record would reach readiness,
// the generated grades and the Settings page as a second answer for the site.
func TestAgentLoopVerdictsAreNotCertificationRecords(t *testing.T) {
	root := filepath.Join(t.TempDir(), "records")
	binding := agentloope2e.Binding{Provider: "gemini", Model: "gemini-3.8-flash", Tier: "premium"}
	runs := []agentloope2e.Run{{Criteria: []agentloope2e.Criterion{{Text: "c", Met: true, Reason: "r"}}}}
	verdict := agentloope2e.Verdict{
		Scenario: "annotates", Agent: agentloope2e.MorningBrief, Binding: binding, Judge: "claude-haiku-5-5",
		Runs: runs, Outcome: agentloope2e.Decide(runs), Stamp: agentloope2e.NewStamp("s", "w", "h", binding, "claude-haiku-5-5"),
		RanAt: "2026-11-23T10:00:00Z",
	}
	if err := verdict.Write(filepath.Join(root, "agent_loop_e2e")); err != nil {
		t.Fatal(err)
	}
	records, err := LoadRecords(root)
	if err != nil || len(records) != 0 {
		t.Errorf("LoadRecords read %d record(s), %v; want none", len(records), err)
	}
}
