// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// Every ledger of executions either states its window or says why it has none.
//
// One row per execution is the shape that grows with traffic rather than with
// the records a customer keeps: a run per automation tick, a delivery per
// webhook, a scan per company. The retention engine is good and these sit
// outside it, so each table here grew without a bound and nothing failed.
//
// The corpus is derived from the catalog by the shape of the name, which is
// what this cannot do better: a ledger called something else is invisible to
// it. Stated because a census that reads short reports PASS and leaves nothing
// to notice. What it does close is the next table named for the thing it
// records joining unexamined.
//
// No window is invented here. How long an operator needs a provider_run to
// debug a failed send is their judgement, and a default that silently deleted
// their history would be the wrong way to find that out. The register carries
// what each table is and what deciding its window would cost.
var runLedgerWindow = gatekit.Waive(map[string]string{
	"runner_job": "River's own job table, which the installation reset also leaves alone: " +
		"its runtime manages retention and truncating underneath a running worker is not ours to do",
	"agent_run": "one row per agent execution, holding the outcome and the degrade reason. The " +
		"window is an operator judgement: a settled run is support history until somebody says how " +
		"long they need it",
	"provider_run": "one row per provider submission, and the state machine a settle re-reads " +
		"(execute.go). The terminal states are what a window would take, and how long a failed send " +
		"stays debuggable is the operator's call",
	"import_run": "one row per migration run, holding the report a resumed run reads to avoid " +
		"understating a one-way cutover. A window has to outlive the cutover it describes",
	"report_run": "one row per report generation. The edition it produced has its own retention " +
		"ladder, so this is the attempt rather than the artefact",
	"workflow_run": "one row per automation tick, and auth.LiveIntakeTaskClause reads the planned " +
		"target of a route_lead run to exclude imported follow-ups. A window here is entangled with " +
		"that clause rather than free",
	"assurance_run": "one row per assurance pass over a deal. The verdict is the product; the run " +
		"is how it was reached",
	"brief_run": "one row per morning brief composed, which the annotate path locks and writes " +
		"narrative into. A window is a judgement about how far back a reader may reopen a brief",
	"capture_backfill": "one row per mailbox backfill, holding the cursor and the generation a " +
		"flush validates against. Deleting a live one would orphan its progress",
	"capture_sweep_run": "one row per capture sweep receipt. Its only DELETE is a merge of " +
		"duplicate receipts, which is not a window",
	"close_date_run": "one row per close-date sweep, holding the cursor the next run resumes from",
	"company_scan": "one row per company scan, and its only DELETE is the collision merge. The " +
		"scan's own output is what a reader sees, so a window is about how long a refusal stays " +
		"explainable",
	"signal_thread_scan": "one row per thread scanned, with the two DELETEs serving a thread merge " +
		"and an audience rescope rather than retention",
	"notification_digest_run": "one row per digest send attempt, holding the mail attempt stamp " +
		"a retry reads",
	"activity_meeting_rsvp_backfill": "one row per RSVP backfill settled. The settle stamp is the " +
		"only column, so a window is the whole question",
	"webhook_delivery": "one row per delivery attempt, holding the dead-letter stamp and the retry " +
		"schedule. The retry path reads it, so a window must clear the longest backoff",
})

// ledgerName is the shape of a table that records executions rather than
// records. Derived from the name because nothing in the catalog marks it: a
// row-per-execution table carries no column saying so.
var ledgerName = regexp.MustCompile(`(_run|_attempt|_delivery|_scan|_backfill|_tick|_job)s?$`)

// windowedDelete is a DELETE whose predicate compares a time: the shape that
// bounds a table rather than serving a merge or a cascade.
var windowedDelete = regexp.MustCompile(`(?is)DELETE\s+FROM\s+([a-z_0-9]+)\b(.{0,240})`)

var timeBound = regexp.MustCompile(`(?i)(_at\s*<|<\s*\$|make_interval|now\(\)\s*-|cutoff)`)

func TestEveryLedgerOfExecutionsStatesItsWindow(t *testing.T) {
	t.Parallel()
	ledgers := ledgerTables(t)
	// The corpus is every ledger-shaped table and does not shrink as windows
	// are decided, so a floor on it catches a reader that has stopped reading.
	if len(ledgers) < 12 {
		t.Fatalf("found %d ledger-shaped tables, which is too few to be this schema: the catalog's "+
			"line shape or ledgerName has moved", len(ledgers))
	}
	bounded := windowedLedgers(t)
	defer runLedgerWindow.AssertAllMatched(t)
	for _, table := range ledgers {
		if bounded[table] {
			continue
		}
		if runLedgerWindow.Waived(t, table) {
			continue
		}
		t.Errorf("%s records one row per execution and nothing bounds it: no DELETE in the tree "+
			"compares a time on it.\nGive it a windowed sweep, or say in runLedgerWindow what the "+
			"table is and what deciding its window would cost.", table)
	}
	for _, table := range runLedgerWindow.Subjects() {
		if bounded[table] {
			t.Errorf("runLedgerWindow explains %q, which a windowed sweep now bounds: delete the "+
				"line, so what remains is the work still owed", table)
		}
	}
}

// ledgerTables derives the corpus from the catalog.
func ledgerTables(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile("migrations/testdata/head_catalog.txt")
	if err != nil {
		t.Fatalf("reading the head catalog: %v", err)
	}
	seen := map[string]bool{}
	for line := range strings.SplitSeq(string(raw), "\n") {
		m := regexp.MustCompile(`^public\.([a-z_0-9]+)\.`).FindStringSubmatch(strings.TrimSpace(line))
		if m != nil && ledgerName.MatchString(m[1]) {
			seen[m[1]] = true
		}
	}
	var out []string
	for table := range seen {
		out = append(out, table)
	}
	sort.Strings(out)
	return out
}

// windowedLedgers reads the tree for DELETEs that compare a time, which is the
// only evidence in the source that a table is bounded.
func windowedLedgers(t *testing.T) map[string]bool {
	t.Helper()
	bounded := map[string]bool{}
	for _, root := range []string{"internal", "../extensions"} {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") ||
				strings.HasSuffix(path, "_test.go") {
				return nil //nolint:nilerr // a tree that cannot be walked is the caller's problem, not a finding
			}
			raw, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			for _, m := range windowedDelete.FindAllStringSubmatch(string(raw), -1) {
				if timeBound.MatchString(m[2]) {
					bounded[m[1]] = true
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}
	return bounded
}
