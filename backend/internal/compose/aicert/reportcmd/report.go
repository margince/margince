// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// The terminal rendering of the certification readiness the aicert library
// computes. What a row MEANS — current, partial, stale, absent, and why —
// belongs to aicert.Readiness, which the generated docs/reference/ai-certification.md
// page reads through as well; this file owns only the columns and the prose a
// person reads at a prompt.

import (
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/margince/margince/backend/internal/compose/aicert"
	"github.com/margince/margince/backend/internal/compose/aitasks"
)

// reportColumns is the table's header and, by its length, how wide a row is —
// so an absent site's dashes and a certified site's numbers cannot drift apart
// from the header they sit under.
var reportColumns = []string{
	"SITE", "SCOPE", "STATUS", "SCENARIOS", "BAND",
	"PROVIDER", "MODEL", "ENV",
	"RUNS", "PASSED", "RELIABILITY",
	"ACCEPTED", "WRONG_ANSWER", "INVALID", "ABSTAINED",
}

// renderReadiness reports every shipped site's certification state: the band a
// record reached, the per-outcome counts behind it, the scope the run actually
// covered, and the (provider, model, env) it was measured on.
//
// Nothing here fails or exits non-zero. The certification lane is paid, manual
// and BYOK-gated, so this is a view a human reads before a release decision —
// not a gate, which would make every prompt edit wait on a paid run.
func renderReadiness(census aicert.Census, stamps map[string]string, perScenario map[string]map[string]string, records []aicert.Record) string {
	rows, unclaimed := aicert.Readiness(census, stamps, perScenario, records)

	var buf strings.Builder
	w := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	var writeErr error
	writeRow := func(format string, args ...any) {
		if writeErr != nil {
			return
		}
		_, writeErr = fmt.Fprintf(w, format, args...)
	}

	writeRow("%s\n\n", summarize(rows))
	writeRow("%s\n", strings.Join(reportColumns, "\t"))
	for _, row := range rows {
		writeRow("%s\n", strings.Join(cells(row), "\t"))
	}
	for _, line := range legend(rows, census.Sites, unclaimed) {
		writeRow("%s\n", line)
	}
	if writeErr == nil {
		writeErr = w.Flush()
	}
	if writeErr != nil {
		// An in-memory strings.Builder never actually errors, but a write into
		// the tabwriter is still, mechanically, a write — checked like any other
		// rather than assumed infallible.
		return fmt.Sprintf("aicert: formatting the readiness report: %v\n", writeErr)
	}
	return buf.String()
}

// cells renders one row's columns, one per reportColumns entry. An absent row
// pads with dashes rather than the record's zero value: every number below is a
// count of runs that happened.
func cells(r aicert.ReadinessRow) []string {
	if !r.Certified {
		out := []string{r.SiteKey(), r.ClaimedScope(), r.Status(), r.Coverage()}
		for len(out) < len(reportColumns) {
			out = append(out, aicert.Unmeasured)
		}
		return out
	}
	rec, tally := r.Record, r.Tally
	return []string{
		r.SiteKey(), r.ClaimedScope(), r.Status(), r.Coverage(), tally.Verdict,
		rec.Provider, rec.ServedModel, rec.EnvClass,
		fmt.Sprintf("%d", tally.Runs), fmt.Sprintf("%d", tally.Passed),
		fmt.Sprintf("%.2f", tally.Reliability()),
		fmt.Sprintf("%d", tally.ReportedAccepted), fmt.Sprintf("%d", tally.ReportedWrongAnswer),
		fmt.Sprintf("%d", tally.ReportedInvalid), fmt.Sprintf("%d", tally.ReportedAbstained),
	}
}

// summarize opens the report with the one number a reader wants first: how much
// of what ships is currently certified at all.
func summarize(rows []aicert.ReadinessRow) string {
	sites, covered := map[string]bool{}, map[string]bool{}
	for _, row := range rows {
		sites[row.SiteKey()] = true
		if row.Status() == aicert.StatusCurrent {
			covered[row.SiteKey()] = true
		}
	}
	return fmt.Sprintf("AI certification readiness: %d of %d shipped sites carry a current record.",
		len(covered), len(sites))
}

// taskContextsNotServed names every certified task whose contract has
// production prepend company context, with the scopes its runs went without —
// each task once, however many of its sites have rows. A task whose contract
// prepends none is left out: nothing was missing from it, and listing it would
// bury the ones where something was.
func taskContextsNotServed(rows []aicert.ReadinessRow) []string {
	named := map[string]bool{}
	var out []string
	for _, row := range rows {
		record := row.Record
		if !row.Certified || record.ContextApplied || len(record.ContextScopes) == 0 || named[record.Task] {
			continue
		}
		named[record.Task] = true
		out = append(out, record.Task+" ("+strings.Join(record.ContextScopes, ", ")+")")
	}
	sort.Strings(out)
	return out
}

// legend states what the table's words mean and what a row does NOT say. Both
// belong in the output rather than in a reader's head: the binding is part of
// every claim here, and a row read as a property of the product rather than of
// one deployment is the way this report would mislead.
func legend(rows []aicert.ReadinessRow, sites []aitasks.Site, unclaimed []aicert.Record) []string {
	lines := []string{
		"",
		"Every row is one (provider, model, env) binding: a band says what that deployment did,",
		"and green-lights no other. A record is written per task and covers every site that task",
		"ships, so sites sharing a task share its record — but each row's numbers are that SITE's",
		"own scenarios, never the task's pooled total.",
		"RUNS/PASSED is how often the site did what its scenarios asked. ACCEPTED, WRONG_ANSWER,",
		"INVALID and ABSTAINED are what the site's validator REPORTED: a run can be accepted and",
		"still fail, when the scenario asked for an abstention.",
	}
	lines = append(lines, staleAndAbsentLines(rows)...)
	// No run is served the company context production prepends — the lane has no
	// database to assemble it from — and for most tasks that costs nothing,
	// because their contract prepends none. The tasks whose contract DOES is the
	// part a reader cannot infer from a row, so those rows say it.
	if without := taskContextsNotServed(rows); len(without) > 0 {
		lines = append(lines,
			"Certified without the company context production prepends (this lane runs with no database to",
			"assemble it from): "+strings.Join(without, "; ")+".")
	}
	return append(lines, unclaimedLines(sites, unclaimed)...)
}

// staleAndAbsentLines counts the two states that owe a paid run, and — for a
// stale one — names why each is stale. The count alone tells a reader how much
// is owed; the reason tells them whether clearing it costs one scenario or a
// whole task, which is the difference between doing it now and scheduling it.
//
// A record that predates per-scenario stamps is counted rather than listed. It
// can name no case, so its line would be the same sentence under a different
// label — and twenty of those bury the rows that DO say which scenario moved.
func staleAndAbsentLines(rows []aicert.ReadinessRow) []string {
	var lines []string
	var stale, absent, unnameable int
	var reasons []string
	for _, row := range rows {
		switch row.Status() {
		case aicert.StatusStale:
			stale++
			if row.Standing.TaskStampOnly {
				unnameable++
				continue
			}
			// Led by a bullet rather than the site name alone: this is a note
			// ABOUT a row, and a line that opened like one would read as a
			// second row for the same site.
			reasons = append(reasons, "  - "+row.SiteKey()+" on "+row.Binding()+": "+row.Standing.Reason())
		case aicert.StatusAbsent:
			absent++
		}
	}
	if stale > 0 {
		lines = append(lines,
			fmt.Sprintf("%d stale: the record was scored against a scenario, or a prompt built from one,", stale),
			"that this build no longer sends. Re-certify to make its band a claim again.")
		lines = append(lines, reasons...)
		if unnameable > 0 {
			lines = append(lines,
				fmt.Sprintf("  - %d of them predate per-scenario stamps and can name no case: only the task stamp", unnameable),
				"    can be compared, so re-certifying one costs its whole task.")
		}
	}
	if absent > 0 {
		lines = append(lines,
			fmt.Sprintf("%d absent: never certified on any binding. This is the honest state, not a failure —", absent),
			"the columns are dashes because no run has measured them.")
	}
	if stale > 0 || absent > 0 {
		lines = append(lines,
			"Run `make e2e-ai TASK=<task> MODEL=<provider:model>` to certify (paid: real model, real network).")
	}
	return lines
}

// unclaimedLines names each record no row above could carry, and which of the
// two reasons it is: its task has moved out of this build, or it names no
// scenario of any site that task ships.
func unclaimedLines(sites []aitasks.Site, unclaimed []aicert.Record) []string {
	shippedTasks := map[string]bool{}
	for _, site := range sites {
		shippedTasks[string(site.Task)] = true
	}
	var lines []string
	for _, rec := range unclaimed {
		name := fmt.Sprintf("Record %s/%s_%s_%s", rec.Task, rec.Provider, rec.ServedModel, rec.EnvClass)
		if !shippedTasks[rec.Task] {
			lines = append(lines, name+" covers no site this build registers — a stale artifact of a task that has moved.")
			continue
		}
		// The record's task ships, but nothing in it says which site each run
		// measured, so no row can carry its numbers honestly.
		lines = append(lines, name+" names no scenario of any site its task ships, so no row above can attribute it.")
	}
	return lines
}
