// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A money aggregate over deals in EUR, USD, JPY and an unrated CHF, written
// through the deal and rate routes and asked on both doors.

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"slices"
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

// Every money population adds the deals that convert, as the reports do.
func TestTheBaseMeasureAddsTheDealsThatConvertAndNamesWhatItLeavesOut(t *testing.T) {
	d := newTwoDoors(t, "money-currency-base", "read")
	seedMixedCurrencyPipeline(t, d)
	// 5×100000 EUR + 5×(10000 USD cents × 0.5) + 5×(1000 yen × 0.006 × 100 cents).
	const want = 500_000 + 25_000 + 3_000
	measures := []any{
		map[string]any{"fn": "count", "as": "deals"},
		map[string]any{"fn": "sum", "field": "amount_base_minor", "as": "base"},
		map[string]any{"fn": "count_distinct", "field": "amount_base_minor", "as": "distinct_prices"},
	}
	query := map[string]any{"entity": "deals-by-stage", "scope_kind": "workspace", "measures": measures}
	doors := map[string]moneyAnswer{
		"route": askMoney(t, d, query),
		"tool":  decodeMoney(t, d.tool(t, "run_analytics_query", query)),
	}
	for door, answer := range doors {
		if len(answer.Rows) != 1 {
			t.Fatalf("%s: ungrouped answer has %d rows: %+v", door, len(answer.Rows), answer.Rows)
		}
		row := answer.Rows[0]
		if fmt.Sprint(row["base"]) != itoa(want) {
			t.Errorf("%s: converted total = %v, want the deals that convert, %d", door, row["base"], want)
		}
		if fmt.Sprint(row["deals"]) != itoa(int64(len(mixedCurrencyDeals)*dealsPerCurrency)) {
			t.Errorf("%s: counted %s deals, want every one including the unrated CHF", door, row["deals"])
		}
		if fmt.Sprint(row["distinct_prices"]) != "3" {
			t.Errorf("%s: %s distinct converted prices, want 3: CHF has no rate and must not price at zero", door, row["distinct_prices"])
		}
		coverage, _ := row["_coverage"].(map[string]any)
		got := []string{
			fmt.Sprint(coverage["partial"]), fmt.Sprint(coverage["no_value_deals"]), fmt.Sprint(coverage["no_rate_deals"]),
			fmt.Sprint(coverage["missing_currencies"]), fmt.Sprint(coverage["aggregates"]),
		}
		if wantCoverage := []string{"true", "0", itoa(dealsPerCurrency), "[CHF]", "[base]"}; !slices.Equal(got, wantCoverage) {
			t.Errorf("%s: coverage %v, want %v", door, got, wantCoverage)
		}
	}

	query["filters"] = []any{map[string]any{"field": "currency", "op": "ne", "value": "CHF"}}
	priced := askMoney(t, d, query)
	if len(priced.Rows) != 1 || fmt.Sprint(priced.Rows[0]["base"]) != itoa(want) {
		t.Fatalf("priced subset = %v, want %d with yen converted from whole units", priced.Rows, want)
	}
	if coverage, _ := priced.Rows[0]["_coverage"].(map[string]any); coverage["partial"] != false {
		t.Errorf("a subset with every deal converted states coverage %v, want none left out", coverage)
	}

	perCompany := askMoney(t, d, map[string]any{"entity": "open-deals-per-company", "scope_kind": "workspace", "measures": measures})
	if len(perCompany.Rows) != 1 || fmt.Sprint(perCompany.Rows[0]["base"]) != itoa(want) {
		t.Fatalf("per-company answer = %+v, want the same %d as the stage total", perCompany.Rows, want)
	}
	if coverage, _ := perCompany.Rows[0]["_coverage"].(map[string]any); coverage["partial"] != true {
		t.Errorf("the per-company total leaves the CHF deals out without saying so: %v", coverage)
	}
}

func TestASumNoDealConvertsIntoStaysBlankAndSaysWhy(t *testing.T) {
	d := newTwoDoors(t, "money-currency-none", "read")
	stages := apptest.DiscoverSeededPipeline(t, d.e)
	for range dealsPerCurrency {
		createdID(t, d.e, "/v1/deals", AnyMap{
			"name": "CHF deal", "pipeline_id": stages.PipelineID, "stage_id": stages.Open,
			"source": "manual", "amount_minor": 2_000, "currency": "CHF",
		})
		createdID(t, d.e, "/v1/deals", AnyMap{
			"name": "No amount", "pipeline_id": stages.PipelineID, "stage_id": stages.Open, "source": "manual",
		})
	}
	query := map[string]any{"entity": "deals-by-stage", "scope_kind": "workspace", "measures": []any{
		map[string]any{"fn": "sum", "field": "amount_base_minor", "as": "base"},
		map[string]any{"fn": "max", "field": "amount_base_minor", "as": "largest"},
	}}
	for door, answer := range map[string]moneyAnswer{
		"route": askMoney(t, d, query),
		"tool":  decodeMoney(t, d.tool(t, "run_analytics_query", query)),
	} {
		if len(answer.Rows) != 1 {
			t.Fatalf("%s: answer has %d rows: %+v", door, len(answer.Rows), answer.Rows)
		}
		row := answer.Rows[0]
		if row["base"] != nil || row["largest"] != nil {
			t.Errorf("%s: no deal converts, yet the answer holds %v", door, row)
		}
		coverage, _ := row["_coverage"].(map[string]any)
		if coverage["partial"] != true || fmt.Sprint(coverage["no_value_deals"]) != "5" ||
			fmt.Sprint(coverage["no_rate_deals"]) != "5" || fmt.Sprint(coverage["aggregates"]) != "[base]" {
			t.Errorf("%s: coverage %v, want 5 deals with no value, 5 with no rate, naming only the sum", door, coverage)
		}
	}
}

// Each group states its own gap: one leaves out deals with no rate, the other deals with no value.
func TestEachGroupOfASumStatesItsOwnGapOnBothDoors(t *testing.T) {
	d := newTwoDoors(t, "money-group-gaps", "read")
	stages := apptest.DiscoverSeededPipeline(t, d.e)
	noRate := createdID(t, d.e, "/v1/companies", AnyMap{"display_name": "No rate Ltd", "source": "manual"})
	noValue := createdID(t, d.e, "/v1/companies", AnyMap{"display_name": "No value Ltd", "source": "manual"})
	deal := func(company string, extra AnyMap) {
		body := AnyMap{"name": "Deal", "pipeline_id": stages.PipelineID, "stage_id": stages.Open, "company_id": company, "source": "manual"}
		maps.Copy(body, extra)
		createdID(t, d.e, "/v1/deals", body)
	}
	for range dealsPerCurrency {
		deal(noRate, AnyMap{"amount_minor": 1_000, "currency": "EUR"})
		deal(noRate, AnyMap{"amount_minor": 2_000, "currency": "CHF"})
		deal(noValue, AnyMap{"amount_minor": 1_000, "currency": "EUR"})
		deal(noValue, nil)
	}
	query := map[string]any{
		"entity": "open-deals-per-company", "scope_kind": "workspace", "group_by": []any{"company_id"},
		"measures": []any{map[string]any{"fn": "sum", "field": "amount_base_minor", "as": "base"}},
	}
	for door, answer := range map[string]moneyAnswer{
		"route": askMoney(t, d, query),
		"tool":  decodeMoney(t, d.tool(t, "run_analytics_query", query)),
	} {
		if len(answer.Rows) != 2 {
			t.Fatalf("%s: %d rows, want one per company: %+v", door, len(answer.Rows), answer.Rows)
		}
		for _, row := range answer.Rows {
			coverage, _ := row["_coverage"].(map[string]any)
			got := fmt.Sprintf("%v %v %v %v", row["base"], coverage["no_value_deals"], coverage["no_rate_deals"], coverage["missing_currencies"])
			want := map[string]string{noRate: "5000 0 5 [CHF]", noValue: "5000 5 0 []"}[fmt.Sprint(row["company_id"])]
			if got != want {
				t.Errorf("%s: company %v reads %q, want %q", door, row["company_id"], got, want)
			}
		}
	}
}

func decodeMoney(t *testing.T, body []byte) moneyAnswer {
	t.Helper()
	var out moneyAnswer
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.UseNumber()
	if err := dec.Decode(&out); err != nil {
		t.Fatalf("decoding the answer: %v\n%s", err, body)
	}
	return out
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
