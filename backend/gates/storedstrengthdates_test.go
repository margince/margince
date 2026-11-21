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
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

var strengthDatesFunctionRe = regexp.MustCompile(`(?i)(CREATE\s+(?:OR\s+REPLACE\s+)?|DROP\s+)FUNCTION\s+(?:IF\s+EXISTS\s+)?(contact_strength_dates\w*|trg_activity_strength_dates)\b`)

// strengthDatesUntriggered are the tables the read reaches with no trigger
// that refolds the dates, each with why.
var strengthDatesUntriggered = gatekit.Waive(map[string]string{
	"setting": "the fold reads only the installation.timezone row, and its writer marks every contact's " +
		"dates unfolded through identity's zone hook, so readers fold the history until the clock refold has folded them again",
})

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
	body := contacts.StoredStrengthDatesSQL(false)
	columns := strengthDatesActivityColumns(body)
	tables := strengthDatesTables(body)
	if !slices.Contains(columns, "archived_at") || !slices.Contains(tables, "activity_participant") {
		t.Fatalf("the census read columns %v and tables %v, too few for the strength read", columns, tables)
	}
	watched := strengthDatesWatchedColumns(currentFunctionBodies(t, strengthDatesFunctionRe)["trg_activity_strength_dates"])
	for _, column := range columns {
		if !slices.Contains(watched, column) {
			t.Errorf("the strength read reads activity.%s, and trg_activity_strength_dates does not compare it, "+
				"so a change to it leaves the stored dates wrong. Add it in a new migration.", column)
		}
	}
	for _, table := range tables {
		if strings.Contains(string(raw), "public."+table+"."+table+"_strength_dates") {
			continue
		}
		if !strengthDatesUntriggered.Waived(t, table) {
			t.Errorf("the strength read reads %s, which has no %s_strength_dates trigger, so a change there "+
				"leaves the stored dates wrong", table, table)
		}
	}
	strengthDatesUntriggered.AssertAllMatched(t)
}

var (
	strengthDatesColumnRe    = regexp.MustCompile(`\ba\.([a-z_]+)`)
	strengthDatesTableRe     = regexp.MustCompile(`(?i)\b(?:FROM|JOIN)\s+([a-z_]+)\b`)
	strengthDatesOldColumnRe = regexp.MustCompile(`\bo\.([a-z_]+)`)
)

// strengthDatesActivityColumns are the activity columns the read asks of its
// row, less the id that names it.
func strengthDatesActivityColumns(body string) []string {
	var out []string
	for _, m := range strengthDatesColumnRe.FindAllStringSubmatch(body, -1) {
		if m[1] != "id" && !slices.Contains(out, m[1]) {
			out = append(out, m[1])
		}
	}
	slices.Sort(out)
	return out
}

func strengthDatesTables(body string) []string {
	var out []string
	for _, m := range strengthDatesTableRe.FindAllStringSubmatch(body, -1) {
		if table := strings.ToLower(m[1]); !slices.Contains(out, table) {
			out = append(out, table)
		}
	}
	slices.Sort(out)
	return out
}

// strengthDatesWatchedColumns are the activity columns the statement trigger
// compares between the old and the new row.
func strengthDatesWatchedColumns(body string) []string {
	var out []string
	for _, m := range strengthDatesOldColumnRe.FindAllStringSubmatch(body, -1) {
		out = append(out, m[1])
	}
	return out
}
