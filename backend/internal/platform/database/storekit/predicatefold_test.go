// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"errors"
	"reflect"
	"strings"
	"testing"
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

func TestAFoldedFieldBindsItsOperandLowercasedAndAnUnfoldedOneAsTyped(t *testing.T) {
	fields := map[string]Field{
		"email": {Expr: "t.email", Type: FieldText, FoldCase: true},
		"name":  {Expr: "t.name", Type: FieldText},
	}
	for _, c := range []struct {
		p    Predicate
		want []any
	}{
		{Predicate{Field: "email", Op: OpEq, Value: "Ann@X.Example"}, []any{"ann@x.example"}},
		{Predicate{Field: "email", Op: OpIn, Value: []any{"A@B.C", "d@e.f"}}, []any{[]string{"a@b.c", "d@e.f"}}},
		{Predicate{Field: "name", Op: OpEq, Value: "Ann"}, []any{"Ann"}},
	} {
		sql, args, err := compileCollecting(t, c.p, fields)
		if err != nil {
			t.Fatalf("%s %s: %v", c.p.Field, c.p.Op, err)
		}
		if !reflect.DeepEqual(args, c.want) {
			t.Errorf("%s %s bound %#v, want %#v", c.p.Field, c.p.Op, args, c.want)
		}
		folded := strings.HasPrefix(sql, "lower(t.email)")
		if folded != fields[c.p.Field].FoldCase {
			t.Errorf("%s %s compiled to %q: a folded field lowers the column, an unfolded one does not", c.p.Field, c.p.Op, sql)
		}
	}
}

func TestADayOnATimestampCompilesToBoundsOnTheRawColumn(t *testing.T) {
	fields := map[string]Field{"seen": {Expr: "t.seen_at", Type: FieldDate, Instant: true}}
	const start, next = "($1::date)::timestamptz", "($1::date + 1)::timestamptz"
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
	if err != nil || sql != "t.seen_at < ((CURRENT_DATE - $1::integer))::timestamptz" {
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
