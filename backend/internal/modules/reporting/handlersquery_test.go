// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"net/http/httptest"
	"reflect"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestReportingQueryRetainsRepeatedAndCommaSeparatedSelections(t *testing.T) {
	for _, query := range []string{
		"metrics=bookings_won&metrics=open_pipeline&metrics=stage_age&blocks=bookings_trend&blocks=stage_distribution&blocks=owner_attainment&blocks=stage_age",
		"metrics=bookings_won,open_pipeline,stage_age&blocks=bookings_trend,stage_distribution,owner_attainment,stage_age",
	} {
		selection, err := SelectionFromQuery(httptest.NewRequest("GET", "/analytics/evaluate?"+query, nil).URL.Query())
		if err != nil {
			t.Fatal(err)
		}
		metrics := []crmcontracts.ReportingMetricID{"bookings_won", "open_pipeline", "stage_age"}
		blocks := []crmcontracts.ReportingBlockKind{"bookings_trend", "stage_distribution", "owner_attainment", "stage_age"}
		if !reflect.DeepEqual(selection.Metrics, metrics) || !reflect.DeepEqual(selection.Blocks, blocks) {
			t.Fatalf("selection lost query members: %+v", selection)
		}
	}
}

func TestReportingQueryNamingMetricsAloneCarriesNoDefaultCharts(t *testing.T) {
	cases := map[string]struct{ metrics, blocks int }{
		"metrics=meetings_held":                  {1, 0},
		"metrics=meetings_held&blocks=stage_age": {1, 1},
		"":                                       {3, 4},
		"blocks=stage_age":                       {3, 1},
	}
	for query, want := range cases {
		selection, err := SelectionFromQuery(httptest.NewRequest("GET", "/analytics/evaluate?"+query, nil).URL.Query())
		if err != nil {
			t.Fatal(err)
		}
		if len(selection.Metrics) != want.metrics || len(selection.Blocks) != want.blocks || selection.Blocks == nil {
			t.Errorf("%q: %d metrics and %v charts, want %d and %d", query, len(selection.Metrics), selection.Blocks, want.metrics, want.blocks)
		}
	}
}
