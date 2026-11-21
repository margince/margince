// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealcoverage_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/dealcoverage"
)

func TestAttachAddsTheCountsOnlyWhereAPartialSumIsAsked(t *testing.T) {
	selects, columns := []string{"sum(x)"}, []string{"total"}
	same, sameColumns := dealcoverage.Attach(nil, "t.base", "t.currency", selects, columns)
	if len(same) != 1 || len(sameColumns) != 1 {
		t.Fatalf("a plan with no partial sum was changed: %v %v", same, sameColumns)
	}
	more, moreColumns := dealcoverage.Attach([]string{"total"}, "t.base", "t.currency", selects, columns)
	if len(more) != 4 || len(moreColumns) != 4 || len(selects) != 1 {
		t.Fatalf("attached %d terms and %d columns to %d, want three of each beside the plan's own", len(more), len(moreColumns), len(selects))
	}
	if !strings.Contains(more[3], "string_agg(DISTINCT t.currency") {
		t.Errorf("the currencies are not listed in the same statement: %s", more[3])
	}
}

func TestFoldStatesTheCoverageOfEachRow(t *testing.T) {
	rows := []map[string]any{
		{"total": int64(5), "coverage_no_value": int64(1), "coverage_no_rate": int64(2), "coverage_currencies": "CHF,JPY"},
		{"total": int64(9), "coverage_no_value": int64(0), "coverage_no_rate": int64(0), "coverage_currencies": nil},
	}
	dealcoverage.Fold(rows, []string{"total"})
	want := []map[string]any{
		{"aggregates": []string{"total"}, "partial": true, "no_value_deals": int64(1), "no_rate_deals": int64(2), "missing_currencies": []string{"CHF", "JPY"}},
		{"aggregates": []string{"total"}, "partial": false, "no_value_deals": int64(0), "no_rate_deals": int64(0), "missing_currencies": []string{}},
	}
	for i, row := range rows {
		if !reflect.DeepEqual(row["coverage"], want[i]) || len(row) != 2 {
			t.Errorf("row %d = %v, want its coverage %v and nothing of the helper columns", i, row, want[i])
		}
	}
}
