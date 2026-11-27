// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// Two deals each hold an amount below 2^53, and their sum is past it.
// The report withholds the sum and names it, and answers the minimum and the
// count beside it. A JSON reader would otherwise parse the sum as another number.
func TestAReportTotalPastTheExactRangeIsWithheldAndTheRowStillAnswers(t *testing.T) {
	d := newTwoDoors(t, "exact-range", "read")
	stages := apptest.DiscoverSeededPipeline(t, d.e)
	half := int64(values.MaxExactInteger/2 + 1)
	for range 2 {
		createdID(t, d.e, "/v1/deals", AnyMap{
			"name": "Large deal", "pipeline_id": stages.PipelineID, "stage_id": stages.Open,
			"source": "manual", "amount_minor": half, "currency": "EUR",
		})
	}
	row := onlyRow(t, d.rest(t, "POST", "/v1/reports/deals-by-stage", map[string]any{
		"group_by": []any{"currency"},
		"aggregates": []any{
			map[string]any{"fn": "sum", "field": "amount_minor", "as": "total"},
			map[string]any{"fn": "min", "field": "amount_minor", "as": "smallest"},
			map[string]any{"fn": "count", "as": "deals"},
		},
	}))
	if row["total"] != nil {
		t.Errorf("total = %v, want it withheld", row["total"])
	}
	named, ok := row["_out_of_range"].([]any)
	if !ok || len(named) != 1 || named[0] != "total" {
		t.Errorf("_out_of_range = %v, want [total]", row["_out_of_range"])
	}
	if smallest, ok := row["smallest"].(json.Number); !ok || smallest.String() != strconv.FormatInt(half, 10) {
		t.Errorf("smallest = %v, want %d: a figure in range is still answered", row["smallest"], half)
	}
	if deals, ok := row["deals"].(json.Number); !ok || deals.String() != "2" {
		t.Errorf("deals = %v, want 2", row["deals"])
	}
}
