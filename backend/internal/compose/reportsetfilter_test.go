// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"fmt"
	"maps"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A drill-through replays a set filter as the set, so the explanation opens
// every meeting its headline counted for that colleague.
func TestADrillThroughReplaysASetFilterAsTheSet(t *testing.T) {
	t.Parallel()
	spec := prebuiltReports["activities-by-kind"]
	plan, err := compileDerivation(spec, derivationQuery{
		Predicates: map[string]string{fieldHostUserID: ids.NewV7().String()},
	})
	if err != nil {
		t.Fatalf("compiling the handle: %v", err)
	}
	clause := clauseFor(t, spec, plan, fieldHostUserID)
	if want := fmt.Sprintf(spec.filterPredicates[fieldHostUserID], "$1"); clause != want {
		t.Errorf("the drill-through filters the colleague by\n  %s\nwant the headline's set predicate\n  %s", clause, want)
	}
}

// A breakdown cell on a field with no set predicate still matches by equality.
func TestADrillThroughPinsABreakdownCellByEquality(t *testing.T) {
	t.Parallel()
	spec := prebuiltReports["activities-by-kind"]
	plan, err := compileDerivation(spec, derivationQuery{
		Predicates: map[string]string{fieldKind: "meeting"},
		GroupBy:    []string{fieldKind},
	})
	if err != nil {
		t.Fatalf("compiling the handle: %v", err)
	}
	if clause, want := clauseFor(t, spec, plan, fieldKind), spec.dimensions[fieldKind]+" = $1"; clause != want {
		t.Errorf("the cell is pinned by %q, want %q", clause, want)
	}
}

func clauseFor(t *testing.T, spec reportSpec, plan derivationPlan, field string) string {
	t.Helper()
	for _, p := range plan.preds {
		if p.field != field {
			continue
		}
		clause, err := predicateClause(spec, p, func(any) int { return 1 })
		if err != nil {
			t.Fatalf("rendering %s: %v", field, err)
		}
		return clause
	}
	t.Fatalf("the plan holds no predicate on %s", field)
	return ""
}

// A field's set predicate is part of what an answer means, so two vocabularies
// that differ only there do not compare as one.
func TestTheVocabularyVersionMovesWithAFieldsSetPredicate(t *testing.T) {
	t.Parallel()
	schema := AnalyticsSchemaFor(seatWith("activity"))
	found := 0
	for name, entity := range schema.Entities {
		for field, def := range entity.Fields {
			if def.Predicate == "" {
				continue
			}
			found++
			stripped := maps.Clone(schema.Entities)
			fields := maps.Clone(entity.Fields)
			def.Predicate = ""
			fields[field] = def
			changed := entity
			changed.Fields = fields
			stripped[name] = changed
			if schemaVersion(stripped) == schema.Version {
				t.Errorf("%s.%s lost its set predicate and the version did not move", name, field)
			}
		}
	}
	if found == 0 {
		t.Fatal("no field in the activity vocabulary carries a set predicate, so this test checks nothing")
	}
}
