// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import "fmt"

// comparedExpr is the side of a scalar comparison the column sits on: the
// column itself, or its lowercase for a field compared case-insensitively.
func comparedExpr(field Field) string {
	if field.FoldCase {
		return "lower(" + field.Expr + ")"
	}
	return field.Expr
}

// compileInstantLeaf compiles a calendar-day comparison against a timestamp
// column as bounds on that column. A day D runs from D's midnight to the next
// one, both read in the session time zone as CURRENT_DATE is, so `eq` is the
// half-open range and each ordering picks the one bound that answers it.
func compileInstantLeaf(p Predicate, field Field, arg func(any) int) (string, error) {
	value, err := scalarOperand(p.Value, field, p.Field, p.Op)
	if err != nil {
		return "", err
	}
	day := operandSQL(value, arg)
	if _, relative := value.(RelativeDays); !relative {
		day += "::date"
	}
	start := "(" + day + ")::timestamptz"
	next := "(" + day + " + 1)::timestamptz"
	column := field.Expr
	switch p.Op {
	case OpEq:
		return fmt.Sprintf("(%s >= %s AND %s < %s)", column, start, column, next), nil
	case OpNeq:
		return fmt.Sprintf("(%s IS NULL OR %s < %s OR %s >= %s)", column, column, start, column, next), nil
	case OpGt:
		return column + " >= " + next, nil
	case OpGte:
		return column + " >= " + start, nil
	case OpLt:
		return column + " < " + start, nil
	case OpLte:
		return column + " < " + next, nil
	default:
		return "", &PredicateError{
			Field: p.Field, Code: CodeFilterOpNotAllowed,
			Message: fmt.Sprintf("operator %q does not apply to the date field %q", p.Op, p.Field),
		}
	}
}
