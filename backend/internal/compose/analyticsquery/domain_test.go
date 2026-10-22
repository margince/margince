// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package analyticsquery

import (
	"errors"
	"maps"
	"testing"
)

func kindSchema() Schema {
	schema := testSchema()
	entity := schema.Entities["deals"]
	fields := maps.Clone(entity.Fields)
	fields["kind"] = Field{
		Name: "kind", Expr: "t.kind", Kind: KindDimension,
		Domain: ValueDomain{Values: []string{"meeting", "task"}, Why: "only meetings and tasks are counted"},
	}
	entity.Fields = fields
	schema.Entities["deals"] = entity
	return schema
}

func TestAFilterOutsideTheFieldsDomainIsRefusedOnEveryComparison(t *testing.T) {
	t.Parallel()
	for _, op := range []FilterOp{OpEq, OpNe, OpLt, OpGte} {
		_, err := Compile(Query{
			Entity:   "deals",
			Measures: []Measure{{Fn: CountAll, As: "n"}},
			Filters:  []Filter{{Field: "kind", Op: op, Value: "email"}},
		}, kindSchema(), noScope)
		var refusal *RefusalError
		if !errors.As(err, &refusal) || refusal.Kind != RefusalUnsupported || refusal.Suggest == "" {
			t.Errorf("kind %s email: err = %v, want an unsupported refusal with a suggestion", op, err)
		}
	}
}

func TestAFilterInsideTheDomainCompiles(t *testing.T) {
	t.Parallel()
	_, err := Compile(Query{
		Entity:   "deals",
		Measures: []Measure{{Fn: CountAll, As: "n"}},
		Filters:  []Filter{{Field: "kind", Op: OpEq, Value: "meeting"}, {Field: "kind", Op: OpIsNotNull}},
	}, kindSchema(), noScope)
	if err != nil {
		t.Fatalf("kind = meeting was refused: %v", err)
	}
}
