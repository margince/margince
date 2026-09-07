// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"errors"
	"strings"
	"testing"
)

// picklistFields is one field of each shape the refusal has a rule for.
var picklistFields = map[string]Field{
	"status":  {Expr: "status", Type: FieldPicklist, Options: []string{"open", "won", "lost"}},
	"name":    {Expr: "display_name", Type: FieldText},
	"amount":  {Expr: "amount_minor", Type: FieldNumber},
	"retired": {Expr: "classification", Type: FieldPicklist},
}

func TestAStoredFilterRefusesAValueOutsideItsPicklist(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		pred    Predicate
		refused bool
	}{
		{"eq on a stranger", Predicate{Field: "status", Op: OpEq, Value: "Open"}, true},
		{"neq on a stranger", Predicate{Field: "status", Op: OpNeq, Value: "Open"}, true},
		{"in with one stranger among members", Predicate{
			Field: "status", Op: OpIn, Value: []any{"open", "Won", "lost"},
		}, true},
		{"eq on a member", Predicate{Field: "status", Op: OpEq, Value: "open"}, false},
		{"in with every member known", Predicate{
			Field: "status", Op: OpIn, Value: []any{"open", "won"},
		}, false},
		// The operand is the QUESTION, not a value: "is this set at all" names
		// no member of the set.
		{"exists on a picklist", Predicate{Field: "status", Op: OpExists, Value: true}, false},
		// A picklist carrying no set is a picklist this engine does not know
		// the values of — the retired field among them — so a stored filter
		// naming one is never refused.
		{"a picklist with no options", Predicate{Field: "retired", Op: OpEq, Value: "anything"}, false},
		// Not this function's refusals: the compiler makes each of them a
		// moment later, in its own words. Answering here would give one
		// mistake two spellings.
		{"a field this engine does not have", Predicate{Field: "nope", Op: OpEq, Value: "x"}, false},
		{"a text field", Predicate{Field: "name", Op: OpEq, Value: "Anything At All"}, false},
		{"a number field", Predicate{Field: "amount", Op: OpEq, Value: 12}, false},
		{"a non-string operand", Predicate{Field: "status", Op: OpEq, Value: 12}, false},
		{"`in` carrying a scalar", Predicate{Field: "status", Op: OpIn, Value: "open"}, false},
		// The walk reaches every branch, at any depth: a tree refused only at
		// its root would let the typo through one `and` down.
		{"a stranger nested two levels deep", Predicate{And: []Predicate{{Or: []Predicate{
			{Field: "status", Op: OpEq, Value: "open"},
			{Field: "status", Op: OpEq, Value: "Lost"},
		}}}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := RefuseUnknownPicklistValues(tc.pred, picklistFields)
			if tc.refused != (err != nil) {
				t.Fatalf("err = %v, want refused=%v", err, tc.refused)
			}
		})
	}
}

// The refusal says which value, which field, and what the field admits — a
// caller correcting a typo needs the set, and it is already published to the
// same client through the field vocabulary.
func TestTheRefusalNamesTheStrangerAndTheSet(t *testing.T) {
	t.Parallel()
	err := RefuseUnknownPicklistValues(
		Predicate{Field: "status", Op: OpIn, Value: []any{"open", "Won", "lost"}}, picklistFields)
	var refusal *PredicateError
	if !errors.As(err, &refusal) {
		t.Fatalf("err = %v, want a PredicateError the transport maps to 422", err)
	}
	if refusal.Field != "status" || refusal.Code != "filter_value_invalid" {
		t.Errorf("refusal = %+v, want the field and filter_value_invalid", refusal)
	}
	// The FIRST stranger, not "one of these is wrong": a message that named the
	// clause without naming the member leaves a caller to find it themselves.
	if !strings.Contains(refusal.Message, `"Won"`) {
		t.Errorf("message = %q, want it to name the stranger", refusal.Message)
	}
	if !strings.Contains(refusal.Message, "open, won, lost") {
		t.Errorf("message = %q, want it to name what the field admits", refusal.Message)
	}
}

// The pairing this whole shape rests on, asserted on one tree so nothing can
// break it silently: the value a WRITE refuses is one an already-stored
// definition still evaluates.
//
// That is the decision, not an implementation detail. Making the compiler
// strict would have a saved view written last quarter begin failing the moment
// somebody removed a value from a custom field's option set.
func TestAValueTheWriteRefusesIsStillEvaluatedWhenAlreadyStored(t *testing.T) {
	t.Parallel()
	stored := Predicate{Field: "status", Op: OpEq, Value: "Open"}

	if err := RefuseUnknownPicklistValues(stored, picklistFields); err == nil {
		t.Fatal("the write accepted a value outside the set, so this test proves nothing about the pair")
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	if _, err := CompilePredicate(stored, picklistFields, arg); err != nil {
		t.Fatalf("the compiler refused a stored value the write would refuse: %v — evaluation must "+
			"stay permissive, or a stored filter starts failing when its field's options change", err)
	}
	if len(args) != 1 || args[0] != "Open" {
		t.Errorf("the value did not travel as a bind parameter: %#v", args)
	}
}
