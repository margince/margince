// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"
)

// Open pipeline groups by the month each deal is expected to close. A deal
// with no close date is its own NULL group. A month's drill-through opens the
// deals that month counted.
func TestOpenPipelineGroupsByCloseMonth(t *testing.T) {
	e := setupForecast(t)
	novemberOne := e.seedOpenDeal(t, "Nov one", 20, &e.Rep1, nil, nil)
	novemberTwo := e.seedOpenDeal(t, "Nov two", 20, &e.Rep1, nil, nil)
	december := e.seedOpenDeal(t, "Dec", 20, &e.Rep1, nil, nil)
	undated := e.seedOpenDeal(t, "Undated", 20, &e.Rep1, nil, nil)
	for deal, day := range map[any]any{
		novemberOne: "2026-11-03", novemberTwo: "2026-11-30", december: "2026-12-01",
	} {
		if _, err := e.owner.Exec(t.Context(), `UPDATE deal SET expected_close_date = $2 WHERE id = $1`, deal, day); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.owner.Exec(t.Context(), `UPDATE deal SET expected_close_date = NULL WHERE id = $1`, undated); err != nil {
		t.Fatal(err)
	}

	admin := e.Admin()
	result := e.runReport(admin, t, "pipeline-current", `{"group_by":["close_month"],"aggregates":[{"fn":"count","as":"deals"}]}`)
	counts := map[string]int64{}
	for _, row := range result.Rows {
		month, _ := row["close_month"].(string)
		counts[month] = wireInt(t, row, "deals")
	}
	want := map[string]int64{"2026-11": 2, "2026-12": 1, "": 1}
	for month, n := range want {
		if counts[month] != n {
			t.Errorf("close_month %q: %d deals, want %d (all rows: %v)", month, counts[month], n, counts)
		}
	}

	handle := derivationURL("pipeline-current", nil, []string{"close_month"},
		[]reportAggregate{{Fn: aggFnCount, As: "deals"}}, map[string]any{"close_month": "2026-11"}, time.Time{}, "")
	opened := e.explainReport(admin, t, "pipeline-current", handle)
	if len(opened.Rows) != 2 {
		t.Errorf("November's drill-through opened %d deals, want the 2 it counted: %v", len(opened.Rows), opened.Rows)
	}

	undatedHandle := derivationURL("pipeline-current", nil, []string{"close_month"},
		[]reportAggregate{{Fn: aggFnCount, As: "deals"}}, map[string]any{"close_month": nil}, time.Time{}, "")
	openedUndated := e.explainReport(admin, t, "pipeline-current", undatedHandle)
	if len(openedUndated.Rows) != 1 || openedUndated.Rows[0]["id"] != undated.String() {
		t.Errorf("the no-close-date drill-through opened %v, want the one undated deal", openedUndated.Rows)
	}
}
