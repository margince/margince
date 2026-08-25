// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"strings"
	"testing"
	"time"
)

// The statement's columns, its placeholders and its bind list have to agree,
// and nothing in this tree checks that they do — a mismatch is a runtime error
// from Postgres, on whichever write path happens to run first.
func TestTheVoiceVersionWriterBindsEveryColumnItNames(t *testing.T) {
	bound := voiceVersionBindings(voiceVersionRow{})
	if got := strings.Count(bindPlaceholders(len(bound)), "$"); got != len(bound) {
		t.Errorf("bindPlaceholders rendered %d placeholder(s) for %d column(s)", got, len(bound))
	}
	// Counting is not the interesting half any more — a pair cannot be
	// miscounted. What a pair CAN still be is duplicated, and two entries
	// naming one column is a statement Postgres refuses.
	seen := map[string]bool{}
	for _, column := range bound {
		if seen[column.name] {
			t.Errorf("the writer names %s twice; the statement would fail at the database", column.name)
		}
		seen[column.name] = true
	}
}

// The value bound to each column is the field named for it.
//
// This is what the pairing buys, and it is the failure the two parallel slices
// this replaced could not see: swapping model_provider and model_name kept
// twenty columns and twenty arguments, passed every count, and put each string
// in the other's column. So each field is given a value only it could produce
// and the row is read back through the binding list by NAME.
func TestEveryVoiceVersionColumnCarriesItsOwnField(t *testing.T) {
	activated := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	predecessor := 41
	row := voiceVersionRow{
		profileVersion: 42, status: "candidate", voiceProfileMD: "# voice",
		sourceHash: "sha-of-the-sources", sourceCount: 7, reason: "the sources moved",
		predecessorVersion: &predecessor, modelProvider: "a-provider", modelName: "a-model",
		builderVersion: "builder-9", activationPolicyVersion: "policy-3",
		reviewReasons: []string{"a human must look"}, activatedAt: &activated,
		source: "a-source", capturedBy: "human:someone",
		now: time.Date(2026, 3, 4, 5, 6, 8, 0, time.UTC),
	}
	want := map[string]any{
		"profile_version":           42,
		"status":                    "candidate",
		"voice_profile_md":          "# voice",
		"source_hash":               "sha-of-the-sources",
		"source_count":              7,
		"reason":                    "the sources moved",
		"predecessor_version":       &predecessor,
		"model_provider":            "a-provider",
		"model_name":                "a-model",
		"builder_version":           "builder-9",
		"activation_policy_version": "policy-3",
		"activated_at":              &activated,
		"source":                    "a-source",
		"captured_by":               "human:someone",
		"updated_at":                row.now,
	}
	bound := map[string]any{}
	for _, column := range voiceVersionBindings(row) {
		bound[column.name] = column.value
	}
	for column, expected := range want {
		if got, ok := bound[column]; !ok {
			t.Errorf("the writer no longer names %s", column)
		} else if got != expected {
			t.Errorf("%s carries %v, want %v — the column list and the values have come apart",
				column, got, expected)
		}
	}
}

// review_reasons is the column that made this writer necessary: one of the
// three hand-written INSERTs omitted it, and because it is NOT NULL DEFAULT
// '{}' the omission was silent. A version with nothing to review must SAY so.
func TestAVersionWithNothingToReviewWritesAnEmptyListRatherThanNothing(t *testing.T) {
	value := valueBoundTo(t, "review_reasons", voiceVersionRow{})
	reasons, ok := value.([]string)
	if !ok {
		t.Fatalf("review_reasons binds a %T; the column is a text array and a nil interface would leave it "+
			"to the database default", value)
	}
	if len(reasons) != 0 {
		t.Errorf("an unset review_reasons bound %v, want an empty list", reasons)
	}
}

// A caller's reasons reach the column unchanged: an empty list and a populated
// one must be distinguishable, which is the whole point of writing either.
func TestAVersionCarriesTheReviewReasonsItWasGiven(t *testing.T) {
	value := valueBoundTo(t, "review_reasons", voiceVersionRow{
		reviewReasons: []string{"the active version changed"},
	})
	reasons, ok := value.([]string)
	if !ok || len(reasons) != 1 || reasons[0] != "the active version changed" {
		t.Errorf("review_reasons bound %v, want the caller's own list", value)
	}
}

// valueBoundTo reads one column's value out of the binding list by name.
//
//craft:ignore naked-any a bind value IS any — it is what pgx.Tx takes and what boundColumn.value is declared as, so a concrete type here would only be a cast the caller has to undo. Every caller type-asserts what it expects and fails the test when the assertion misses, which is the assertion being made.
func valueBoundTo(t *testing.T, column string, row voiceVersionRow) any {
	t.Helper()
	for _, bound := range voiceVersionBindings(row) {
		if bound.name == column {
			return bound.value
		}
	}
	t.Fatalf("the writer no longer names %s at all, which is the omission this writer exists to prevent", column)
	return nil
}
