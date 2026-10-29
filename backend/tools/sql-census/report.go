// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// shownBelowBar is how many statements under the bar the report still lists, so
// a reader sees how close the next ones are.
const shownBelowBar = 10

func report(w io.Writer, stmts []*statement, flagKB int) error {
	var out strings.Builder
	var measured, failed []*statement
	for _, s := range stmts {
		if s.err == "" {
			measured = append(measured, s)
		} else {
			failed = append(failed, s)
		}
	}
	sort.SliceStable(measured, func(i, j int) bool { return measured[i].peakKB > measured[j].peakKB })
	flagged := 0
	for _, s := range measured {
		if s.peakKB >= flagKB {
			flagged++
		}
	}
	fmt.Fprintf(&out, "# SQL census\n\n%d statements, %d planned, %d could not be planned alone. ", len(stmts), len(measured), len(failed))
	fmt.Fprintf(&out, "**%d** grow a backend by %d MB or more while planning.\n\n", flagged, flagKB/1024)
	fmt.Fprintln(&out, "| # | plan peak MB | plan ms | SQL KB | sent from |")
	fmt.Fprintln(&out, "|--:|--:|--:|--:|---|")
	for i, s := range measured[:min(len(measured), flagged+shownBelowBar)] {
		fmt.Fprintf(&out, "| %d | %d | %.0f | %.1f | `%s` |\n", i+1, s.peakKB/1024, s.planMs, float64(len(s.sql))/1024, sender(s))
	}
	if len(failed) > 0 {
		fmt.Fprintf(&out, "\n## Not planned (%d)\n\nA fragment or a statement that needs its caller's types cannot be planned alone; the census does not see its cost.\n\n", len(failed))
		for _, s := range failed {
			fmt.Fprintf(&out, "- `%s`: %s\n", sender(s), s.err)
		}
	}
	_, err := io.WriteString(w, out.String())
	return err
}

// sender is the first frame outside a test: the function that sent the
// statement. A clause built in another file is not on the stack by then.
func sender(s *statement) string {
	for _, chain := range s.chains {
		for _, frame := range chain {
			file, _, _ := strings.Cut(frame, " ")
			if !strings.Contains(file, "_test.go") {
				return file
			}
		}
	}
	return "unknown"
}
