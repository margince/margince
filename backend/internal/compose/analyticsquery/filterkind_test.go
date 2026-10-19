// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package analyticsquery

// A filter-only field narrows and does nothing else.
//
// The report specs carry filters that are thresholds rather than columns.
// Grouping on one splits every row into two buckets, whose names answer no
// question a reader asked.

import (
	"errors"
	"maps"
	"strings"
	"testing"
)

// stalledSchema is testSchema plus the filter kind, typed the way the seam
// types a boolean-valued filter expression.
func stalledSchema() Schema {
	schema := testSchema()
	entity := schema.Entities["deals"]
	fields := maps.Clone(entity.Fields)
	fields["stalled"] = Field{
		Name: "stalled", Expr: "(t.stage_entered_at < now() - interval '30 days')",
		Kind: KindFilter, Shape: ShapeBoolean,
	}
	entity.Fields = fields
	schema.Entities["deals"] = entity
	return schema
}

func TestAFilterOnlyFieldNarrowsAQuery(t *testing.T) {
	t.Parallel()
	plan, err := Compile(Query{
		Entity:   "deals",
		GroupBy:  []string{"stage"},
		Measures: []Measure{{Fn: CountAll, As: "deals"}},
		Filters:  []Filter{{Field: "stalled", Op: OpEq, Value: true}},
	}, stalledSchema(), noScope)
	if err != nil {
		t.Fatalf("filtering on a filter-only field was refused: %v", err)
	}
	if !strings.Contains(plan.SQL, "stage_entered_at") {
		t.Errorf("the filter's own expression is not in the plan:\n%s", plan.SQL)
	}
	// Bound and not rendered. The args carry the plan's own row limit beside
	// the caller's value, so this asks by position rather than by count.
	if !strings.Contains(plan.SQL, "$1") || len(plan.Args) == 0 || plan.Args[0] != true {
		t.Errorf("the value did not bind as the first parameter: args=%v\n%s", plan.Args, plan.SQL)
	}
}

// Refused by kind and not as unknown. The two send a caller to different
// places. One says the name does not exist. This one says the name exists and
// cannot be grouped on, naming the ones that can.
func TestAFilterOnlyFieldCannotBeGroupedOn(t *testing.T) {
	t.Parallel()
	_, err := Compile(Query{
		Entity:   "deals",
		GroupBy:  []string{"stalled"},
		Measures: []Measure{{Fn: CountAll, As: "deals"}},
	}, stalledSchema(), noScope)
	if err == nil {
		t.Fatal("grouping by a filter-only field was allowed")
	}
	var refusal *RefusalError
	if !errors.As(err, &refusal) {
		t.Fatalf("err = %v, want a RefusalError the tool surface can classify", err)
	}
	if refusal.Kind != RefusalInvalid {
		t.Errorf("refusal kind = %q, want %q: the name exists, so this is not an unsupported name", refusal.Kind, RefusalInvalid)
	}
	if !strings.Contains(refusal.Suggest, "stage") {
		t.Errorf("the refusal suggests %q, which names no dimension the caller could group on instead", refusal.Suggest)
	}
}

// An unknown filter name is answered with the whole filterable vocabulary.
// Suggesting the dimensions alone hid the filters that exist.
func TestAnUnknownFilterNameSuggestsEveryFilterableField(t *testing.T) {
	t.Parallel()
	_, err := Compile(Query{
		Entity:   "deals",
		GroupBy:  []string{"stage"},
		Measures: []Measure{{Fn: CountAll, As: "deals"}},
		Filters:  []Filter{{Field: "stuck", Op: OpEq, Value: true}},
	}, stalledSchema(), noScope)
	if err == nil {
		t.Fatal("a filter on a name the schema does not carry was allowed")
	}
	var refusal *RefusalError
	if !errors.As(err, &refusal) {
		t.Fatalf("err = %v, want a RefusalError", err)
	}
	for _, want := range []string{"stalled", "amount", "stage"} {
		if !strings.Contains(refusal.Suggest, want) {
			t.Errorf("the suggestion omits %q, which this caller may filter on: %q", want, refusal.Suggest)
		}
	}
}

// A custom field the vocabulary lacks was taken out of reports or retired, so
// the refusal says how to repair the question, as a grouping and as a filter.
func TestACustomFieldOutsideReportsIsRefusedWithItsRepair(t *testing.T) {
	t.Parallel()
	for name, q := range map[string]Query{
		"group":  {Entity: "deals", GroupBy: []string{"cf_territory"}, Measures: []Measure{{Fn: CountAll}}},
		"filter": {Entity: "deals", Measures: []Measure{{Fn: CountAll}}, Filters: []Filter{{Field: "cf_territory", Op: OpEq, Value: "EMEA"}}},
	} {
		_, err := Compile(q, stalledSchema(), noScope)
		refusal, ok := errors.AsType[*RefusalError](err)
		if !ok {
			t.Fatalf("%s: err = %v, want a RefusalError", name, err)
		}
		if !strings.Contains(refusal.Message, "not in reports") || !strings.Contains(refusal.Suggest, "In reports") {
			t.Errorf("%s: refused with %q / %q, which names no repair", name, refusal.Message, refusal.Suggest)
		}
	}
}
