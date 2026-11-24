// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// The agent_loop live lane's section of the report. It lists every filed verdict
// and whether it still describes this build. The verdicts sit beside the
// certification records and never reach their grades. The tables above do not
// count them.

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/margince/margince/backend/internal/compose/agentloope2e/live"
)

var agentLoopColumns = []string{"SCENARIO", "AGENT", "BINDING", "RESULT", "PASSED/RUNS", "STATE"}

// renderAgentLoop reports the live lane's verdicts, with the stamp parts that
// moved for each stale one.
func renderAgentLoop(lines []live.Line) string {
	if len(lines) == 0 {
		return "\nAgent loop live lane: no verdict is filed yet. `make e2e-agent-loop` writes them; they never change a site's grade.\n"
	}
	var buf strings.Builder
	buf.WriteString("\nAgent loop live lane: end-state verdicts from `make e2e-agent-loop`. A smoke result has too few runs to certify anything, and none of these changes a site's grade.\n\n")
	w := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	rows := []string{strings.Join(agentLoopColumns, "\t")}
	for _, line := range lines {
		rows = append(rows, strings.Join(agentLoopCells(line), "\t"))
	}
	for _, row := range rows {
		if _, err := fmt.Fprintln(w, row); err != nil {
			return fmt.Sprintf("aicert: formatting the agent loop report: %v\n", err)
		}
	}
	if err := w.Flush(); err != nil {
		return fmt.Sprintf("aicert: formatting the agent loop report: %v\n", err)
	}
	return buf.String()
}

func agentLoopCells(line live.Line) []string {
	v := line.Verdict
	result := "fail"
	if v.Outcome.Pass {
		result = "pass"
	}
	if v.Outcome.Smoke {
		result += " (smoke)"
	}
	state := "current"
	if !line.Current() {
		state = "stale: " + strings.Join(line.Moved, ", ")
	}
	return []string{
		v.Scenario, string(v.Agent), fmt.Sprintf("%s:%s (%s)", v.Binding.Provider, v.Binding.Model, v.Binding.Tier),
		result, fmt.Sprintf("%d/%d", v.Outcome.Passed, v.Outcome.Runs), state,
	}
}
