// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package analyticsquery

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestAReportAndItsQuestionNarrowToTheNarrowerPopulation(t *testing.T) {
	t.Parallel()
	team := Scope{Kind: "team", ID: "t-1"}
	cases := []struct {
		name             string
		report, question Scope
		want             Scope
	}{
		{"a question naming none takes the report's", team, Scope{}, team},
		{"a report naming none keeps the question's", Scope{}, team, team},
		{"a company report keeps the question's", Scope{Kind: "workspace"}, team, team},
		{"a company question takes the report's", team, Scope{Kind: "workspace"}, team},
		{"the same population stands", team, team, team},
	}
	entity := periodSchema().Entities["deals"]
	for _, c := range cases {
		q, err := Narrow(entity, c.report, Query{Entity: "deals", ScopeKind: c.question.Kind, ScopeID: c.question.ID})
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got := (Scope{Kind: q.ScopeKind, ID: q.ScopeID}); got != c.want {
			t.Errorf("%s: asked over %+v, want %+v", c.name, got, c.want)
		}
	}
}

// Two populations neither of which holds the other cannot both hold, and the
// question is refused rather than asked over either one.
func TestDisjointPopulationsAreRefusedNotWidened(t *testing.T) {
	t.Parallel()
	_, err := Narrow(periodSchema().Entities["deals"], Scope{Kind: "team", ID: "t-1"},
		Query{Entity: "deals", ScopeKind: "owner", ScopeID: "u-1"})
	var refusal *RefusalError
	if !errors.As(err, &refusal) || !strings.Contains(refusal.Message, "different team or person") {
		t.Fatalf("a team report asking one owner's question answered %v, want an explained refusal", err)
	}
}

func TestFiltersNoRecordCanMeetAreRefusedAndNoneIsDropped(t *testing.T) {
	t.Parallel()
	entity := periodSchema().Entities["deals"]
	refused := map[string][]Filter{
		"two values of one field": {
			{Field: "stage", Op: OpEq, Value: "won"}, {Field: "stage", Op: OpEq, Value: "lost"},
		},
		"periods that do not overlap": {
			{Field: "period_year", Op: OpEq, Value: "2026", Basis: &januaryBerlin},
			{Field: "period_quarter", Op: OpEq, Value: "2025-Q4", Basis: &januaryBerlin},
		},
		"a kept period that names none": {
			{Field: "period_year", Op: OpGte, Value: "FY2025/26", Basis: &januaryBerlin},
		},
	}
	for name, filters := range refused {
		_, err := Narrow(entity, Scope{}, Query{Entity: "deals", Filters: filters})
		var refusal *RefusalError
		if !errors.As(err, &refusal) || refusal.Kind != RefusalInvalid {
			t.Errorf("%s answered %v, want an invalid refusal", name, err)
		}
	}
	held := []Filter{
		{Field: "period_year", Op: OpEq, Value: "2026", Basis: &januaryBerlin},
		{Field: "period_quarter", Op: OpEq, Value: "2026-Q2", Basis: &januaryBerlin},
		{Field: "stage", Op: OpEq, Value: "won"},
	}
	q, err := Narrow(entity, Scope{Kind: "workspace"}, Query{Entity: "deals", Filters: held})
	if err != nil {
		t.Fatalf("filters that can all hold were refused: %v", err)
	}
	if !slices.EqualFunc(q.Filters, held, func(a, b Filter) bool { return a.Field == b.Field && a.Value == b.Value }) {
		t.Errorf("narrowing changed the filters to %+v", q.Filters)
	}
}
