// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Read tools over forecasts, reports and availability. Each test makes the
// state through the product's writers, then asks the route as a passport and
// the tool over MCP with the same input.

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// The forecast, its snapshots, its movement and its input checks read the same
// on their routes and on the tools.
func TestForecastRoutesAnswerAsTheirTools(t *testing.T) {
	d := newTwoDoors(t, "forecast-read-doors", "read")
	stages := apptest.DiscoverSeededPipeline(t, d.e)
	openDeal(t, d.e, stages, "Forecast door deal")
	opening := apptest.FreezeAt(t, d.e, AdminPerms, time.Date(2036, 2, 1, 9, 0, 0, 0, time.UTC))
	closing := apptest.FreezeAt(t, d.e, AdminPerms, time.Date(2036, 2, 10, 9, 0, 0, 0, time.UTC))

	apptest.FinishInputCheck(t, d.e, AdminPerms)

	// The route adds the landing and writes instants in the installation's zone.
	// The figures, the period and the snapshots listed must agree.
	got := sameRecords[sharedForecast](t, "forecast readings",
		d.rest(t, "GET", "/v1/forecast?as_of=2036-02-14", nil),
		d.tool(t, "forecast_readings", map[string]any{"as_of": "2036-02-14"}))
	if len(got.Snapshots) != 2 || got.Snapshots[0].ID != closing || got.Snapshots[1].ID != opening {
		t.Errorf("the forecast lists snapshots %+v, want the closing then the opening one", got.Snapshots)
	}
	sameDocument(t, "forecast movement",
		d.rest(t, "GET", "/v1/forecast/movement?from="+opening+"&to="+closing+"&reading=weighted", nil),
		d.tool(t, "forecast_movement", map[string]any{"from": opening, "to": closing, "reading": "weighted"}))
	// The route writes its instants in the installation's zone and the tool in
	// UTC, so the run, its verdict and what it read are compared.
	check := sameRecords[sharedInputCheck](t, "forecast input checks",
		d.rest(t, "GET", "/v1/forecast/assurance", nil), d.tool(t, "forecast_input_checks", map[string]any{}))
	if check.Status != "complete" || len(check.Sources) != 2 {
		t.Errorf("the input check reads %+v, want the finished run with its 2 sources", check)
	}
	sameDocument(t, "input check exceptions",
		d.rest(t, "GET", "/v1/forecast/assurance/exceptions", nil), d.tool(t, "list_input_checks", map[string]any{}))
	coverage := sameRecords[sharedInputCheck](t, "data coverage",
		d.rest(t, "GET", "/v1/analytics/coverage", nil), d.tool(t, "data_coverage", map[string]any{}))
	if coverage.RunID != check.RunID || len(coverage.Sources) != 2 {
		t.Errorf("data coverage reads %+v, want the same run %s and its 2 sources", coverage, check.RunID)
	}
}

// Before the first nightly input check, every door says no check has run yet.
// A bare not-found reads as a check the caller may not see, which is a
// different and false claim about the sources.
func TestNoInputCheckYetIsSaidAlikeOnEveryDoor(t *testing.T) {
	d := newTwoDoors(t, "no-input-check-doors", "read")
	for _, c := range []struct{ path, tool string }{
		{"/v1/forecast/assurance", "forecast_input_checks"},
		{"/v1/analytics/coverage", "data_coverage"},
	} {
		var answer json.RawMessage
		if status := d.e.Call(t, "GET", c.path, nil, d.bearer, &answer); status != http.StatusNotFound {
			t.Fatalf("%s answered %d %s, want 404 before any check ran", c.path, status, answer)
		}
		if code := problemCode(t, answer); code != "no_input_check_yet" {
			t.Errorf("%s refused with %q, want no_input_check_yet", c.path, code)
		}
		if refusal := d.toolRefused(t, c.tool, map[string]any{}); !namesCode(refusal, "no_input_check_yet") {
			t.Errorf("%s refused with %s, not the route's no_input_check_yet", c.tool, refusal)
		}
	}
}

type sharedInputCheck struct {
	RunID         string `json:"run_id"`
	Status        string `json:"status"`
	Readiness     string `json:"readiness"`
	EligibleDeals int    `json:"eligible_deals"`
	Sources       []struct {
		Source string `json:"source"`
		State  string `json:"state"`
	} `json:"sources"`
}

// A day named in as_of is the same day on both doors in a zone west of UTC.
// There, midnight UTC on a quarter's first day is still the quarter before.
// A door that skipped the midday anchor would answer the wrong quarter.
func TestForecastRouteAndToolReadTheSameDayInAWesternZone(t *testing.T) {
	d := newTwoDoors(t, "forecast-zone-doors", "read")
	if status := d.e.Call(t, "PATCH", "/v1/installation/settings",
		AnyMap{"timezone": "America/Los_Angeles"}, nil, nil); status != 200 {
		t.Fatalf("setting the installation zone → %d", status)
	}

	got := sameRecords[sharedForecast](t, "forecast on a quarter's first day",
		d.rest(t, "GET", "/v1/forecast?as_of=2036-04-01", nil),
		d.tool(t, "forecast_readings", map[string]any{"as_of": "2036-04-01"}))
	if got.PeriodStart != "2036-04-01" {
		t.Errorf("the forecast for 2036-04-01 starts %s, want the quarter that begins that day", got.PeriodStart)
	}
}

type sharedForecast struct {
	PeriodStart  string `json:"period_start"`
	PeriodEnd    string `json:"period_end"`
	ScopeKind    string `json:"scope_kind"`
	WonMinor     int64  `json:"won_minor"`
	EvidenceMin  int64  `json:"evidence_minor"`
	OpenMinor    int64  `json:"open_minor"`
	WeightedMin  int64  `json:"weighted_minor"`
	EligibleN    int    `json:"eligible_count"`
	PricedN      int    `json:"priced_count"`
	BaseCurrency string `json:"base_currency"`
	Snapshots    []struct {
		ID      string `json:"id"`
		Trigger string `json:"trigger"`
	} `json:"snapshots"`
}

// An analytics query and a report run read the same on POST /analytics/query
// and run_analytics_query, and on POST /reports/{report} and run_report.
func TestAnalyticsAndReportRoutesAnswerAsTheirTools(t *testing.T) {
	d := newTwoDoors(t, "analytics-read-doors", "read")
	stages := apptest.DiscoverSeededPipeline(t, d.e)
	for i := range 6 {
		openDeal(t, d.e, stages, "Analytics door deal "+string(rune('A'+i)))
	}

	query := map[string]any{
		"entity":   "open-deals-per-company",
		"measures": []any{map[string]any{"fn": "count", "as": "dealcount"}},
	}
	sameDocument(t, "analytics query",
		d.rest(t, "POST", "/v1/analytics/query", query), d.tool(t, "run_analytics_query", query))

	// The route adds derivation links and the tool a row count.
	// The rows' figures must agree.
	got := sameRecords[sharedReport](t, "report run",
		d.rest(t, "POST", "/v1/reports/deals-by-stage", map[string]any{}),
		d.tool(t, "run_report", map[string]any{"report": "deals-by-stage"}))
	if len(got.Rows) != 1 || got.Rows[0].Deals != 6 {
		t.Errorf("the report counts %+v, want the 6 seeded deals in one stage", got.Rows)
	}
}

type sharedReport struct {
	Report  string   `json:"report"`
	Columns []string `json:"columns"`
	Rows    []struct {
		StageID string `json:"stage_id"`
		Deals   int    `json:"deals"`
	} `json:"rows"`
}

// Sales reporting reads the same on its routes and on read_reporting, whichever
// mode names the question.
func TestReportingRoutesAnswerAsReadReporting(t *testing.T) {
	d := newTwoDoors(t, "reporting-read-doors", "read")

	catalog := d.rest(t, "GET", "/v1/analytics/metrics", nil)
	sameDocument(t, "reporting catalog", catalog,
		memberOf(t, d.tool(t, "read_reporting", map[string]any{"mode": "catalog"}), "catalog"))
	sameDocument(t, "reporting reports",
		d.rest(t, "GET", "/v1/analytics/reports", nil),
		memberOf(t, d.tool(t, "read_reporting", map[string]any{"mode": "reports"}), "reports"))

	var metrics struct {
		Metrics []struct {
			ID string `json:"id"`
		} `json:"metrics"`
	}
	if err := json.Unmarshal(catalog, &metrics); err != nil || len(metrics.Metrics) == 0 {
		t.Fatalf("the catalog names no metric to evaluate (%v):\n%s", err, catalog)
	}
	metric := metrics.Metrics[0].ID
	// Each evaluation stamps the moment it ran, and its key hashes that moment,
	// so the figures and the population they were drawn from are compared.
	evaluation := sameRecords[sharedEvaluation](t, "reporting evaluation",
		d.rest(t, "GET", "/v1/analytics/evaluate?period=this_month&target_basis=month&close_window=all_open&metrics="+metric, nil),
		memberOf(t, d.tool(t, "read_reporting", map[string]any{
			"mode": "evaluate",
			"selection": map[string]any{
				"period": "this_month", "target_basis": "month", "close_window": "all_open", "metrics": []string{metric},
			},
		}), "evaluation"))
	if len(evaluation.Metrics) != 1 || evaluation.Metrics[0].ID != metric || len(evaluation.Context.MemberIDs) == 0 {
		t.Errorf("the evaluation reads %+v, want the one metric asked for over a population", evaluation)
	}

	// A saved report and its editions. The edition, evidence and compare modes
	// need a published edition, which only a scheduled run makes, so they are
	// left to their own reporting tests.
	saved := createdRecord(t, d.e, "/v1/analytics/reports", AnyMap{
		"name": "Door report", "audience": "private",
		"selection": AnyMap{"period": "this_month", "target_basis": "month", "close_window": "all_open", "metrics": []string{metric}},
	})
	sameDocument(t, "a saved report",
		d.rest(t, "GET", "/v1/analytics/reports/"+saved, nil),
		memberOf(t, d.tool(t, "read_reporting", map[string]any{"mode": "report", "id": saved}), "report"))
	sameDocument(t, "a report's editions",
		d.rest(t, "GET", "/v1/analytics/reports/"+saved+"/editions", nil),
		memberOf(t, d.tool(t, "read_reporting", map[string]any{"mode": "editions", "id": saved}), "editions"))
}

type sharedEvaluation struct {
	Context struct {
		PeriodKind            string   `json:"period_kind"`
		PopulationFingerprint string   `json:"population_fingerprint"`
		MemberIDs             []string `json:"member_ids"`
		Currency              string   `json:"currency"`
		DefinitionVersion     string   `json:"definition_version"`
	} `json:"context"`
	Metrics []struct {
		ID    string  `json:"id"`
		Value float64 `json:"value"`
		Unit  string  `json:"unit"`
	} `json:"metrics"`
}

// The calendar's free time reads the same on GET /availability and
// check_availability.
func TestAvailabilityRouteAnswersAsTheTool(t *testing.T) {
	d := newTwoDoorsWith(t, "availability-read-doors", []compose.Option{(&apptest.BookingCalendar{}).Option()}, "read")
	apptest.EnableBooking(t, d.e)
	booked := apptest.NextBookableMorning(t)
	day := booked.Truncate(24 * time.Hour)
	from, to := day.Add(8*time.Hour).Format(time.RFC3339), day.Add(18*time.Hour).Format(time.RFC3339)
	bookedFrom, bookedTo := booked.Format(time.RFC3339), booked.Add(time.Hour).Format(time.RFC3339)
	contact := createdRecord(t, d.e, "/v1/contacts", AnyMap{
		"source": "manual", "full_name": "Bea Booked",
		"emails": []AnyMap{{"email": "bea@buyer.example", "email_type": "work", "is_primary": true}},
	})
	if status := d.e.Call(t, "POST", "/v1/bookings", AnyMap{
		"start": bookedFrom, "end": bookedTo, "subject": "Door booking",
		"links": []AnyMap{{"entity_type": "contact", "entity_id": contact}},
	}, nil, nil); status != 201 {
		t.Fatalf("booking the hour → %d", status)
	}

	// The route also names the calendar backing; the free slots must agree and
	// leave the booked hour out.
	got := sameRecords[sharedSlots](t, "availability",
		d.rest(t, "GET", "/v1/availability?from="+from+"&to="+to, nil),
		d.tool(t, "check_availability", map[string]any{"from": from, "to": to}))
	if len(got.Slots) == 0 {
		t.Fatalf("a working day with one booked hour offers no slot: %+v", got)
	}
	for _, slot := range got.Slots {
		if slot.Start >= bookedFrom && slot.Start < bookedTo {
			t.Errorf("slot %+v overlaps the booked hour", slot)
		}
	}
}

type sharedSlots struct {
	Slots []struct {
		Start string `json:"start"`
		End   string `json:"end"`
	} `json:"slots"`
}
