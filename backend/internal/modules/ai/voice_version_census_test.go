// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// The two directions of the writer-versus-table comparison, and their own
// tests.
//
// They live outside the integration build tag because they are pure set
// arithmetic over two string lists: the database read is what needs a
// database, not the judgement made from it. Keeping them here means the
// judgement is exercised on every `go test ./...` rather than only in the lane
// that has Postgres — including the direction that had no test at all.

import (
	"slices"
	"testing"
)

// columnsNothingWrites is the direction the census was built for: a column the
// table has that the writer does not fill and nobody has ratified as the
// database's own.
func columnsNothingWrites(inTable, written []string, waived func(string) bool) []string {
	var missing []string
	for _, column := range inTable {
		if slices.Contains(written, column) || waived(column) {
			continue
		}
		missing = append(missing, column)
	}
	return missing
}

// columnsTheTableDoesNotHave is the mirror, and the one the census was blind
// to. Reading only the table's side answers "is every column written?" and
// never "is every written column real?" — so a writer naming a column a
// migration dropped, or misspelling one, passed the gate and failed the INSERT
// at runtime for every caller.
//
// No waiver list applies here. A column the table does not have is not a
// decision anybody can ratify; it is a statement that cannot execute.
func columnsTheTableDoesNotHave(inTable, written []string) []string {
	var unknown []string
	for _, column := range written {
		if !slices.Contains(inTable, column) {
			unknown = append(unknown, column)
		}
	}
	return unknown
}

func TestColumnsNothingWritesReportsOnlyTheUnwrittenAndUnratified(t *testing.T) {
	inTable := []string{"id", "voice_profile_id", "review_reasons"}
	written := []string{"voice_profile_id"}
	waived := func(column string) bool { return column == "id" }

	got := columnsNothingWrites(inTable, written, waived)
	if !slices.Equal(got, []string{"review_reasons"}) {
		t.Errorf("unwritten columns: %v, want [review_reasons] — the written one and the ratified one are "+
			"both accounted for, and only the forgotten one is a finding", got)
	}
	if got := columnsNothingWrites(inTable, []string{"voice_profile_id", "review_reasons"}, waived); got != nil {
		t.Errorf("a writer covering everything unratified still reports %v", got)
	}
}

func TestColumnsTheTableDoesNotHaveReportsAWriterAheadOfItsTable(t *testing.T) {
	inTable := []string{"id", "voice_profile_id"}

	if got := columnsTheTableDoesNotHave(inTable, []string{"voice_profile_id"}); got != nil {
		t.Errorf("a writer naming only real columns reports %v", got)
	}
	got := columnsTheTableDoesNotHave(inTable, []string{"voice_profile_id", "reviewed_reasons"})
	if !slices.Equal(got, []string{"reviewed_reasons"}) {
		t.Errorf("misspelled column: %v, want [reviewed_reasons] — a name the table does not have is an "+
			"INSERT that cannot run, and reading only the table's side never asks the question", got)
	}
}
