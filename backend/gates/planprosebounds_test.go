// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// A plan's prose columns are bounded twice, and the two numbers must agree.
//
// The module refuses an over-long field at the seam (weeklyplan.bounded, which
// answers a 422 naming the field) and the database refuses it again in a CHECK.
// Neither is redundant: the seam gives the client a usable error instead of a
// constraint violation, and the CHECK is what holds when a second writer
// arrives that never passed through the seam.
//
// They drift silently. A CHECK raised to 4000 while Go still refuses at 2000
// makes the database's extra room unreachable; lowered to 1000, the seam
// cheerfully accepts text the INSERT then rejects as a 500. Both directions are
// failures and neither shows up in a test of either side alone, so this
// compares them.
//
// The corpus is DERIVED from the migrations rather than listed here: a gate
// that hard-codes which columns it protects stops protecting the next one
// somebody adds, and reports PASS while doing it.

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// A `length(<column>) <= <n>` clause in any CHECK on a weekly_plan table.
var planBoundCheck = regexp.MustCompile(`(?i)\b(?:char_length|character_length|length)\((\w+)\)\s*<=\s*(\d+)`)

// A dropped bound constraint, by its name.
var planBoundDrop = regexp.MustCompile(`(?i)DROP\s+CONSTRAINT\s+(?:IF\s+EXISTS\s+)?(\w*_bound\w*)`)

// The Go constants the weeklyplan module bounds its prose with.
var planBoundConst = regexp.MustCompile(`(?m)^\s*(\w*[Bb]ound)\s*=\s*(\d+)`)

// gatekit:fixture the column-to-constant pairing, which neither side declares
//
// Which Go constant each column is bounded by at the seam. This pairing is the
// one thing a machine cannot read off either side — the call sites say
// `bounded(fieldLabel, ..., labelBound)` and the column name is not in it — so
// it is stated here and every column found in the schema must appear.
var planColumnBound = map[string]string{
	"label":            "labelBound",
	"help_requested":   "proseBound",
	"manager_response": "proseBound",
	"risks":            "proseBound",
	"capacity_note":    "proseBound",
}

func TestEveryPlanProseColumnMatchesItsGoBound(t *testing.T) {
	t.Parallel()
	consts := planGoBounds(t)
	columns := planSchemaBounds(t)

	if len(columns) == 0 {
		t.Fatal("no bounded prose column found on a weekly_plan table: the scan " +
			"has stopped reading its own subject, which reports PASS while holding nothing")
	}
	for column, sqlBound := range columns {
		name, paired := planColumnBound[column]
		if !paired {
			t.Errorf("weekly_plan column %q carries a length CHECK but no entry in "+
				"planColumnBound: name the Go constant that bounds it at the seam, "+
				"or this gate silently stops covering it", column)
			continue
		}
		goBound, known := consts[name]
		if !known {
			t.Errorf("column %q names Go constant %s, which weeklyplan does not define",
				column, name)
			continue
		}
		if goBound != sqlBound {
			t.Errorf("column %q is bounded at %d in SQL and %d in Go (%s): "+
				"the seam and the constraint must refuse the same text",
				column, sqlBound, goBound, name)
		}
	}
}

// The other direction: a Go bound this gate claims to pair must actually reach
// a column. A constant left in the map after its column is dropped makes the
// pairing above look complete when it is not.
func TestEveryPairedPlanBoundReachesAColumn(t *testing.T) {
	t.Parallel()
	columns := planSchemaBounds(t)
	for column := range planColumnBound {
		if _, found := columns[column]; !found {
			t.Errorf("planColumnBound names column %q, which carries no length CHECK "+
				"on any weekly_plan table: drop the entry or restore the constraint", column)
		}
	}
}

// planGoBounds reads the module's own bound constants.
func planGoBounds(t *testing.T) map[string]int {
	t.Helper()
	out := map[string]int{}
	for _, file := range planModuleFiles(t) {
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("reading %s: %v", file, err)
		}
		for _, m := range planBoundConst.FindAllStringSubmatch(string(body), -1) {
			n, err := strconv.Atoi(m[2])
			if err != nil {
				t.Fatalf("%s: bound %q is not a number: %v", file, m[2], err)
			}
			out[m[1]] = n
		}
	}
	return out
}

// planSchemaBounds reads every length CHECK on a weekly_plan table out of the
// migrations, newest wins — a later migration may raise a ceiling, and the
// current one is what the database actually enforces.
func planSchemaBounds(t *testing.T) map[string]int {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("migrations", "core", "*.up.sql"))
	if err != nil {
		t.Fatalf("listing migrations: %v", err)
	}
	sortStrings(files)
	out := map[string]int{}
	for _, file := range files {
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("reading %s: %v", file, err)
		}
		text := string(body)
		if !strings.Contains(text, "weekly_plan") {
			continue
		}
		for _, m := range planBoundCheck.FindAllStringSubmatch(text, -1) {
			n, err := strconv.Atoi(m[2])
			if err != nil {
				t.Fatalf("%s: bound %q is not a number: %v", file, m[2], err)
			}
			out[m[1]] = n
		}
		// A later migration that DROPS a bound retires it here too. Without
		// this the map keeps the historical number and the gate goes on
		// asserting a ceiling the database no longer enforces — a pass that
		// describes a schema that no longer exists.
		for _, m := range planBoundDrop.FindAllStringSubmatch(text, -1) {
			delete(out, boundedColumnOf(m[1]))
		}
	}
	return out
}

// boundedColumnOf reads the column out of a bound constraint's name.
//
// The tree names them `<table>_<column>_bound(ed)`, so the column is what is
// left once the table prefix and the suffix are stripped. Derived rather than
// mapped: a constraint this gate could not attribute would silently survive a
// drop.
func boundedColumnOf(constraint string) string {
	name := strings.TrimPrefix(constraint, "weekly_plan_commitment_")
	name = strings.TrimPrefix(name, "weekly_plan_")
	name = strings.TrimSuffix(name, "_bounded")
	return strings.TrimSuffix(name, "_bound")
}

func planModuleFiles(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("internal", "modules", "weeklyplan", "*.go"))
	if err != nil {
		t.Fatalf("listing the weeklyplan module: %v", err)
	}
	return files
}
