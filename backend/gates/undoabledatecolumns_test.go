// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// A `date` column's audit image is written in Postgres's own spelling of a date.
//
// The undo path decides whether a field has moved since an entry by comparing
// that entry's audit image against the live row AS JSON
// (compose/superseded.go's fieldsThatMovedSince). Postgres renders a `date`
// column as "2026-12-01"; a Go time.Time marshals as "2026-12-01T00:00:00Z".
// Both name the same day and nothing compares them as days, so an image written
// from a time.Time reads as MOVED the instant it is written — and Undo refuses a
// change nobody has touched, reporting a supersession that never happened.
//
// That is not a cosmetic failure. Automatic application is allowed to change
// records without asking precisely because a person can put the change back, so
// a kind whose Undo always refuses breaks the bargain the feature rests on. It
// is also invisible: nothing errors, the refusal is a plausible sentence, and
// the field simply becomes permanently un-undoable.
//
// The subject is DERIVED, from the schema and from the undo path's own list of
// record types, so a `date` column added to any of them is a column this gate
// demands an answer about rather than one it silently stops covering.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// dateColumnLine matches one `date`-typed column in the head catalog, e.g.
// `public.deal.expected_close_date date gen=- def=-`. Anchored on the type
// standing alone so `timestamp with time zone` never matches.
var dateColumnLine = regexp.MustCompile(`^public\.([a-z_]+)\.([a-z_0-9]+) date(?: NOT NULL)? `)

// tablesUndoReads are the tables whose field images reach the undo path's
// comparison: compose/undoability.go's undoableRecordTypes, plus the edge type
// fieldsThatMovedSince admits alongside them.
//
// Asserted rather than derived, and the test below holds it against the source
// so a seventh record type cannot quietly escape this gate.
var tablesUndoReads = []string{
	"person", "organization", "deal", "lead", "project", "activity", "relationship",
}

// dateColumnFloor guards against the reader silently matching nothing. Under-
// recognition is the one way this gate must not break: it would read a smaller
// schema, report PASS, and leave no failing assertion to notice.
const dateColumnFloor = 6

// undoReadableDateColumns derives, from the schema, every `date` column on a
// table the undo path compares images for.
func undoReadableDateColumns(t *testing.T) map[string][]string {
	t.Helper()
	raw, err := os.ReadFile("migrations/testdata/head_catalog.txt")
	if err != nil {
		t.Fatalf("reading the head catalog: %v", err)
	}
	reads := map[string]bool{}
	for _, table := range tablesUndoReads {
		reads[table] = true
	}
	columns := map[string][]string{}
	total := 0
	for _, line := range strings.Split(string(raw), "\n") {
		m := dateColumnLine.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil || !reads[m[1]] {
			continue
		}
		columns[m[1]] = append(columns[m[1]], m[2])
		total++
	}
	if total < dateColumnFloor {
		t.Fatalf("derived only %d date column(s) on the tables undo reads and expects at "+
			"least %d — the catalog reader is broken, not the schema", total, dateColumnFloor)
	}
	return columns
}

func TestEveryUndoReadableDateColumnIsWrittenAsADate(t *testing.T) {
	t.Parallel()
	for table, columns := range undoReadableDateColumns(t) {
		// Scoped to the module that OWNS the table, because a column name is not
		// unique across the schema: `fx_rate_date` sits on contract and offer too,
		// and neither is a table the undo path compares images for. A module writes
		// only tables it owns (tableownership_test.go holds that), so its own
		// sources are where this table's writes are and nowhere else.
		owner, declared := tableOwners[table]
		if !declared {
			t.Errorf("table %q carries date columns undo reads and has no declared owner, "+
				"so this gate cannot tell where its writes live", table)
			continue
		}
		sources := goSourcesUnder(t, owner)
		for _, column := range columns {
			assertWrittenAsADate(t, sources, table, column)
		}
	}
}

// assertWrittenAsADate reports any write of one column that is not through a
// date-aware writer. A column nothing writes is fine — it is set at insert or by
// a trigger, and no audit image carries it.
func assertWrittenAsADate(t *testing.T, sources map[string]string, table, column string) {
	t.Helper()
	for path, body := range sources {
		for _, line := range strings.Split(body, "\n") {
			trimmed := strings.TrimSpace(line)
			if !strings.Contains(trimmed, `"`+column+`"`) || !strings.Contains(trimmed, ".Set(") {
				continue
			}
			t.Errorf("%s writes %s.%s through Set: %s\n"+
				"a `date` column's image must be recorded the way Postgres renders one, "+
				"or Undo refuses the change as superseded the instant it is written. "+
				"Use storekit.SetDate (storekit.PlainDate sheds the contract's Date wrapper).",
				path, table, column, trimmed)
		}
	}
}

// A column named through a constant escapes the literal scan above, so the
// constants the deals module uses for its two date columns are checked by name.
// Both are date columns on a table undo reads, and both were written through
// Set until Undo was found to refuse every close-date change.
func TestTheDateColumnConstantsAreWrittenAsDates(t *testing.T) {
	t.Parallel()
	sources := goSourcesUnder(t, tableOwners["deal"])
	for _, constant := range []string{"closeDateField", "fxRateDateColumn"} {
		for path, body := range sources {
			for _, line := range strings.Split(body, "\n") {
				trimmed := strings.TrimSpace(line)
				if !strings.Contains(trimmed, "p.Set("+constant+",") {
					continue
				}
				t.Errorf("%s writes the date column %s through Set: %s\n"+
					"use storekit.SetDate, or Undo refuses the change as superseded",
					path, constant, trimmed)
			}
		}
	}
}

// The list of tables this gate rules over is the undo path's own, and it is the
// one thing above that is asserted rather than derived. A seventh record type
// added there without being added here would carry date columns nothing checks.
func TestThisGateRulesOverEveryTypeTheUndoPathCompares(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("internal/compose/undoability.go")
	if err != nil {
		t.Fatalf("reading the undo path's record types: %v", err)
	}
	block := regexp.MustCompile(`(?s)undoableRecordTypes = \[\]string\{(.*?)\}`).FindStringSubmatch(string(raw))
	if block == nil {
		t.Fatal("undoability.go declares no undoableRecordTypes list, which this gate rules from")
	}
	ruled := map[string]bool{}
	for _, table := range tablesUndoReads {
		ruled[table] = true
	}
	for _, quoted := range regexp.MustCompile(`"([a-z_]+)"`).FindAllStringSubmatch(block[1], -1) {
		if !ruled[quoted[1]] {
			t.Errorf("the undo path compares images for %q and this gate does not rule over it — "+
				"add it to tablesUndoReads, or a date column on that table goes unchecked", quoted[1])
		}
	}
	// And the edge type, which fieldsThatMovedSince admits alongside them.
	if !ruled["relationship"] {
		t.Error("this gate does not rule over the edge type, whose image the undo path also compares")
	}
}

// goSourcesUnder reads every non-test Go file below one directory, keyed by
// path. Derived from the tree rather than listed, so a new module is covered
// the day it is written.
func goSourcesUnder(t *testing.T, root string) map[string]string {
	t.Helper()
	sources := map[string]string{}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		sources[path] = string(body)
		return nil
	}); err != nil {
		t.Fatalf("reading the module sources: %v", err)
	}
	if len(sources) == 0 {
		t.Fatalf("found no Go sources under %s, so this gate is reading nothing", root)
	}
	return sources
}
