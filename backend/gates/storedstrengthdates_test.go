// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// The strength dates a contact row stores are the live recency read folded at
// now(). The SQL functions that fold them cannot call Go. So their migration
// carries the read's rendered text, and this gate holds the live body to it.
// A trigger keeps the dates on every input, so the gate also holds that each
// activity column and each table the read reaches has one.

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

var strengthDatesFunctionRe = regexp.MustCompile(`(?i)(CREATE\s+(?:OR\s+REPLACE\s+)?|DROP\s+)FUNCTION\s+(?:IF\s+EXISTS\s+)?(contact_strength_dates\w*)\b`)

// strengthDatesUnwatched are the inputs of the read that no trigger needs to
// watch, each with why. A key is a table, "table EVENT" or "table.column".
var strengthDatesUnwatched = gatekit.Waive(map[string]string{
	"setting": "the fold reads only the installation.timezone row, and its writer marks every contact's " +
		"dates unfolded through identity's zone hook, so readers fold the history until the clock refold has folded them again",
	"activity INSERT": "a new activity has no link yet, and the insert of its link touches the contact",
	"activity DELETE": "the delete cascades to the activity's links, and their delete trigger recomputes each contact",
})

// strengthDatesZoneHook is the wiring the setting waiver rests on.
const strengthDatesZoneHook = "WithZoneChange(contacts.UnfoldStrengthDatesTx)"

func TestStoredStrengthDatesFoldAsTheLiveRead(t *testing.T) {
	t.Parallel()
	bodies := currentFunctionBodies(t, strengthDatesFunctionRe)
	for name, oneActivity := range map[string]bool{"contact_strength_dates": false, "contact_strength_dates_among": true} {
		want := normalizeSQL(contacts.StoredStrengthDatesSQL(oneActivity))
		if got, ok := bodies[name]; !ok {
			t.Errorf("no migration defines %s", name)
		} else if normalizeSQL(got) != want {
			t.Errorf("%s no longer folds what contacts.StoredStrengthDatesSQL renders, so the stored dates "+
				"and the live read disagree. Replace the function in a new migration with:\n%s", name, want)
		}
	}
}

func TestEveryInputOfTheStoredStrengthDatesHasATrigger(t *testing.T) {
	t.Parallel()
	raw, err := gatekit.ReadFile(vocabularyCatalog)
	if err != nil {
		t.Fatal(err)
	}
	wiring, err := gatekit.ReadFile("internal/compose/installationcountry.go")
	if err != nil {
		t.Fatal(err)
	}
	reads := strengthDatesReads(contacts.StoredStrengthDatesSQL(false))
	if !slices.Contains(reads["activity"], "archived_at") || !slices.Contains(reads["activity_participant"], "response") {
		t.Fatalf("the census read %v, too little for the strength read", reads)
	}
	waived := func(input string) bool {
		if input == "setting" && !strings.Contains(string(wiring), strengthDatesZoneHook) {
			return false
		}
		return strengthDatesUnwatched.Waived(t, input)
	}
	for _, finding := range strengthDatesFindings(reads, strengthDatesTriggers(string(raw)),
		currentFunctionBodies(t, strengthDatesTriggerFunctionRe), waived) {
		t.Error(finding)
	}
	strengthDatesUnwatched.AssertAllMatched(t)
}

// The census names a joined table's column its UPDATE trigger does not
// compare. It also names an event the table has no trigger for.
func TestTheStrengthDatesCensusSeesAnUnwatchedJoinedColumn(t *testing.T) {
	t.Parallel()
	reads := strengthDatesReads("SELECT tp.role FROM activity_participant tp WHERE tp.response IS NULL")
	triggers := map[string]map[string]string{"activity_participant": {"INSERT": "f_added", "UPDATE": "f_moved"}}
	bodies := map[string]string{"f_moved": "WHERE (o.role) IS DISTINCT FROM (n.role)"}
	findings := strings.Join(strengthDatesFindings(reads, triggers, bodies, func(string) bool { return false }), "\n")
	for _, want := range []string{"activity_participant.response", "activity_participant has no DELETE trigger"} {
		if !strings.Contains(findings, want) {
			t.Errorf("the census missed %q in a fixture that has it; it reported:\n%s", want, findings)
		}
	}
	if strings.Contains(findings, "activity_participant.role") {
		t.Errorf("the census reported a column its trigger compares:\n%s", findings)
	}
}

var (
	strengthDatesTriggerFunctionRe = regexp.MustCompile(`(?i)(CREATE\s+(?:OR\s+REPLACE\s+)?|DROP\s+)FUNCTION\s+(?:IF\s+EXISTS\s+)?(trg_\w*strength_dates\w*)\b`)
	strengthDatesAliasRe           = regexp.MustCompile(`(?i)\b(?:FROM|JOIN)\s+([a-z_]+)\s+([a-z_]+)\b`)
	strengthDatesTriggerRe         = regexp.MustCompile(`(?m)^public\.([a-z_]+)\.[a-z_]*strength_dates[a-z_]* CREATE TRIGGER \S+ AFTER (INSERT|UPDATE|DELETE) ON .* EXECUTE FUNCTION ([a-z_]+)\(`)
	strengthDatesOldColumnRe       = regexp.MustCompile(`\bo\.([a-z_]+)`)
)

// strengthDatesReads maps each table the read joins to the columns it asks of
// it through any alias. The id that names the row is left out.
func strengthDatesReads(body string) map[string][]string {
	out := map[string][]string{}
	for _, m := range strengthDatesAliasRe.FindAllStringSubmatch(body, -1) {
		table, alias := strings.ToLower(m[1]), strings.ToLower(m[2])
		if slices.Contains([]string{"on", "where", "join", "left", "cross", "inner", "group", "order"}, alias) {
			continue
		}
		if _, seen := out[table]; !seen {
			out[table] = nil
		}
		columnRe := regexp.MustCompile(`\b` + regexp.QuoteMeta(alias) + `\.([a-z_]+)`)
		for _, c := range columnRe.FindAllStringSubmatch(body, -1) {
			if c[1] != "id" && !slices.Contains(out[table], c[1]) {
				out[table] = append(out[table], c[1])
			}
		}
	}
	for table := range out {
		slices.Sort(out[table])
	}
	return out
}

// strengthDatesTriggers maps each table to the function its strength-date
// trigger runs for each event.
func strengthDatesTriggers(catalog string) map[string]map[string]string {
	out := map[string]map[string]string{}
	for _, m := range strengthDatesTriggerRe.FindAllStringSubmatch(catalog, -1) {
		if out[m[1]] == nil {
			out[m[1]] = map[string]string{}
		}
		out[m[1]][m[2]] = m[3]
	}
	return out
}

// strengthDatesFindings lists the inputs of the read a trigger misses. That is an
// event a table has no trigger for, or a column its UPDATE trigger does not compare.
func strengthDatesFindings(reads map[string][]string, triggers map[string]map[string]string,
	bodies map[string]string, waived func(string) bool,
) []string {
	var findings []string
	for _, table := range sortedKeys(reads) {
		if len(triggers[table]) == 0 && waived(table) {
			continue
		}
		for _, event := range []string{"INSERT", "UPDATE", "DELETE"} {
			if _, ok := triggers[table][event]; !ok && !waived(table+" "+event) {
				findings = append(findings, fmt.Sprintf("the strength read reads %s, and %s has no %s trigger "+
					"that moves the stored dates. Add one in a new migration.", table, table, event))
			}
		}
		compared := strengthDatesOldColumnRe.FindAllStringSubmatch(bodies[triggers[table]["UPDATE"]], -1)
		for _, column := range reads[table] {
			if !slices.ContainsFunc(compared, func(m []string) bool { return m[1] == column }) && !waived(table+"."+column) {
				findings = append(findings, fmt.Sprintf("the strength read reads %s.%s, and %s's UPDATE trigger "+
					"does not compare it, so a change to it leaves the stored dates wrong", table, column, table))
			}
		}
	}
	return findings
}
