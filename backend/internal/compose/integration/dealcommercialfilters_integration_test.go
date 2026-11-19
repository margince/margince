// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/deals"
)

// The board draws its cards from the deal list and its stage totals from
// deals-by-stage. For every commercial filter, "unset" included, the two must
// count the same deals, or a filtered column's total disagrees with its cards.
func TestTheDealListAndTheStageTotalsCountOneSetPerCommercialFilter(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	for _, deal := range []deals.CreateDealInput{
		{Name: "Renewal", CommercialMotion: new("renewal"), Priority: new("high"), AcquisitionSource: new("outbound")},
		{Name: "New business", CommercialMotion: new("new_business"), AcquisitionSource: new("referral")},
		{Name: "Nothing recorded"},
	} {
		deal.PipelineID, deal.StageID, deal.Source = pipeline, open, "manual"
		if _, err := e.Deals.CreateDeal(e.Admin(), deal); err != nil {
			t.Fatalf("creating %q: %v", deal.Name, err)
		}
	}
	setters := map[string]func(*deals.ListDealsInput, *string){
		"commercial_motion":  func(in *deals.ListDealsInput, v *string) { in.CommercialMotion = v },
		"priority":           func(in *deals.ListDealsInput, v *string) { in.Priority = v },
		"acquisition_source": func(in *deals.ListDealsInput, v *string) { in.AcquisitionSource = v },
	}
	want := map[string]map[string]int{
		"commercial_motion":  {"renewal": 1, "upsell": 0, "unset": 1},
		"priority":           {"high": 1, "low": 0, "unset": 2},
		"acquisition_source": {"outbound": 1, "referral": 1, "unset": 1},
	}
	// Derived from the owner, so a filter added there without a case here fails,
	// and so does one the owner stops answering.
	names := slices.Sorted(maps.Keys(deals.CommercialFilterPredicates("")))
	if !slices.Equal(names, slices.Sorted(maps.Keys(want))) {
		t.Errorf("the commercial filters are %v, want %v", names, slices.Sorted(maps.Keys(want)))
	}
	for _, name := range names {
		set, cases := setters[name], want[name]
		if set == nil || cases == nil {
			t.Errorf("%s is a commercial filter with no case here", name)
			continue
		}
		for value, count := range cases {
			in := deals.ListDealsInput{PipelineID: &pipeline}
			set(&in, new(value))
			listed, _, err := e.Deals.ListDeals(e.Admin(), in)
			if err != nil {
				t.Fatalf("listing %s=%s: %v", name, value, err)
			}
			plan := fmt.Sprintf(`{"group_by":["pipeline_id"],"filters":{"pipeline_id":%q,%q:%q},"aggregates":[{"fn":"count","as":"deals"}]}`,
				pipeline.String(), name, value)
			counted := 0
			for _, row := range runPrebuiltReport(e.Admin(), t, e, "deals-by-stage", plan) {
				n, err := strconv.Atoi(cell(row, "deals"))
				if err != nil {
					t.Fatalf("%s=%s: the deals cell %v is not a count", name, value, row["deals"])
				}
				counted += n
			}
			if len(listed) != count || counted != count {
				t.Errorf("%s=%s: the list has %d deals and the stage totals count %d, want %d in both",
					name, value, len(listed), counted, count)
			}
		}
	}
}

// An analytics question compares the commercial columns as plain values. The
// list's "unset" word belongs to the report plan, and here is_null asks it.
func TestAnAnalyticsQuestionComparesTheCommercialColumnsAsValues(t *testing.T) {
	d := newTwoDoors(t, "deal-commercial-analytics", "read")
	stages := apptest.DiscoverSeededPipeline(t, d.e)
	// Five of each, so no filter separates out fewer than the privacy floor.
	for range analyticsquery.DefaultFloor {
		for _, deal := range []AnyMap{
			{"name": "Renewal", "commercial_motion": "renewal", "priority": "high"},
			{"name": "New business", "commercial_motion": "new_business"},
			{"name": "Nothing recorded"},
		} {
			deal["pipeline_id"], deal["stage_id"], deal["source"] = stages.PipelineID, stages.Open, "manual"
			createdID(t, d.e, "/v1/deals", deal)
		}
	}
	for _, tc := range []struct {
		field, op string
		value     any
		want      string
	}{
		{"priority", "is_null", nil, "10"},
		{"commercial_motion", "ne", "renewal", "5"},
	} {
		filter := map[string]any{"field": tc.field, "op": tc.op}
		if tc.value != nil {
			filter["value"] = tc.value
		}
		answer := askMoney(t, d, map[string]any{
			"entity": "deals-by-stage", "scope_kind": "workspace",
			"measures": []any{map[string]any{"fn": "count", "as": "deals"}},
			"filters":  []any{filter},
		})
		if len(answer.Rows) != 1 || fmt.Sprint(answer.Rows[0]["deals"]) != tc.want {
			t.Errorf("%s %s %v counted %+v, want %s deals", tc.field, tc.op, tc.value, answer.Rows, tc.want)
		}
	}
}
