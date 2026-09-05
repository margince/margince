// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package analyticsquery

// What the compiler must never do, stated as tests.
//
// The golden cases below assert the rendered SQL character for character. That
// looks brittle and is the point: this is the one place in the tree where a
// string becomes a statement, and a change to it that nobody meant to make is
// exactly what these exist to stop.

import (
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// testSchema is a small vocabulary with one of everything the compiler
// distinguishes: a dimension, a numeric measure, and a text field that is
// neither.
// noScope is a caller whose authority narrows nothing — an unbounded reader.
//
// A FUNCTION rather than nil: Compile refuses nil, because "no scope source"
// and "nothing to narrow" look identical at a call site and only one of them
// is safe to render.
func noScope(func(any) int) ([]string, error) { return nil, nil }

func testSchema() Schema {
	return Schema{
		Version: "test_v1",
		Entities: map[string]Entity{
			"deals": {
				Name:      "deals",
				From:      "deal t",
				BaseWhere: "t.archived_at IS NULL",
				Fields: map[string]Field{
					"stage":  {Name: "stage", Expr: "t.stage_id", Kind: KindDimension},
					"owner":  {Name: "owner", Expr: "t.owner_id", Kind: KindDimension},
					"amount": {Name: "amount", Expr: "t.amount_minor", Kind: KindMeasure},
				},
			},
		},
	}
}

func TestACompiledQueryBindsEveryValueAndRendersNoCallerText(t *testing.T) {
	t.Parallel()
	plan, err := Compile(Query{
		Entity:   "deals",
		GroupBy:  []string{"stage"},
		Measures: []Measure{{Fn: Sum, Field: "amount", As: "total"}},
		Filters:  []Filter{{Field: "owner", Op: OpEq, Value: "u-1"}},
	}, testSchema(), noScope)
	if err != nil {
		t.Fatal(err)
	}

	const want = "SELECT t.stage_id, count(*), sum(t.amount_minor) FROM deal t" +
		" WHERE t.archived_at IS NULL AND t.owner_id = $1" +
		" GROUP BY 1 ORDER BY 1 LIMIT $2"
	if plan.SQL != want {
		t.Errorf("the rendered statement changed.\n got: %s\nwant: %s", plan.SQL, want)
	}
	// The filter's value is a BIND, not text in the statement.
	if strings.Contains(plan.SQL, "u-1") {
		t.Error("the filter value reached the statement; it must be a bind parameter")
	}
	if len(plan.Args) != 2 || plan.Args[0] != "u-1" {
		t.Errorf("args are %v; the value belongs at $1", plan.Args)
	}
	// The caller's alias never reaches SQL either — results map by position.
	if strings.Contains(plan.SQL, "total") {
		t.Error("the caller's alias reached the statement; results map by position")
	}
	if plan.Columns[len(plan.Columns)-1] != "total" {
		t.Errorf("columns are %v; the alias belongs on the last column", plan.Columns)
	}
}

func TestAFieldNameCarryingSQLIsRefusedBeforeAnythingIsRendered(t *testing.T) {
	t.Parallel()
	// Every one of these is a name a caller could send. None is in the schema,
	// so each is refused by LOOKUP rather than by pattern-matching — which is
	// what makes the defence complete rather than a list of known attacks.
	for _, name := range []string{
		"amount; DROP TABLE deal",
		"amount) FROM deal WHERE 1=1 --",
		"t.amount_minor",
		"*",
		"",
	} {
		queries := []Query{
			{
				Entity:   "deals",
				GroupBy:  []string{name},
				Measures: []Measure{{Fn: CountAll}},
			},
			{Entity: "deals", Measures: []Measure{{Fn: Sum, Field: name}}},
			{
				Entity:   "deals",
				Measures: []Measure{{Fn: CountAll}},
				Filters:  []Filter{{Field: name, Op: OpEq, Value: 1}},
			},
		}
		for _, q := range queries {
			plan, err := Compile(q, testSchema(), noScope)
			if err == nil {
				t.Errorf("a query naming %q compiled to: %s", name, plan.SQL)
				continue
			}
			var refusal *RefusalError
			if !errors.As(err, &refusal) {
				t.Errorf("naming %q answered %v, which is not a typed refusal", name, err)
			}
			if !errors.Is(err, apperrors.ErrInvalidArgument) {
				t.Errorf("naming %q does not map to an argument error", name)
			}
		}
	}
}

func TestARefusalNamesWhatWouldHaveWorked(t *testing.T) {
	t.Parallel()
	_, err := Compile(Query{
		Entity:   "deals",
		Measures: []Measure{{Fn: Sum, Field: "stage"}},
	}, testSchema(), noScope)

	var refusal *RefusalError
	if !errors.As(err, &refusal) {
		t.Fatalf("summing a dimension answered %v", err)
	}
	if refusal.Kind != RefusalInvalid {
		t.Errorf("summing a dimension is %q; it means nothing rather than being unbuilt", refusal.Kind)
	}
	// The clarification is the whole reason this type exists: a refusal a
	// caller cannot act on costs a round trip and teaches nothing.
	if !strings.Contains(refusal.Suggest, "amount") {
		t.Errorf("the refusal suggests %q, which does not name a measure that works", refusal.Suggest)
	}
}

func TestCountTakesNoFieldBecauseCountOfAColumnAsksSomethingElse(t *testing.T) {
	t.Parallel()
	_, err := Compile(Query{
		Entity:   "deals",
		Measures: []Measure{{Fn: CountAll, Field: "amount"}},
	}, testSchema(), noScope)

	var refusal *RefusalError
	if !errors.As(err, &refusal) {
		t.Fatalf("count over a field answered %v", err)
	}
	// count(amount) skips nulls, so answering it as count(*) would report an
	// unpriced deal as priced — the exact misreading the forecast's
	// eligible/priced pair exists to prevent.
	if !strings.Contains(refusal.Suggest, "count_distinct") {
		t.Errorf("the refusal suggests %q; count_distinct is the measure they meant", refusal.Suggest)
	}
}

func TestAnUnboundedQueryStillCarriesALimit(t *testing.T) {
	t.Parallel()
	plan, err := Compile(Query{
		Entity: "deals", GroupBy: []string{"owner"},
		Measures: []Measure{{Fn: CountAll}},
	}, testSchema(), noScope)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Args[len(plan.Args)-1] != defaultLimit {
		t.Errorf("a query with no limit bound %v; grouping by a high-cardinality column would return a row per record",
			plan.Args[len(plan.Args)-1])
	}
}

func TestEveryPlanCarriesTheCountTheFloorIsJudgedOn(t *testing.T) {
	t.Parallel()
	// The caller asked for a sum and nothing else. The floor still needs to
	// know how many rows each group covers — a floor that applied only when
	// somebody happened to request a count is one a caller turns off by not
	// asking.
	plan, err := Compile(Query{
		Entity: "deals", GroupBy: []string{"stage"},
		Measures: []Measure{{Fn: Sum, Field: "amount"}},
	}, testSchema(), noScope)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Columns[plan.CountColumn] != countRowsColumn {
		t.Fatalf("column %d is %q and should be the plan's own row count",
			plan.CountColumn, plan.Columns[plan.CountColumn])
	}
	if plan.CountColumn < plan.GroupCount {
		t.Error("the count sits among the group keys, so a key would be read as a count")
	}
}

func TestAnAliasCannotShadowTheEnginesOwnColumns(t *testing.T) {
	t.Parallel()
	// The exploit this closes: aliasing a measure to the privacy floor's own
	// input column made the floor judge a group by that measure's value. A
	// two-record group whose max amount was 100000 then read as a group of
	// 100000 and was served whole.
	for _, name := range ReservedColumns {
		_, err := Compile(Query{
			Entity:   "deals",
			GroupBy:  []string{"stage"},
			Measures: []Measure{{Fn: Max, Field: "amount", As: name}},
		}, testSchema(), noScope)
		if err == nil {
			t.Errorf("a measure aliased to %q compiled; that is the engine's own column", name)
			continue
		}
		var refusal *RefusalError
		if !errors.As(err, &refusal) || refusal.Kind != RefusalInvalid {
			t.Errorf("aliasing to %q answered %v rather than an invalid-query refusal", name, err)
		}
	}
}

func TestTwoResultColumnsCannotShareAName(t *testing.T) {
	t.Parallel()
	// Whichever the caller meant, one of the two numbers they asked for is
	// missing and nothing says which.
	for _, q := range []Query{
		// A measure over a group key's name.
		{
			Entity:   "deals",
			GroupBy:  []string{"stage"},
			Measures: []Measure{{Fn: CountAll, As: "stage"}},
		},
		// Two measures sharing an alias.
		{
			Entity: "deals",
			Measures: []Measure{
				{Fn: Sum, Field: "amount", As: "n"},
				{Fn: Avg, Field: "amount", As: "n"},
			},
		},
		// Two measures whose DEFAULT names collide.
		{
			Entity: "deals",
			Measures: []Measure{
				{Fn: Sum, Field: "amount"},
				{Fn: Sum, Field: "amount"},
			},
		},
	} {
		if _, err := Compile(q, testSchema(), noScope); err == nil {
			t.Errorf("a query with two columns of one name compiled: %+v", q)
		}
	}
}

func TestAQueryCannotBeCompiledWithoutAScopeSource(t *testing.T) {
	t.Parallel()
	// Nil and "nothing to narrow" look identical at a call site, and the
	// difference is whether a caller reads rows they may not. A compiler that
	// treated nil as unbounded is one refactor away from rendering an
	// ungated statement.
	if _, err := Compile(Query{
		Entity: "deals", Measures: []Measure{{Fn: CountAll}},
	}, testSchema(), nil); err == nil {
		t.Fatal("a query compiled with no scope source, which renders an ungated statement")
	}
}

func TestTheCallersAuthorityReachesTheStatement(t *testing.T) {
	t.Parallel()
	narrowed := func(func(any) int) ([]string, error) {
		return []string{"t.owner_id = 'me'"}, nil
	}
	plan, err := Compile(Query{
		Entity: "deals", Measures: []Measure{{Fn: CountAll}},
	}, testSchema(), narrowed)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.SQL, "t.owner_id = 'me'") {
		t.Errorf("the caller's own narrowing is absent from:\n%s", plan.SQL)
	}
}

// A percentile renders under the sample floor: below five values the answer
// is NULL, not a number — a median over three deals is one deal's value
// wearing a statistic's name. The same refusal the report engine writes, at
// the same threshold, so a screen and a tool cannot disagree about one week.
func TestAPercentileAnswersNullBelowTheSampleFloor(t *testing.T) {
	t.Parallel()
	plan, err := Compile(Query{
		Entity: "deals",
		Measures: []Measure{
			{Fn: Median, Field: "amount", As: "typical"},
			{Fn: P75, Field: "amount", As: "upper"},
		},
	}, testSchema(), noScope)
	if err != nil {
		t.Fatal(err)
	}
	wantMedian := "(CASE WHEN count(t.amount_minor) >= 5 THEN " +
		"percentile_cont(0.5) WITHIN GROUP (ORDER BY t.amount_minor) END)"
	if !strings.Contains(plan.SQL, wantMedian) {
		t.Errorf("median renders without the floor:\n got: %s\nwant it to contain: %s",
			plan.SQL, wantMedian)
	}
	if !strings.Contains(plan.SQL, "percentile_cont(0.75)") {
		t.Errorf("p75 does not ask for the 75th percentile: %s", plan.SQL)
	}
}

// A percentile over a non-numeric field is refused before rendering: the 50th
// percentile of a stage id means nothing, and Postgres computing it anyway is
// exactly why the refusal is written here.
func TestAPercentileOverANonNumericFieldIsRefused(t *testing.T) {
	t.Parallel()
	_, err := Compile(Query{
		Entity:   "deals",
		Measures: []Measure{{Fn: Median, Field: "stage"}},
	}, testSchema(), noScope)
	if err == nil {
		t.Fatal("median over a dimension compiled; it must be refused by name")
	}
}
