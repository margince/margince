// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// The three questions CI asks of the report instead of printing it. Which
// records did a change make stale, how many are owed, and is every task of one
// preset current? Each reads the same rows the printed table does, so a CI
// answer and a table a person reads cannot disagree.

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/compose/aicert"
)

// Columns of the printed table a key is made of: the site, the status, and the
// binding the record was measured on.
const (
	columnSite     = 0
	columnStatus   = 2
	columnProvider = 5
	columnModel    = 6
	columnEnv      = 7
)

var columnGap = regexp.MustCompile(`\s{2,}`)

// rowKey names one record's row: the site it speaks for and the binding it was
// measured on.
func rowKey(cell []string) string {
	return strings.Join([]string{cell[columnSite], cell[columnProvider], cell[columnModel], cell[columnEnv]}, " | ")
}

// statusesOf reads the status of each row of a printed readiness report, keyed
// by rowKey. It reads the text, not the build, because it answers for a base
// tree whose code is not this one's. Lines that are not table rows are skipped.
func statusesOf(report string) map[string]string {
	out := map[string]string{}
	inTable := false
	for line := range strings.SplitSeq(report, "\n") {
		cell := columnGap.Split(strings.TrimSpace(line), -1)
		switch {
		case cell[0] == reportColumns[0]:
			inTable = true
		case strings.TrimSpace(line) == "":
			inTable = false
		case inTable && len(cell) >= len(reportColumns):
			out[rowKey(cell)] = cell[columnStatus]
		}
	}
	return out
}

// newlyStale lists the records current in the base report that are not current
// in rows: what the tree behind rows made stale, partial or gone.
func newlyStale(base map[string]string, rows []aicert.ReadinessRow) []string {
	var out []string
	for _, row := range rows {
		cell := cells(row)
		if base[rowKey(cell)] == aicert.StatusCurrent && row.Status() != aicert.StatusCurrent {
			out = append(out, fmt.Sprintf("%s on %s: now %s", row.SiteKey(), bindingOf(cell), row.Status()))
		}
	}
	slices.Sort(out)
	return out
}

func bindingOf(cell []string) string {
	return cell[columnProvider] + " " + cell[columnModel] + " (" + cell[columnEnv] + ")"
}

// renderNewlyStale is the pull request comment, empty when nothing went stale.
func renderNewlyStale(list []string) string {
	if len(list) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "This change makes %d AI certification records stale. A paid run (`make e2e-ai`) re-certifies them; nothing here waits on it.\n\n", len(list))
	for _, line := range list {
		b.WriteString("- " + line + "\n")
	}
	return b.String()
}

// totals is the one line the weekly job prints: what is current and what is owed.
func totals(rows []aicert.ReadinessRow) string {
	count := map[string]int{}
	for _, row := range rows {
		count[row.Status()]++
	}
	return fmt.Sprintf("AI certification records: %d current, %d partial, %d stale, %d absent.",
		count[aicert.StatusCurrent], count[aicert.StatusPartial], count[aicert.StatusStale], count[aicert.StatusAbsent])
}

// notCurrentRungs names each rung of one preset whose record is not current. A
// fallback that is the same model or none is no rung of its own.
func notCurrentRungs(report presetReport) []string {
	var out []string
	for _, rung := range report.Rungs {
		if rung.FirstState != aicert.StatusCurrent {
			out = append(out, fmt.Sprintf("%s: %s on %s is %s", rung.Task, rung.FirstTier, rung.FirstModel, rung.FirstState))
		}
		switch rung.FallbackState {
		case aicert.StatusCurrent, aicert.RungSameModel, aicert.RungNone:
		default:
			out = append(out, fmt.Sprintf("%s: fallback %s on %s is %s", rung.Task, rung.FallbackTier, rung.FallbackModel, rung.FallbackState))
		}
	}
	return out
}
