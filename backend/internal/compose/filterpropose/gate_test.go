// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package filterpropose

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// vocabulary is a contact-like field list, as the vocabulary read answers it.
// tag is a link field: its type admits `contains`, and the vocabulary leaves it
// out, which is what tells a vocabulary check from the type matrix.
var vocabulary = []Field{
	{Name: "country", Type: "text", Operators: []string{"eq", "neq", "in", "contains", "exists"}},
	{Name: "tag", Type: "text", Operators: []string{"eq", "neq", "in", "exists"}},
	{
		Name: "lifecycle", Type: "picklist", Operators: []string{"eq", "neq", "in", "exists"},
		Options: []string{"prospect", "customer", "churned"},
	},
	{Name: "last_activity_at", Type: "date", Operators: []string{"eq", "neq", "gt", "gte", "lt", "lte", "exists"}},
	{Name: "score", Type: "number", Operators: []string{"eq", "gt", "gte", "lt", "lte", "in", "exists"}},
	{Name: "amount", Type: "currency", Operators: []string{"gt", "gte", "lt", "lte"}, Currency: "EUR"},
	{Name: "cf_budget_jpy", Type: "currency", Operators: []string{"gt"}, Currency: "JPY", Custom: true},
	{Name: "cf_unpriced", Type: "currency", Operators: []string{"gt"}, Custom: true},
}

func leafOf(t *testing.T, tree *storekit.Predicate) storekit.Predicate {
	t.Helper()
	if tree == nil || len(tree.And) != 1 {
		t.Fatalf("want one clause under an and root, got %+v", tree)
	}
	return tree.And[0]
}

func oneClause(c Clause) Answer {
	return Answer{Join: joinAnd, Groups: []Group{{Join: joinAnd, Clauses: []Clause{c}}}}
}

func TestAClauseTheVocabularyCannotExpressIsNamedBackAndTheRestSurvive(t *testing.T) {
	cases := map[string]struct {
		clause Clause
		code   string
	}{
		"a field the caller cannot filter on": {
			Clause{Phrase: "who are likely to buy", Field: "purchase_intent", Op: "eq", Text: new("high")}, CodeUnknownField,
		},
		"an operator the vocabulary leaves out although the type admits it": {
			Clause{Phrase: "tagged with vip", Field: "tag", Op: "contains", Text: new("vip")}, CodeOperatorNotAllowed,
		},
		"a picklist value outside its options": {
			Clause{Phrase: "hot prospects", Field: "lifecycle", Op: "eq", Text: new("hot prospect")}, CodeValueNotAllowed,
		},
		"a value of the wrong type": {
			Clause{Phrase: "score above high", Field: "score", Op: "gt", Text: new("high")}, CodeValueNotAllowed,
		},
		"a date the compiler cannot read": {
			Clause{Phrase: "active since yesterday-ish", Field: "last_activity_at", Op: "gte", Text: new("recently")}, CodeValueNotAllowed,
		},
		"an amount in a currency nobody named": {
			Clause{Phrase: "budget over 10", Field: "cf_unpriced", Op: "gt", Number: new(float64(10))}, CodeValueNotAllowed,
		},
		"exists with no flag": {
			Clause{Phrase: "has a country", Field: "country", Op: "exists"}, CodeValueNotAllowed,
		},
	}
	kept := Clause{Phrase: "in Germany", Field: "country", Op: "eq", Text: new("DE")}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := Gate(Answer{Join: joinAnd, Groups: []Group{{Join: joinAnd, Clauses: []Clause{kept, tc.clause}}}}, vocabulary)
			if len(got.Unsupported) != 1 || got.Unsupported[0].Code != tc.code {
				t.Fatalf("want one %s, got %+v", tc.code, got.Unsupported)
			}
			if got.Unsupported[0].Phrase != tc.clause.Phrase || got.Unsupported[0].Field != tc.clause.Field {
				t.Errorf("the drop names %+v, want the phrase and field it was read from", got.Unsupported[0])
			}
			want := storekit.Predicate{Field: "country", Op: "eq", Value: "DE"}
			if leaf := leafOf(t, got.Tree); !reflect.DeepEqual(leaf, want) {
				t.Errorf("the good clause became %+v, want %+v", leaf, want)
			}
		})
	}
}

func TestAPicklistValueIsAnsweredAsTheOptionSpellsIt(t *testing.T) {
	got := Gate(oneClause(Clause{Field: "lifecycle", Op: "in", List: []string{"Customer", " churned"}}), vocabulary)
	want := storekit.Predicate{Field: "lifecycle", Op: "in", Value: []any{"customer", "churned"}}
	if leaf := leafOf(t, got.Tree); !reflect.DeepEqual(leaf, want) {
		t.Errorf("got %+v, want %+v", leaf, want)
	}
}

func TestOperandsBecomeTheEngineOwnShapes(t *testing.T) {
	cases := map[string]struct {
		clause Clause
		want   storekit.Predicate
	}{
		"a relative date": {
			Clause{Field: "last_activity_at", Op: "lt", DaysAgo: new(45)},
			storekit.Predicate{Field: "last_activity_at", Op: "lt", Value: map[string]any{"days_ago": 45.0}},
		},
		"a fixed date": {
			Clause{Field: "last_activity_at", Op: "gte", Text: new("2026-09-01")},
			storekit.Predicate{Field: "last_activity_at", Op: "gte", Value: "2026-09-01"},
		},
		"euros in major units": {
			Clause{Field: "amount", Op: "gt", Number: new(float64(50000))},
			storekit.Predicate{Field: "amount", Op: "gt", Value: 5000000.0},
		},
		"yen, which have no minor unit": {
			Clause{Field: "cf_budget_jpy", Op: "gt", Number: new(float64(50000))},
			storekit.Predicate{Field: "cf_budget_jpy", Op: "gt", Value: 50000.0},
		},
		"a numeric in list": {
			Clause{Field: "score", Op: "in", List: []string{"10", "20"}},
			storekit.Predicate{Field: "score", Op: "in", Value: []any{10.0, 20.0}},
		},
		"an empty-field question": {
			Clause{Field: "last_activity_at", Op: "exists", Flag: new(false)},
			storekit.Predicate{Field: "last_activity_at", Op: "exists", Value: false},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := Gate(oneClause(tc.clause), vocabulary)
			if len(got.Unsupported) != 0 {
				t.Fatalf("dropped: %+v", got.Unsupported)
			}
			if leaf := leafOf(t, got.Tree); !reflect.DeepEqual(leaf, tc.want) {
				t.Errorf("got %#v, want %#v", leaf, tc.want)
			}
		})
	}
}

func TestThePhrasesTheModelCouldNotUseAreKeptAsItsOwn(t *testing.T) {
	answer := Answer{
		Join: joinAnd, Groups: []Group{},
		Unsupported: []Unsupported{{Phrase: "likely to buy", Reason: "No field records buying intent."}},
	}
	got := Gate(answer, vocabulary)
	if got.Tree != nil {
		t.Errorf("a sentence nothing could express proposed %+v", got.Tree)
	}
	want := []Dropped{{Phrase: "likely to buy", Code: CodeNotExpressible, Reason: "No field records buying intent."}}
	if !reflect.DeepEqual(got.Unsupported, want) {
		t.Errorf("got %+v, want %+v", got.Unsupported, want)
	}
}

func TestTheTreeHasAGroupAtItsRootAndCompiles(t *testing.T) {
	germany := Clause{Field: "country", Op: "eq", Text: new("DE")}
	austria := Clause{Field: "country", Op: "eq", Text: new("AT")}
	quiet := Clause{Field: "last_activity_at", Op: "lt", DaysAgo: new(45)}
	answer := Answer{Join: joinAnd, Groups: []Group{
		{Join: joinOr, Clauses: []Clause{germany, austria}},
		{Join: joinAnd, Clauses: []Clause{quiet}},
	}}
	got := Gate(answer, vocabulary)
	encoded, err := json.Marshal(got.Tree)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"and":[{"or":[{"field":"country","op":"eq","value":"DE"},{"field":"country","op":"eq","value":"AT"}]},` +
		`{"field":"last_activity_at","op":"lt","value":{"days_ago":45}}]}`
	if string(encoded) != want {
		t.Errorf("got %s\nwant %s", encoded, want)
	}
	fields := map[string]storekit.Field{
		"country":          {Expr: "t.country", Type: storekit.FieldText},
		"last_activity_at": {Expr: "t.last_activity_at", Type: storekit.FieldDate},
	}
	if _, err := storekit.CompilePredicate(*got.Tree, fields, func(any) int { return 1 }); err != nil {
		t.Errorf("the proposed tree does not compile: %v", err)
	}

	single := Gate(oneClause(germany), vocabulary)
	if single.Tree == nil || len(single.Tree.And) != 1 {
		t.Errorf("a lone clause is not wrapped in a root group: %+v", single.Tree)
	}
	lone := Gate(Answer{Join: joinAnd, Groups: answer.Groups[:1]}, vocabulary)
	if lone.Tree == nil || len(lone.Tree.Or) != 2 {
		t.Errorf("a lone group is not the root: %+v", lone.Tree)
	}
}

func TestAClausePastTheEngineLimitIsNamedRatherThanFailingTheTree(t *testing.T) {
	clauses := make([]Clause, storekit.PredicateMaxLeaves+1)
	for i := range clauses {
		clauses[i] = Clause{Field: "score", Op: "gt", Number: new(float64(i))}
	}
	got := Gate(Answer{Join: joinAnd, Groups: []Group{{Join: joinAnd, Clauses: clauses}}}, vocabulary)
	if len(got.Unsupported) != 1 || got.Unsupported[0].Code != CodeTooManyConditions {
		t.Fatalf("want one too_many_conditions, got %+v", got.Unsupported)
	}
	if got.Tree == nil || len(got.Tree.And) != storekit.PredicateMaxLeaves {
		t.Errorf("want %d clauses kept, got %+v", storekit.PredicateMaxLeaves, got.Tree)
	}
}

func TestAPicklistValueMatchesExactlyBeforeItFoldsCase(t *testing.T) {
	regions := []Field{{
		Name: "cf_region", Type: "picklist", Operators: []string{"eq"},
		Options: []string{"US", "us", "EU"}, Custom: true,
	}}
	cases := map[string]struct {
		value string
		want  string
		code  string
	}{
		"an exact value is kept as written":        {value: "us", want: "us"},
		"an unambiguous case variant is respelled": {value: "eu", want: "EU"},
		"an ambiguous case variant is refused":     {value: "Us", code: CodeValueNotAllowed},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := Gate(oneClause(Clause{Field: "cf_region", Op: "eq", Text: new(tc.value)}), regions)
			if tc.code != "" {
				if len(got.Unsupported) != 1 || got.Unsupported[0].Code != tc.code {
					t.Fatalf("want one %s, got %+v", tc.code, got.Unsupported)
				}
				return
			}
			want := storekit.Predicate{Field: "cf_region", Op: "eq", Value: tc.want}
			if leaf := leafOf(t, got.Tree); !reflect.DeepEqual(leaf, want) {
				t.Errorf("got %+v, want %+v", leaf, want)
			}
		})
	}
}

// A reader without custom_field:read is sent a custom picklist with no options.
// Any value would then pass unchecked and could match nothing, so it is declined.
func TestAPicklistValueWhoseOptionsAreWithheldIsDeclined(t *testing.T) {
	withheld := []Field{{Name: "cf_tier", Type: "picklist", Operators: []string{"eq", "in", "exists"}, Custom: true}}
	for name, clause := range map[string]Clause{
		"eq": {Phrase: "gold tier", Field: "cf_tier", Op: "eq", Text: new("gold")},
		"in": {Phrase: "gold or silver", Field: "cf_tier", Op: "in", List: []string{"gold", "silver"}},
	} {
		t.Run(name, func(t *testing.T) {
			got := Gate(oneClause(clause), withheld)
			if got.Tree != nil || len(got.Unsupported) != 1 || got.Unsupported[0].Code != CodeValueNotVerifiable {
				t.Errorf("want the clause declined as value_not_verifiable, got tree %+v, unsupported %+v", got.Tree, got.Unsupported)
			}
		})
	}
	// exists asks nothing of the options, so it still stands.
	got := Gate(oneClause(Clause{Field: "cf_tier", Op: "exists", Flag: new(true)}), withheld)
	if got.Tree == nil || len(got.Unsupported) != 0 {
		t.Errorf("an exists clause on a withheld picklist was refused: %+v", got.Unsupported)
	}
}

func TestARelativeDateIsTakenOnlyAsABoundCountingBack(t *testing.T) {
	for _, op := range []string{"gt", "gte", "lt", "lte"} {
		got := Gate(oneClause(Clause{Field: "last_activity_at", Op: op, DaysAgo: new(0)}), vocabulary)
		if len(got.Unsupported) != 0 {
			t.Errorf("%s days_ago 0 was refused: %+v", op, got.Unsupported)
		}
	}
	cases := map[string]Clause{
		"eq":       {Phrase: "exactly 45 days ago", Field: "last_activity_at", Op: "eq", DaysAgo: new(45)},
		"neq":      {Phrase: "not 45 days ago", Field: "last_activity_at", Op: "neq", DaysAgo: new(45)},
		"negative": {Phrase: "in 3 days", Field: "last_activity_at", Op: "lt", DaysAgo: new(-3)},
	}
	for name, clause := range cases {
		t.Run(name, func(t *testing.T) {
			got := Gate(oneClause(clause), vocabulary)
			if got.Tree != nil || len(got.Unsupported) != 1 || got.Unsupported[0].Code != CodeValueNotAllowed {
				t.Fatalf("want the clause named back as value_not_allowed, got tree %+v unsupported %+v", got.Tree, got.Unsupported)
			}
			if name == "negative" && !strings.Contains(got.Unsupported[0].Reason, "counts back from today") {
				t.Errorf("a date in the future is named back as %q, not as one counting back from today", got.Unsupported[0].Reason)
			}
		})
	}
}
