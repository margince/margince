// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A converted sum adds the deals that convert, and the row says which deals it
// left out and why. Run through the report route and the run_report tool, for
// every prebuilt report that sums base-currency money.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/clocktest"
)

type partialSumDeal struct {
	amount   int64
	currency string
}

type partialSumAnswer struct {
	Rows []map[string]any `json:"rows"`
}

// onlyRow decodes numbers as written and returns the report's one row.
func onlyRow(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	var out partialSumAnswer
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	if err := dec.Decode(&out); err != nil || len(out.Rows) != 1 {
		t.Fatalf("want one row, got %s (%v)", raw, err)
	}
	return out.Rows[0]
}

func TestAPartialSumStatesWhatItLeavesOutOnEveryReportPathAndTool(t *testing.T) {
	// CHF has no rate on the sheet, and the base currency is EUR.
	mixed := []partialSumDeal{{10_000, "EUR"}, {10_000, "EUR"}, {5_000, "CHF"}, {0, ""}}
	for _, tc := range []struct {
		name            string
		deals           []partialSumDeal
		base            any
		noValue, noRate int
		currencies      []string
	}{
		{"mixed", mixed, "20000", 1, 1, []string{"CHF"}},
		{"all without a rate", []partialSumDeal{{5_000, "CHF"}, {7_000, "CHF"}}, nil, 0, 2, []string{"CHF"}},
		{"only a deal with no value", []partialSumDeal{{0, ""}}, nil, 1, 0, []string{}},
		{"no deals", nil, nil, 0, 0, nil},
	} {
		for _, report := range []string{"deals-by-stage", "pipeline-current", "forecast"} {
			t.Run(report+"/"+tc.name, func(t *testing.T) {
				d := newTwoDoors(t, "partial-sums", "read")
				stages := apptest.DiscoverSeededPipeline(t, d.e)
				for _, deal := range tc.deals {
					body := AnyMap{"name": "Deal", "pipeline_id": stages.PipelineID, "stage_id": stages.Open, "source": "manual"}
					if deal.currency != "" {
						body["amount_minor"], body["currency"] = deal.amount, deal.currency
					}
					createdID(t, d.e, "/v1/deals", body)
				}
				plan := map[string]any{"report": report, "group_by": []any{"pipeline_id"}, "aggregates": []any{
					map[string]any{"fn": "sum", "field": "amount_base_minor", "as": "raw"},
					map[string]any{"fn": "sum", "field": "weighted_base_minor", "as": "weighted"},
				}}
				if tc.currencies == nil {
					// An empty group has no row at all, never a row of zeros.
					var none partialSumAnswer
					if err := json.Unmarshal(d.rest(t, "POST", "/v1/reports/"+report, map[string]any{"group_by": plan["group_by"], "aggregates": plan["aggregates"]}), &none); err != nil || len(none.Rows) != 0 {
						t.Errorf("no deals answered %v (%v), want no rows", none.Rows, err)
					}
					return
				}
				route := onlyRow(t, d.rest(t, "POST", "/v1/reports/"+report, map[string]any{"group_by": plan["group_by"], "aggregates": plan["aggregates"]}))
				tool := onlyRow(t, d.tool(t, "run_report", plan))

				for door, row := range map[string]map[string]any{"route": route, "run_report": tool} {
					if row["raw"] == nil != (tc.base == nil) || (tc.base != nil && fmt.Sprint(row["raw"]) != tc.base) {
						t.Errorf("%s: raw sum %v, want %v", door, row["raw"], tc.base)
					}
					if (row["weighted"] == nil) != (tc.base == nil) {
						t.Errorf("%s: weighted sum %v, want it absent exactly when the raw sum is", door, row["weighted"])
					}
					coverage, _ := row["coverage"].(map[string]any)
					if coverage == nil {
						t.Fatalf("%s: row %v states no coverage", door, row)
					}
					got := []any{coverage["partial"], fmt.Sprint(coverage["no_value_deals"]), fmt.Sprint(coverage["no_rate_deals"]), coverage["missing_currencies"], coverage["aggregates"]}
					want := []any{tc.noValue+tc.noRate > 0, fmt.Sprint(tc.noValue), fmt.Sprint(tc.noRate), anySlice(tc.currencies), []any{"raw", "weighted"}}
					if !reflect.DeepEqual(got, want) {
						t.Errorf("%s: coverage %v, want %v", door, got, want)
					}
				}
			})
		}
	}
}

func anySlice(values []string) []any {
	out := make([]any, len(values))
	for i, v := range values {
		out[i] = v
	}
	return out
}

// The sum and the currencies it names come from one statement, so a rate that
// arrives later changes both together and never one of them.
func TestARateArrivingChangesTheSumAndItsMissingCurrenciesTogether(t *testing.T) {
	d := newTwoDoors(t, "partial-sums-rate", "read")
	stages := apptest.DiscoverSeededPipeline(t, d.e)
	for _, deal := range []partialSumDeal{{10_000, "EUR"}, {10_000, "CHF"}} {
		createdID(t, d.e, "/v1/deals", AnyMap{
			"name": "Deal", "pipeline_id": stages.PipelineID, "stage_id": stages.Open,
			"source": "manual", "amount_minor": deal.amount, "currency": deal.currency,
		})
	}
	ask := func() map[string]any {
		sum := map[string]any{"fn": "sum", "field": "amount_base_minor", "as": "raw"}
		return onlyRow(t, d.rest(t, "POST", "/v1/reports/pipeline-current", map[string]any{"aggregates": []any{sum}}))
	}
	before := ask()
	if fmt.Sprint(before["raw"]) != "10000" || fmt.Sprint(before["coverage"].(map[string]any)["missing_currencies"]) != "[CHF]" {
		t.Fatalf("before the rate: %v", before)
	}
	today := clocktest.Now(t).UTC().Format("2006-01-02")
	if status := d.e.Call(t, "POST", "/v1/fx-rates",
		map[string]any{"from_currency": "CHF", "rate": "2", "effective_date": today}, nil, nil); status != http.StatusCreated {
		t.Fatalf("setting the CHF rate → %d", status)
	}
	after := ask()
	if fmt.Sprint(after["raw"]) != "30000" || after["coverage"].(map[string]any)["partial"] != false ||
		fmt.Sprint(after["coverage"].(map[string]any)["missing_currencies"]) != "[]" {
		t.Errorf("after the rate: %v, want the full 30000 and no missing currency", after)
	}
}
