// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"reflect"
	"testing"
)

func TestMultiselectFiltersCompareWholeChoices(t *testing.T) {
	fields := map[string]Field{"choices": {Expr: "t.cf_choices", Type: FieldMultiselect}}
	for _, tc := range []struct {
		op    string
		value any
		sql   string
	}{ //craft:ignore naked-any Predicate.Value accepts scalar or list operands
		{OpEq, "A,B", "COALESCE($1 = ANY(t.cf_choices), false)"},
		{OpNeq, "A,B", "NOT COALESCE($1 = ANY(t.cf_choices), false)"},
		{OpIn, []any{"A,B", "C++"}, "t.cf_choices && $1::text[]"},
	} {
		var args []any
		sql, err := CompilePredicate(Predicate{Field: "choices", Op: tc.op, Value: tc.value}, fields, func(value any) int { args = append(args, value); return len(args) }) //craft:ignore naked-any the compiler's parameter accumulator accepts typed operands
		if err != nil || sql != tc.sql || len(args) != 1 {
			t.Fatalf("%s: %s %v %v", tc.op, sql, args, err)
		}
	}
}

func TestMalformedChoiceArraysAreNeverPartiallySaved(t *testing.T) {
	if _, ok := stringSet([]any{"valid", 2}); ok {
		t.Fatal("accepted a malformed array")
	}
	got, ok := stringSet([]any{"A,B", "C++", "A,B"})
	if !ok || !reflect.DeepEqual(got, []string{"A,B", "C++"}) {
		t.Fatalf("changed choice spelling: %v", got)
	}
}
