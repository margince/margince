// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"errors"
	"strings"
	"testing"
)

var placeFields = map[string]Field{
	"location": {Expr: "t", Type: FieldDistance, GeoTarget: "company"},
}

func within(operand map[string]any) Predicate {
	return Predicate{Field: "location", Op: OpWithinRadius, Value: operand}
}

func TestWithinRadiusCompilesToBoundValuesOnly(t *testing.T) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	sql, err := CompilePredicate(within(map[string]any{
		"lat": 48.7758, "lon": 9.1829, "radius_km": 80.0, "label": "Stuttgart",
	}), placeFields, arg)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	for _, want := range []string{"t.geocode_status = $", "t.geocode_lat IS NOT NULL", "t.geocode_lat BETWEEN"} {
		if !strings.Contains(sql, want) {
			t.Errorf("SQL is missing %q:\n%s", want, sql)
		}
	}
	if strings.Contains(sql, "48.7758") || strings.Contains(sql, "Stuttgart") {
		t.Errorf("a value reached the SQL text instead of a bind:\n%s", sql)
	}
	if len(args) == 0 {
		t.Error("no value was bound")
	}
}

func TestWithinRadiusRefusesABadOperand(t *testing.T) {
	for name, operand := range map[string]map[string]any{
		"missing radius":  {"lat": 48.0, "lon": 9.0},
		"zero radius":     {"lat": 48.0, "lon": 9.0, "radius_km": 0.0},
		"negative radius": {"lat": 48.0, "lon": 9.0, "radius_km": -5.0},
		"lat off earth":   {"lat": 91.0, "lon": 9.0, "radius_km": 5.0},
		"lon off earth":   {"lat": 48.0, "lon": 181.0, "radius_km": 5.0},
		"missing center":  {"radius_km": 5.0},
		"unknown key":     {"lat": 48.0, "lon": 9.0, "radius_km": 5.0, "unit": "miles"},
		"place by name":   {"center": "Stuttgart", "radius_km": 5.0},
	} {
		t.Run(name, func(t *testing.T) {
			arg := func(any) int { return 1 }
			_, err := CompilePredicate(within(operand), placeFields, arg)
			var perr *PredicateError
			if !errors.As(err, &perr) || perr.Code != CodeFilterValueInvalid {
				t.Fatalf("err = %v, want a %s PredicateError", err, CodeFilterValueInvalid)
			}
		})
	}
}

func TestADistanceFieldAdmitsOnlyWithinRadius(t *testing.T) {
	got := OperatorsFor(placeFields["location"])
	if len(got) != 1 || got[0] != OpWithinRadius {
		t.Fatalf("OperatorsFor(distance) = %v, want [within_radius]", got)
	}
	arg := func(any) int { return 1 }
	_, err := CompilePredicate(Predicate{Field: "location", Op: OpEq, Value: "x"}, placeFields, arg)
	var perr *PredicateError
	if !errors.As(err, &perr) || perr.Code != CodeFilterOpNotAllowed {
		t.Fatalf("eq on a distance field: err = %v, want %s", err, CodeFilterOpNotAllowed)
	}
}
