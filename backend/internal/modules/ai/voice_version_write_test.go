// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"strings"
	"testing"
)

// The statement's columns, its placeholders and its bind list have to agree,
// and nothing in this tree checks that they do — a mismatch is a runtime error
// from Postgres, on whichever write path happens to run first.
func TestTheVoiceVersionWriterBindsEveryColumnItNames(t *testing.T) {
	args := voiceVersionArgs(voiceVersionRow{})
	if len(args) != len(voiceVersionWriteColumns) {
		t.Fatalf("the writer names %d column(s) and binds %d argument(s): the statement would fail at the "+
			"database, on whichever write path ran first", len(voiceVersionWriteColumns), len(args))
	}
	if got := strings.Count(bindPlaceholders(len(args)), "$"); got != len(args) {
		t.Errorf("bindPlaceholders rendered %d placeholder(s) for %d argument(s)", got, len(args))
	}
}

// review_reasons is the column that made this writer necessary: one of the
// three hand-written INSERTs omitted it, and because it is NOT NULL DEFAULT
// '{}' the omission was silent. A version with nothing to review must SAY so.
func TestAVersionWithNothingToReviewWritesAnEmptyListRatherThanNothing(t *testing.T) {
	at := indexOfColumn(t, "review_reasons")
	args := voiceVersionArgs(voiceVersionRow{})
	reasons, ok := args[at].([]string)
	if !ok {
		t.Fatalf("review_reasons binds a %T; the column is a text array and a nil interface would leave it "+
			"to the database default", args[at])
	}
	if len(reasons) != 0 {
		t.Errorf("an unset review_reasons bound %v, want an empty list", reasons)
	}
}

// A caller's reasons reach the column unchanged: an empty list and a populated
// one must be distinguishable, which is the whole point of writing either.
func TestAVersionCarriesTheReviewReasonsItWasGiven(t *testing.T) {
	at := indexOfColumn(t, "review_reasons")
	args := voiceVersionArgs(voiceVersionRow{reviewReasons: []string{"the active version changed"}})
	reasons, ok := args[at].([]string)
	if !ok || len(reasons) != 1 || reasons[0] != "the active version changed" {
		t.Errorf("review_reasons bound %v, want the caller's own list", args[at])
	}
}

func indexOfColumn(t *testing.T, column string) int {
	t.Helper()
	for i, name := range voiceVersionWriteColumns {
		if name == column {
			return i
		}
	}
	t.Fatalf("the writer no longer names %s at all, which is the omission this writer exists to prevent", column)
	return -1
}
