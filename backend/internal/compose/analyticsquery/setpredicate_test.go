// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package analyticsquery

import (
	"errors"
	"maps"
	"strings"
	"testing"
)

const seatSetPredicate = "EXISTS (SELECT 1 FROM credit WHERE credit.id = %s)"

func seatSetSchema() Schema {
	schema := testSchema()
	entity := schema.Entities["deals"]
	fields := maps.Clone(entity.Fields)
	fields["seat"] = Field{Name: "seat", Expr: "t.seat", Kind: KindDimension, Predicate: seatSetPredicate}
	entity.Fields = fields
	schema.Entities["deals"] = entity
	return schema
}

// A field matched against a set asks whether the set holds the value, in
// place of comparing its listing expression.
func TestAFilterOnASetAsksTheSet(t *testing.T) {
	t.Parallel()
	plan, err := Compile(Query{
		Entity:   "deals",
		Measures: []Measure{{Fn: CountAll, As: "n"}},
		Filters:  []Filter{{Field: "seat", Op: OpEq, Value: "u1"}},
	}, seatSetSchema(), noScope)
	if err != nil {
		t.Fatalf("seat = u1 was refused: %v", err)
	}
	if !strings.Contains(plan.SQL, "EXISTS (SELECT 1 FROM credit WHERE credit.id = $") {
		t.Errorf("the filter does not ask the set:\n%s", plan.SQL)
	}
	if strings.Contains(plan.SQL, "t.seat =") {
		t.Errorf("the filter compares the listing seat:\n%s", plan.SQL)
	}
}

// A set holds a value or it does not, so any other comparison is refused
// with the one that means something.
func TestAnOrderedComparisonOnASetIsRefused(t *testing.T) {
	t.Parallel()
	for _, op := range []FilterOp{OpNe, OpLt, OpGte} {
		_, err := Compile(Query{
			Entity:   "deals",
			Measures: []Measure{{Fn: CountAll, As: "n"}},
			Filters:  []Filter{{Field: "seat", Op: op, Value: "u1"}},
		}, seatSetSchema(), noScope)
		var refusal *RefusalError
		if !errors.As(err, &refusal) || refusal.Kind != RefusalUnsupported || !strings.Contains(refusal.Suggest, string(OpEq)) {
			t.Errorf("seat %s u1: err = %v, want an unsupported refusal suggesting %s", op, err, OpEq)
		}
	}
}
