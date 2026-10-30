// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
)

// announcedBy names, for each table the check reads that has no event family
// of its own, the family whose events announce a change to it. An empty family
// is a table no marked event announces, and why says why the reading may stand
// until the next run.
var announcedBy = map[string]struct{ family, why string }{
	"activity_commitment_admission": {"", "whether a promise is still open work; it changes as the promise's own " +
		"messages arrive, and those are activity events"},
	"activity_link":        {"activity", "linking an activity is announced as activity.updated"},
	"activity_participant": {"activity", "a participant's answer is written with its activity"},
	"activity_waiting_horizon": {"", "a measured number of days that bounds how old a wait may be; it moves " +
		"for every deal at once and the next run reads it"},
	"capture_thread_verdict": {"", "a held thread hides a signal's evidence, and every read of the findings " +
		"checks the cited message's visibility again"},
	"contact_email": {"", "the address a contact writes from; adding one moves no deal until mail " +
		"from it arrives, which is an activity event"},
	"import_run": {"", "tells an imported task edit from a human one; a finished import run does not change"},
	"setting":    {"", "the installation's time zone, which moves every deal's day at once and only an admin sets"},
	"worklist_pin": {"", "a task its assignee pinned counts as a next step; a pin is the assignee's own " +
		"list and emits no event"},
	"assurance_exception":      {"forecast", "the check's own findings; an answer is forecast.exception_resolved"},
	"assurance_resolution":     {"forecast", "an answer to a finding is forecast.exception_resolved"},
	"audit_log":                {"deal", "the close-date pushes counted are deal.updated writes"},
	"deal_stage_evidence":      {"deal", "evidence for an exit criterion is announced as deal.updated"},
	"deal_suggestion_evidence": {"deal_suggestion", "evidence is written with its suggestion"},
	"relationship":             {"deal", "a deal's stakeholder edge is announced as deal.updated on the deal"},
	"stage_exit_criterion":     {"stage", "a stage's exit criteria are announced as stage.updated"},
}

// sqlTable is a table named after `FROM` or `JOIN`. A name followed by "(" is
// a function and one followed by "." a column, and neither is a table.
var sqlTable = regexp.MustCompile(`(?i)\b(?:FROM|JOIN)\s+([a-z_][a-z0-9_]*)\b([.(]?)`)

// tablesRead lists the tables the statements name, each once.
func tablesRead(statements ...string) []string {
	seen := map[string]bool{}
	for _, statement := range statements {
		for _, match := range sqlTable.FindAllStringSubmatch(statement, -1) {
			name := strings.ToLower(match[1])
			if match[2] != "" || name == "lateral" {
				continue
			}
			seen[name] = true
		}
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// checkReads is what the nightly check reads: the deals and the buying
// signals, rendered from the statements it runs.
func checkReads() string {
	arg := func(any) int { return 1 }
	at := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	return assuranceSubjectsQuery(at, "''", nil, arg) + assuranceSignalSubjectsQuery(at, arg)
}

// undecidedFamilies answers two lists. undecided holds the types, in a family
// that announces a table read, that decided has no answer for. unannounced
// holds the tables with neither a family nor an announcedBy entry.
func undecidedFamilies(tables, catalog []string, decided func(string) bool) (undecided, unannounced []string) {
	families := map[string]bool{}
	for _, table := range tables {
		if slices.ContainsFunc(catalog, func(eventType string) bool { return strings.HasPrefix(eventType, table+".") }) {
			families[table] = true
			continue
		}
		announced, ok := announcedBy[table]
		if !ok || announced.why == "" {
			unannounced = append(unannounced, table)
			continue
		}
		if announced.family != "" {
			families[announced.family] = true
		}
	}
	for _, eventType := range catalog {
		family, _, _ := strings.Cut(eventType, ".")
		if families[family] && !decided(eventType) {
			undecided = append(undecided, eventType)
		}
	}
	return undecided, unannounced
}

func decidedInput(eventType string) bool {
	_, ok := assurance.InputChangeOf(eventType)
	return ok
}

func TestEveryTableTheCheckReadsHasItsEventsDecided(t *testing.T) {
	tables := tablesRead(checkReads())
	for _, want := range []string{"deal", "stage", "offer", "contract", "activity", "signal", "company", "deal_suggestion"} {
		if !slices.Contains(tables, want) {
			t.Fatalf("the census of tables the check reads has no %s (found %v): the derivation lost its subjects", want, tables)
		}
	}
	undecided, unannounced := undecidedFamilies(tables, events.Types(), decidedInput)
	for _, table := range unannounced {
		t.Errorf("the input check reads %s, which has no event family and no entry in announcedBy: "+
			"name the family whose events announce a change to it, and why", table)
	}
	for _, eventType := range undecided {
		t.Errorf("the input check reads what %s announces, and assurance.inputChanges has no decision for it: "+
			"say whether it makes the check's reading out of date", eventType)
	}
	for table := range announcedBy {
		if !slices.Contains(tables, table) {
			t.Errorf("announcedBy names %s, which the input check no longer reads: remove the entry", table)
		}
	}
}

// A table the check starts to read is caught, and so is a new type in a family
// it reads.
func TestANewTableOrEventTheCheckReadsIsReported(t *testing.T) {
	_, unannounced := undecidedFamilies(tablesRead("SELECT 1 FROM deal_forecast_note n"), events.Types(), decidedInput)
	if !slices.Equal(unannounced, []string{"deal_forecast_note"}) {
		t.Fatalf("unannounced = %v, want the new table alone", unannounced)
	}
	catalog := append(events.Types(), "signal.reopened")
	if undecided, _ := undecidedFamilies([]string{"signal"}, catalog, decidedInput); !slices.Equal(undecided, []string{"signal.reopened"}) {
		t.Fatalf("undecided = %v, want the new signal type alone", undecided)
	}
}

func TestTablesReadSkipsFunctionsAndColumns(t *testing.T) {
	got := tablesRead(`SELECT x FROM jsonb_array_elements(s.evidence) e
		JOIN LATERAL (SELECT 1 FROM signal s) l ON true
		WHERE extract(epoch FROM a.occurred_at) > 0 AND EXISTS (SELECT 1 FROM activity_link al)`)
	if want := []string{"activity_link", "signal"}; !slices.Equal(got, want) {
		t.Fatalf("tablesRead = %v, want %v", got, want)
	}
}
