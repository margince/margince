// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"net/http/httptest"
	"reflect"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// testOpening stands for whatever the catalog publishes. These tests are about
// the parsing rule, and the catalog's own default is held in compose.
var testOpening = OpeningSelection{
	Metrics: []crmcontracts.ReportingMetricID{"bookings_won", "open_pipeline", "stage_age"},
	Blocks: []crmcontracts.ReportingBlockKind{
		"bookings_trend", "stage_distribution", "owner_attainment", "stage_age",
	},
}

func TestReportingQueryRetainsRepeatedAndCommaSeparatedSelections(t *testing.T) {
	for _, query := range []string{
		"metrics=bookings_won&metrics=open_pipeline&metrics=stage_age&blocks=bookings_trend&blocks=stage_distribution&blocks=owner_attainment&blocks=stage_age",
		"metrics=bookings_won,open_pipeline,stage_age&blocks=bookings_trend,stage_distribution,owner_attainment,stage_age",
	} {
		selection, err := SelectionFromQuery(httptest.NewRequest("GET", "/analytics/evaluate?"+query, nil).URL.Query(), testOpening)
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
		"":                                       {len(testOpening.Metrics), len(testOpening.Blocks)},
		"blocks=stage_age":                       {len(testOpening.Metrics), 1},
	}
	for query, want := range cases {
		selection, err := SelectionFromQuery(httptest.NewRequest("GET", "/analytics/evaluate?"+query, nil).URL.Query(), testOpening)
		if err != nil {
			t.Fatal(err)
		}
		if len(selection.Metrics) != want.metrics || len(selection.Blocks) != want.blocks || selection.Blocks == nil {
			t.Errorf("%q: %d metrics and %v charts, want %d and %d", query, len(selection.Metrics), selection.Blocks, want.metrics, want.blocks)
		}
	}
}
