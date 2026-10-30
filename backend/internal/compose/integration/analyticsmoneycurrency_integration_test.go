// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A money aggregate over deals in EUR, USD, JPY and an unrated CHF, written
// through the deal and rate routes and asked on both doors.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/clocktest"
)

// Five deals per currency, so every currency group clears the privacy floor.
// JPY has no minor digits: 1000 is a thousand yen, where 1000 USD minor units
// are ten dollars.
var mixedCurrencyDeals = map[string]int64{"EUR": 100_000, "USD": 10_000, "JPY": 1_000, "CHF": 2_000}

const dealsPerCurrency = 5

func seedMixedCurrencyPipeline(t *testing.T, d *twoDoors) {
	t.Helper()
	if base := baseCurrency(t, d.e); base != "EUR" {
		t.Fatalf("the bootstrapped installation's base currency is %s; these sums are worked out in EUR", base)
	}
	today := clocktest.Now(t).UTC().Format(time.DateOnly)
	// CHF gets no rate: its deals are priced natively and cannot be converted.
	for from, rate := range map[string]string{"USD": "0.5", "JPY": "0.006"} {
		if status := d.e.Call(t, "POST", "/v1/fx-rates",
			map[string]any{"from_currency": from, "rate": rate, "effective_date": today}, nil, nil); status != http.StatusCreated {
			t.Fatalf("setting the %s rate → %d", from, status)
		}
	}
	stages := apptest.DiscoverSeededPipeline(t, d.e)
	for currency, amount := range mixedCurrencyDeals {
		for range dealsPerCurrency {
			createdID(t, d.e, "/v1/deals", AnyMap{
				"name": currency + " deal", "pipeline_id": stages.PipelineID, "stage_id": stages.Open,
				"source": "manual", "amount_minor": amount, "currency": currency,
			})
		}
	}
}

// moneyAnswer holds rows decoded with UseNumber, so a cell prints as the
// integer the server sent.
type moneyAnswer struct {
	Rows []map[string]any `json:"rows"`
}

func askMoney(t *testing.T, d *twoDoors, query map[string]any) moneyAnswer {
	t.Helper()
	var out moneyAnswer
	body := d.rest(t, "POST", "/v1/analytics/query", query)
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.UseNumber()
	if err := dec.Decode(&out); err != nil {
		t.Fatalf("decoding the answer: %v\n%s", err, body)
	}
	return out
}

func TestANativeMoneyAggregateAcrossCurrenciesIsRefusedOnBothDoors(t *testing.T) {
	d := newTwoDoors(t, "money-currency-refused", "read")
	seedMixedCurrencyPipeline(t, d)

	for _, entity := range []string{"deals-by-stage", "open-deals-per-company", "win-loss"} {
		for _, fn := range []string{"sum", "avg", "min", "max", "median"} {
			query := map[string]any{
				"entity": entity, "scope_kind": "workspace",
				"measures": []any{map[string]any{"fn": fn, "field": "amount_minor"}},
			}
			var refusal struct {
				Details struct{ Kind, Message, Suggest string } `json:"details"`
			}
			if err := json.Unmarshal(d.restStatus(t, "POST", "/v1/analytics/query", query, http.StatusBadRequest), &refusal); err != nil {
				t.Fatalf("decoding the refusal: %v", err)
			}
			if refusal.Details.Kind != "invalid" || !strings.Contains(refusal.Details.Suggest, "group by currency") ||
				!strings.Contains(refusal.Details.Suggest, "amount_base_minor") {
				t.Errorf("%s %s(amount_minor): refusal %+v does not say to group by currency or use amount_base_minor",
					entity, fn, refusal.Details)
			}
			if tool := d.toolRefused(t, "run_analytics_query", query); !strings.Contains(tool, "amount_base_minor") {
				t.Errorf("%s %s(amount_minor): the tool's refusal names no converted measure: %s", entity, fn, tool)
			}
		}
	}

	report := map[string]any{
		"group_by":   []any{"stage_id"},
		"aggregates": []any{map[string]any{"fn": "sum", "field": "amount_minor", "as": "total"}},
	}
	if body := d.restStatus(t, "POST", "/v1/reports/deals-by-stage", report, http.StatusUnprocessableEntity); !strings.Contains(string(body), "amount_base_minor") {
		t.Errorf("the report route's refusal names no converted measure: %s", body)
	}
	if tool := d.toolRefused(t, "run_report", map[string]any{
		"report":   "deals-by-stage",
		"group_by": report["group_by"], "aggregates": report["aggregates"],
	}); !strings.Contains(tool, "amount_base_minor") {
		t.Errorf("run_report's refusal names no converted measure: %s", tool)
	}
}

func TestEachCurrencyIsSummedInItsOwnMinorUnits(t *testing.T) {
	d := newTwoDoors(t, "money-currency-split", "read")
	seedMixedCurrencyPipeline(t, d)
	sum := []any{map[string]any{"fn": "sum", "field": "amount_minor", "as": "total"}}

	grouped := askMoney(t, d, map[string]any{"entity": "deals-by-stage", "group_by": []any{"currency"}, "measures": sum})
	if len(grouped.Rows) != len(mixedCurrencyDeals) {
		t.Fatalf("grouped by currency the answer has %d rows, want one per currency: %+v", len(grouped.Rows), grouped.Rows)
	}
	for _, row := range grouped.Rows {
		currency := fmt.Sprint(row["currency"])
		if want := mixedCurrencyDeals[currency] * dealsPerCurrency; fmt.Sprint(row["total"]) != itoa(want) {
			t.Errorf("%s totals %s, want %d", currency, row["total"], want)
		}
	}

	pinned := askMoney(t, d, map[string]any{
		"entity": "deals-by-stage", "measures": sum,
		"filters": []any{map[string]any{"field": "currency", "op": "eq", "value": "JPY"}},
	})
	if len(pinned.Rows) != 1 || fmt.Sprint(pinned.Rows[0]["total"]) != itoa(mixedCurrencyDeals["JPY"]*dealsPerCurrency) {
		t.Errorf("pinned to JPY the answer is %+v, want one row of %d yen", pinned.Rows, mixedCurrencyDeals["JPY"]*dealsPerCurrency)
	}
}

// The converted measure on each of the three populations the decision names.
// CHF has no rate, so it is left out of the total and out of the priced count.
func TestTheBaseMeasureConvertsEachDealAndLeavesTheUnratedOut(t *testing.T) {
	d := newTwoDoors(t, "money-currency-base", "read")
	seedMixedCurrencyPipeline(t, d)
	// 5×100000 EUR + 5×(10000 USD cents × 0.5) + 5×(1000 yen × 0.006 × 100 cents).
	const want = 500_000 + 25_000 + 3_000
	for _, entity := range []string{"deals-by-stage", "open-deals-per-company"} {
		answer := askMoney(t, d, map[string]any{"entity": entity, "scope_kind": "workspace", "measures": []any{
			map[string]any{"fn": "count", "as": "deals"},
			map[string]any{"fn": "sum", "field": "amount_base_minor", "as": "base"},
			map[string]any{"fn": "count_distinct", "field": "amount_base_minor", "as": "distinct_prices"},
		}})
		if len(answer.Rows) != 1 {
			t.Fatalf("%s: ungrouped answer has %d rows: %+v", entity, len(answer.Rows), answer.Rows)
		}
		row := answer.Rows[0]
		if fmt.Sprint(row["base"]) != itoa(want) {
			t.Errorf("%s: converted total %s, want %d — 3010 short of it means the yen were read as cents",
				entity, row["base"], want)
		}
		if fmt.Sprint(row["deals"]) != itoa(int64(len(mixedCurrencyDeals)*dealsPerCurrency)) {
			t.Errorf("%s: counted %s deals, want every one including the unrated CHF", entity, row["deals"])
		}
		// One converted price per rated currency: CHF has none to count.
		if fmt.Sprint(row["distinct_prices"]) != "3" {
			t.Errorf("%s: %s distinct converted prices, want 3 — CHF has no rate and must not price at zero", entity, row["distinct_prices"])
		}
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
