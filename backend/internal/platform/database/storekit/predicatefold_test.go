// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// fixtureID is any well-formed id; an id leaf compiles the same for all.
const fixtureID = "0190a9b8-0000-7000-8000-000000000002"

// compileCollecting compiles p and answers the SQL with every bound value.
func compileCollecting(t *testing.T, p Predicate, fields map[string]Field) (string, []any, error) {
	t.Helper()
	var args []any
	sql, err := CompilePredicate(p, fields, func(v any) int { args = append(args, v); return len(args) })
	return sql, args, err
}

// A text condition ignores capitals, surrounding spaces and accents whichever
// operator asks, so "cto" finds "CTO" and " Muller " finds "Müller". Both sides
// fold in SQL, where list search's f_unaccent lives.
func TestEveryTextOperatorComparesUnderOneRule(t *testing.T) {
	fields := map[string]Field{
		"email": {Expr: "t.email", Type: FieldText, FoldCase: true},
		"title": {Expr: "t.title", Type: FieldText},
	}
	const column, operand = "f_fold_apostrophes(lower(t.title))", "f_fold_apostrophes(lower($1::text))"
	for _, c := range []struct {
		p       Predicate
		wantSQL string
		want    []any
	}{
		{Predicate{Field: "title", Op: OpEq, Value: " cto "}, column + " = " + operand, []any{"cto"}},
		{Predicate{Field: "title", Op: OpNeq, Value: "CTO"}, column + " IS DISTINCT FROM " + operand, []any{"CTO"}},
		{Predicate{Field: "title", Op: OpContains, Value: " Muller "}, column + " LIKE " + operand, []any{"%Muller%"}},
		{
			Predicate{Field: "title", Op: OpIn, Value: []any{" cto", "Cfo "}},
			column + " = ANY(ARRAY(SELECT f_fold_apostrophes(lower(v)) FROM unnest($1::text[]) AS v))",
			[]any{[]string{"cto", "Cfo"}},
		},
		{
			Predicate{Field: "email", Op: OpEq, Value: "Ann@X.Example"},
			"f_fold_apostrophes(lower(t.email)) = " + operand,
			[]any{"ann@x.example"},
		},
	} {
		sql, args, err := compileCollecting(t, c.p, fields)
		if err != nil {
			t.Fatalf("%s %s: %v", c.p.Field, c.p.Op, err)
		}
		if sql != c.wantSQL || !reflect.DeepEqual(args, c.want) {
			t.Errorf("%s %s %v compiled to %q %#v, want %q %#v", c.p.Field, c.p.Op, c.p.Value, sql, args, c.wantSQL, c.want)
		}
	}
	if _, _, err := compileCollecting(t, Predicate{Field: "title", Op: OpContains, Value: "   "}, fields); err == nil {
		t.Error("contains took a value of spaces alone, which matches every row")
	}
}

// A picklist holds option keys, compared as stored, and a field folded for
// case alone still lowers only.
func TestOnlyFreeTextFoldsAccents(t *testing.T) {
	fields := map[string]Field{
		"status": {Expr: "t.status", Type: FieldPicklist},
		"key":    {Expr: "t.key", Type: FieldCompanyKey, FoldCase: true},
	}
	sql, args, err := compileCollecting(t, Predicate{Field: "status", Op: OpEq, Value: "Open"}, fields)
	if err != nil || sql != "t.status = $1" || !reflect.DeepEqual(args, []any{"Open"}) {
		t.Errorf("picklist eq compiled to %q %#v (%v), want the key compared as stored", sql, args, err)
	}
	sql, _, err = compileCollecting(t, Predicate{Field: "key", Op: OpContains, Value: "acme"}, fields)
	if err != nil || strings.Contains(sql, "f_fold_apostrophes") {
		t.Errorf("company key contains compiled to %q (%v), want it unchanged", sql, err)
	}
}

// A text field reached through a link compares under the same rule inside its
// subquery.
func TestALinkedTextFieldComparesUnderTheTextRule(t *testing.T) {
	fields := map[string]Field{
		"technology": {Expr: "f.value", Type: FieldText, Link: "EXISTS (SELECT 1 FROM fact f WHERE f.id = t.id AND %s)"},
	}
	sql, args, err := compileCollecting(t, Predicate{Field: "technology", Op: OpEq, Value: " WordPress "}, fields)
	want := "EXISTS (SELECT 1 FROM fact f WHERE f.id = t.id AND f_fold_apostrophes(lower(f.value)) = f_fold_apostrophes(lower($1::text)))"
	if err != nil || sql != want || !reflect.DeepEqual(args, []any{"WordPress"}) {
		t.Errorf("linked eq compiled to %q %#v (%v), want %q", sql, args, err, want)
	}
}

func TestADayOnATimestampCompilesToBoundsOnTheRawColumn(t *testing.T) {
	fields := map[string]Field{"seen": {Expr: "t.seen_at", Type: FieldDate, Instant: true}}
	start, next := occurredday.DayStartSQL("$1::date"), occurredday.DayStartSQL("$1::date + 1")
	for op, want := range map[string]string{
		OpEq:  "(t.seen_at >= " + start + " AND t.seen_at < " + next + ")",
		OpNeq: "(t.seen_at IS NULL OR t.seen_at < " + start + " OR t.seen_at >= " + next + ")",
		OpGt:  "t.seen_at >= " + next,
		OpGte: "t.seen_at >= " + start,
		OpLt:  "t.seen_at < " + start,
		OpLte: "t.seen_at < " + next,
	} {
		sql, args, err := compileCollecting(t, Predicate{Field: "seen", Op: op, Value: "2026-03-10"}, fields)
		if err != nil || sql != want || !reflect.DeepEqual(args, []any{"2026-03-10"}) {
			t.Errorf("%s compiled to %q %v (%v), want %q over one bound day", op, sql, args, err, want)
		}
	}
	sql, _, err := compileCollecting(t, Predicate{Field: "seen", Op: OpLt, Value: map[string]any{"days_ago": 45.0}}, fields)
	if err != nil || sql != "t.seen_at < "+occurredday.DayStartSQL("("+occurredday.TodaySQL("now()")+" - $1::integer)") {
		t.Errorf("a relative day compiled to %q (%v), want the start of that day", sql, err)
	}
}

func TestALinkScopeBoundsTheRowsTheLinkMayFind(t *testing.T) {
	scoped := Field{
		Expr: "r.company_id", Type: FieldID,
		Link: "EXISTS (SELECT 1 FROM edge r WHERE r.contact_id = t.id AND %s)",
		LinkScope: func(arg func(any) int) (string, error) {
			return SQLf("r.owner_id = $%d", arg("viewer")), nil
		},
	}
	fields := map[string]Field{"company_id": scoped}
	sql, args, err := compileCollecting(t, Predicate{Field: "company_id", Op: OpNeq, Value: fixtureID}, fields)
	if err != nil {
		t.Fatal(err)
	}
	want := "NOT EXISTS (SELECT 1 FROM edge r WHERE r.contact_id = t.id AND (r.company_id = $1) AND r.owner_id = $2)"
	if sql != want || !reflect.DeepEqual(args, []any{fixtureID, "viewer"}) {
		t.Errorf("compiled %q with %v, want %q with the operand then the scope's value", sql, args, want)
	}

	refused := errors.New("edge read refused")
	scoped.LinkScope = func(func(any) int) (string, error) { return "", refused }
	if _, _, err := compileCollecting(t, Predicate{Field: "company_id", Op: OpEq, Value: fixtureID},
		map[string]Field{"company_id": scoped}); !errors.Is(err, refused) {
		t.Errorf("a failing scope compiled with err=%v, want its own error rather than an unbounded link", err)
	}
}

func TestAWithheldLinkRefusesTheOperatorsItsVocabularyOmits(t *testing.T) {
	fields := map[string]Field{"industry": {
		Expr: "o.industry", Type: FieldText, Withheld: true,
		Link: "EXISTS (SELECT 1 FROM company o WHERE o.id = t.company_id AND %s)",
	}}
	_, _, err := compileCollecting(t, Predicate{Field: "industry", Op: OpContains, Value: "x"}, fields)
	var pe *PredicateError
	if !errors.As(err, &pe) || pe.Code != CodeFilterOpNotAllowed {
		t.Errorf("contains on a withheld link answered %v, want the operator refused as it is when not withheld", err)
	}
	if sql, _, err := compileCollecting(t, Predicate{Field: "industry", Op: OpEq, Value: "x"}, fields); err != nil || sql != "FALSE" {
		t.Errorf("eq on a withheld link compiled to %q, %v; want FALSE", sql, err)
	}
}
