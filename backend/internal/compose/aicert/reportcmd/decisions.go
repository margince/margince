// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// The decision table the report prints after the site table: every committed
// decision record, whether it still describes the decision question this build
// asks, and whether it earns the row the runtime serves the decisions lane by.

import (
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/margince/margince/backend/internal/compose/aicert"
)

var decisionColumns = []string{
	"SITE", "STATUS", "VERDICT", "SERVES", "PROVIDER", "MODEL", "ENV",
	"RUNS", "KEPT", "KEPT_WRONG", "FALLBACK_RATE", "FALLBACKS", "SERVED_PASS_RATE",
}

// renderDecisions reports the decision records. With none it says the lane
// serves nothing, which is the state until a decision certification is run.
func renderDecisions(rows []aicert.DecisionRow) string {
	if len(rows) == 0 {
		return "\nDecision models: no decision record is committed, so the decisions lane serves no site.\n"
	}
	var buf strings.Builder
	buf.WriteString("\nDecision models: the lane answers a site only where SERVES is yes (make gen writes those rows).\n\n")
	w := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	lines := []string{strings.Join(decisionColumns, "\t")}
	var reasons []string
	for _, row := range rows {
		rec := row.Record
		lines = append(lines, strings.Join(decisionCells(row), "\t"))
		if reason := row.Standing.Reason(); reason != "" {
			reasons = append(reasons, "  - "+row.Site+" on "+rec.Provider+" · "+rec.Model+" · "+rec.EnvClass+": "+reason)
		}
	}
	for _, line := range lines {
		if _, err := fmt.Fprintln(w, line); err != nil {
			return fmt.Sprintf("aicert: formatting the decision report: %v\n", err)
		}
	}
	if err := w.Flush(); err != nil {
		return fmt.Sprintf("aicert: formatting the decision report: %v\n", err)
	}
	for _, reason := range reasons {
		buf.WriteString(reason + "\n")
	}
	return buf.String()
}

func decisionCells(row aicert.DecisionRow) []string {
	rec := row.Record
	servesCell := "no"
	if row.Serves() {
		servesCell = "yes"
	}
	cells := []string{row.Site, row.Status(), rec.Verdict, servesCell, rec.Provider, rec.Model, rec.EnvClass, fmt.Sprintf("%d", rec.Runs)}
	stats := rec.Decision
	if stats == nil {
		for len(cells) < len(decisionColumns) {
			cells = append(cells, aicert.Unmeasured)
		}
		return cells
	}
	return append(cells, fmt.Sprintf("%d", stats.Kept), fmt.Sprintf("%d", stats.KeptWrong),
		fmt.Sprintf("%.2f", stats.FallbackRate), fallbacksCell(stats.FallbackByReason), fmt.Sprintf("%.2f", stats.ServedPassRate))
}

func fallbacksCell(byReason map[string]int) string {
	if len(byReason) == 0 {
		return aicert.Unmeasured
	}
	reasons := make([]string, 0, len(byReason))
	for reason, n := range byReason {
		reasons = append(reasons, fmt.Sprintf("%s=%d", reason, n))
	}
	sort.Strings(reasons)
	return strings.Join(reasons, ",")
}
