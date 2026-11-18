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
	// Derived from the owner, so a filter added there without a case here fails.
	for _, name := range slices.Sorted(maps.Keys(deals.CommercialFilterPredicates(""))) {
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
