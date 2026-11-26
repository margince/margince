// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"math/big"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
)

func numeric(digits string, exp int32) pgtype.Numeric {
	n, ok := new(big.Int).SetString(digits, 10)
	if !ok {
		panic("not a number: " + digits)
	}
	return pgtype.Numeric{Int: n, Exp: exp, Valid: true}
}

// A numeric sum, a bigint min or max and a float are held to one edge. It is
// the last integer a float64 parses back as itself.
func TestANumberPastTheExactRangeIsCaughtWhateverItsType(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  bool
	}{
		{"the largest exact bigint", reportingExactInteger, false},
		{"one past it as bigint", reportingExactInteger + 1, true},
		{"one past it below zero", -reportingExactInteger - 1, true},
		{"a numeric sum one past it", numeric("9007199254740992", 0), true},
		{"a numeric sum at it", numeric("9007199254740991", 0), false},
		{"a numeric average whose whole part is at it", numeric("90071992547409915", -1), false},
		{"a numeric with a positive exponent past it", numeric("90071992547410", 2), true},
		{"a float past it", float64(1 << 54), true},
		{"a count", int64(12), false},
		{"text", "9007199254740993", false},
		{"null", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := beyondExactRange(tc.value); got != tc.want {
				t.Errorf("beyondExactRange(%v) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}

// A total past 2^53 is withheld and named, and the rest of the row is still
// answered.
func TestARowWithholdsOnlyTheTotalPastTheExactRange(t *testing.T) {
	row := map[string]any{"stage": "won", "count": int64(2), "total": numeric("9007199254740993", 0)}
	withholdBeyondExactRange(row, []string{"stage", "count", "total"})
	if row["total"] != nil || row["stage"] != "won" || row["count"] != int64(2) {
		t.Fatalf("row = %v, want only the total withheld", row)
	}
	if named, ok := row[outOfRangeColumn].([]string); !ok || !slices.Equal(named, []string{"total"}) {
		t.Fatalf("%s = %v, want [total]", outOfRangeColumn, row[outOfRangeColumn])
	}

	within := map[string]any{"total": numeric("100", 0)}
	withholdBeyondExactRange(within, []string{"total"})
	if _, named := within[outOfRangeColumn]; named {
		t.Fatalf("a row within the range names withheld columns: %v", within)
	}
}

// A chart with a point past the exact range is withheld with the range reason,
// as a metric past it is. The evaluation is not refused.
func TestAChartPastTheExactRangeIsWithheldAndTheEvaluationAnswers(t *testing.T) {
	past, fine := float64(1<<54), 12.0
	out := &reporting.Evaluation{}
	chart := withholdChartBeyondExactRange(out, crmcontracts.ReportingChart{
		Coverage: crmcontracts.ReportingCoverage{Status: "ok"},
		Points:   []crmcontracts.ReportingPoint{{Value: &fine}, {Value: &past}},
	})
	if len(chart.Points) != 0 || chart.Coverage.Status != reportingUnavailable || chart.Coverage.Reason == nil {
		t.Fatalf("chart = %+v, want no points and the range reason", chart)
	}
	kept := withholdChartBeyondExactRange(out, crmcontracts.ReportingChart{Points: []crmcontracts.ReportingPoint{{Value: &fine}}})
	if len(kept.Points) != 1 {
		t.Fatalf("a chart within the range lost its points: %+v", kept)
	}
}
