// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"reflect"
	"sort"
	"testing"
)

// The alias table and the row reader meet at a field key, and the way they
// disagree is not a typo — the compiler catches an undefined constant. It is a
// key one side uses and the other never produces: a header alias pointing at a
// field the reader never asks for imports nothing from that column, and a field
// the reader asks for that no alias produces reads empty from every export
// forever. Both are silent. `at` answers "" for an unindexed field, and an
// import that quietly ignored a column while reporting success is the outcome
// LinkedInImportResult exists to prevent.
//
// Driven through headerIndex and linkedInRowFrom rather than compared against a
// list of the keys: a test restating the seven constants is a second copy of
// them, and would agree with the reader about a field neither one reads.

// probeValues are the shapes a probed cell is tried with. A field is reachable
// if ANY of them changes the row, which keeps this test from carrying a
// per-field table — the one thing that would make it a second copy of the
// vocabulary. The date is here because `connected` is parsed rather than
// copied, so only a value that parses can prove that column is read.
var probeValues = []string{"probe-value", "02 Jan 2006", "probe@example.com"}

// headerAliasFor picks one alias per field, so the probe header carries every
// field exactly once. Sorted, so the header this test builds is the same on
// every run and a failure names the same alias twice running.
func headerAliasFor(t *testing.T) map[string]string {
	t.Helper()
	byField := map[string][]string{}
	for alias, field := range linkedInHeaderAliases {
		byField[field] = append(byField[field], alias)
	}
	if len(byField) == 0 {
		t.Fatal("linkedInHeaderAliases is empty, so this test probes nothing")
	}
	chosen := make(map[string]string, len(byField))
	for field, aliases := range byField {
		sort.Strings(aliases)
		chosen[field] = aliases[0]
	}
	return chosen
}

func TestEveryFieldTheAliasTableProducesIsReadFromTheRow(t *testing.T) {
	aliases := headerAliasFor(t)
	fields := make([]string, 0, len(aliases))
	for field := range aliases {
		fields = append(fields, field)
	}
	sort.Strings(fields)

	header := make([]string, len(fields))
	for i, field := range fields {
		header[i] = aliases[field]
	}
	index := headerIndex(header)
	if index == nil {
		t.Fatalf("headerIndex refused a header carrying one alias for each of the %d fields "+
			"the alias table produces (%v) — it cannot recognize its own vocabulary", len(fields), header)
	}
	if len(index) != len(fields) {
		t.Errorf("headerIndex placed %d of the %d fields the alias table produces; a field the "+
			"index never carries reads empty from every export", len(index), len(fields))
	}

	// The name columns are what headerIndex requires before it accepts a header
	// at all, so the baseline fills them: without a name linkedInRowFrom refuses
	// the row and every probe below would compare two refusals.
	baseline, ok := rowFrom(header, index, map[string]string{csvFirst: "Baseline", csvLast: "Person"})
	if !ok {
		t.Fatal("linkedInRowFrom refused a row carrying a first and last name, so no probe can be read against it")
	}
	for _, field := range fields {
		if reached(t, header, index, baseline, field) {
			continue
		}
		t.Errorf("the alias %q maps a column onto the field %q and linkedInRowFrom reads nothing "+
			"from it: every value in that column is dropped, and the import reports the row as "+
			"imported. Read the field, or stop mapping a header onto it.",
			aliases[field], field)
	}
}

// reached reports whether filling field's column changes the row the reader
// builds, trying each probe shape in turn.
func reached(t *testing.T, header []string, index map[string]int, baseline linkedInRow, field string) bool {
	t.Helper()
	for _, value := range probeValues {
		cells := map[string]string{csvFirst: "Baseline", csvLast: "Person", field: value}
		row, ok := rowFrom(header, index, cells)
		if ok && !reflect.DeepEqual(row, baseline) {
			return true
		}
	}
	return false
}

// rowFrom assembles a record positioned by index and reads it back through the
// real reader, so what this test exercises is the production path.
func rowFrom(header []string, index map[string]int, cells map[string]string) (linkedInRow, bool) {
	record := make([]string, len(header))
	for field, value := range cells {
		if at, ok := index[field]; ok {
			record[at] = value
		}
	}
	return linkedInRowFrom(record, index)
}
