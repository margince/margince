// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// The decision table the report prints after the site table. It lists every
// committed decision record, whether it still describes this build's decision
// question, and whether it earns a certified row. A bound lane answers every
// site with or without one; the row is what an operator trusts the binding on.

import (
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/margince/margince/backend/internal/compose/aicert"
)

var decisionColumns = []string{
	"SITE", "STATUS", "VERDICT", "CERTIFIED_ROW", "PROVIDER", "MODEL", "ENV",
	"RUNS", "KEPT", "KEPT_WRONG", "FALLBACK_RATE", "FALLBACKS", "SERVED_PASS_RATE",
}

// renderDecisions reports the decision records. With none it says no site has
// a measured verdict; a bound lane still answers every site.
func renderDecisions(rows []aicert.DecisionRow) string {
	if len(rows) == 0 {
		return "\nDecision models: no decision record is committed, so no site has a measured verdict; a bound lane answers every site all the same.\n"
	}
	var buf strings.Builder
	buf.WriteString("\nDecision models: a bound lane answers every site; CERTIFIED_ROW yes is a current certified record behind it (make gen writes those rows).\n\n")
	w := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	lines := []string{strings.Join(decisionColumns, "\t")}
	var reasons []string
	for _, row := range rows {
		rec := row.Record
		lines = append(lines, strings.Join(decisionCells(row), "\t"))
		if reason := row.NoRowReason(); reason != "" {
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
	certifiedCell := "no"
	if row.Certified() {
		certifiedCell = "yes"
	}
	cells := []string{row.Site, row.Status(), rec.Verdict, certifiedCell, rec.Provider, rec.Model, rec.EnvClass, fmt.Sprintf("%d", rec.Runs)}
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
