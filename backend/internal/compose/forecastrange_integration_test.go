// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The forecast's range reaches every surface from the one assembler. The HTTP
// door and the tool state one range, and a snapshot freezes it. A share draws
// it again over the rows its reader may see.

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// One deal worth 1,000,000 at the typed 20% wins in two of ten outcomes. So
// the low end is nothing and the high end is the whole deal.
func TestTheHTTPDoorAndTheToolStateOneRange(t *testing.T) {
	f := newStageCohortFixture(t)
	today := clocktest.Now(t)
	f.openDeal(t, f.stage(0), today)

	overHTTP, overTool := f.forecastOverHTTP(t, today), f.forecastOverTool(t, today)
	if overHTTP.RangeLowMinor == nil || overHTTP.RangeHighMinor == nil ||
		*overHTTP.RangeLowMinor != 0 || *overHTTP.RangeHighMinor != 1_000_000 {
		t.Fatalf("GET /forecast range %v..%v, want 0..1,000,000", overHTTP.RangeLowMinor, overHTTP.RangeHighMinor)
	}
	if overTool.RangeLowMinor == nil || overTool.RangeHighMinor == nil ||
		*overTool.RangeLowMinor != *overHTTP.RangeLowMinor || *overTool.RangeHighMinor != *overHTTP.RangeHighMinor {
		t.Fatalf("the tool's range %v..%v differs from the HTTP door's %d..%d",
			overTool.RangeLowMinor, overTool.RangeHighMinor, *overHTTP.RangeLowMinor, *overHTTP.RangeHighMinor)
	}
}

// One deal worth 1,000,000 at the typed 20%, under the weighted measure, lands
// at 200,000. The tool states the landing the HTTP door states.
func TestTheHTTPDoorAndTheToolStateOneLanding(t *testing.T) {
	f := newStageCohortFixture(t)
	today := clocktest.Now(t)
	f.openDeal(t, f.stage(0), today)
	weighted := string(forecasting.MeasureWeighted)
	if err := config.Save(reportingActor(f.env), StoredConfig(f.env.Pool), identity.ForecastForwardMeasure, weighted); err != nil {
		t.Fatal(err)
	}

	overHTTP, overTool := f.forecastOverHTTP(t, today), f.forecastOverTool(t, today)
	if overHTTP.Landing == nil || string(overHTTP.Landing.Measure) != weighted || overHTTP.Landing.AmountMinor != 200_000 {
		t.Fatalf("GET /forecast landing %+v, want 200,000 by the weighted measure", overHTTP.Landing)
	}
	door, tool := overHTTP.Landing, overTool.Landing
	if tool == nil || tool.AmountMinor != door.AmountMinor || tool.Measure != string(door.Measure) ||
		tool.WonMinor != door.WonMinor || tool.RemainingMinor != door.RemainingMinor {
		t.Fatalf("the tool's landing %+v differs from the HTTP door's %+v", tool, door)
	}
}

// The reporting metric states the landing, its basis and its range as
// GET /forecast states them. Its context names the whole quarter apart from
// the cutoff its actuals run through.
func TestTheReportingLandingStatesTheBasisAndRangeTheForecastStates(t *testing.T) {
	f := newStageCohortFixture(t)
	today := clocktest.Now(t)
	f.openDeal(t, f.stage(0), today)
	weighted := string(forecasting.MeasureWeighted)
	if err := config.Save(reportingActor(f.env), StoredConfig(f.env.Pool), identity.ForecastForwardMeasure, weighted); err != nil {
		t.Fatal(err)
	}
	overHTTP := f.forecastOverHTTP(t, today)
	service := newReportingService(f.env.Pool, func() time.Time { return today })
	evaluation := f.evaluateAs(reportingActor(f.env), t, service, crmcontracts.ReportingSelection{
		Scope: crmcontracts.ReportingScope{Kind: "workspace"}, Period: "this_quarter", TargetBasis: "fiscal_quarter",
		CloseWindow: "all_open", Metrics: []crmcontracts.ReportingMetricID{reportingForecastLanding},
	})
	metric := evaluation.Metrics[0]
	basis, door := metric.ForecastBasis, overHTTP.Landing
	if basis == nil || door == nil || !reflect.DeepEqual(basis.Landing, *door) || metric.Value == nil || *metric.Value != float64(door.AmountMinor) {
		t.Fatalf("the reporting landing %v with basis %+v differs from GET /forecast's %+v", metric.Value, basis, door)
	}
	if *metric.RangeLowMinor != *overHTTP.RangeLowMinor || *metric.RangeHighMinor != *overHTTP.RangeHighMinor ||
		basis.PricedCount != int64(overHTTP.PricedCount) || basis.EligibleCount != int64(overHTTP.EligibleCount) {
		t.Fatalf("the reporting range %d..%d over %d of %d deals differs from GET /forecast's %d..%d over %d of %d",
			*metric.RangeLowMinor, *metric.RangeHighMinor, basis.PricedCount, basis.EligibleCount,
			*overHTTP.RangeLowMinor, *overHTTP.RangeHighMinor, overHTTP.PricedCount, overHTTP.EligibleCount)
	}
	frame := evaluation.Context
	if frame.Horizon == nil || !frame.Horizon.EndAt.Equal(basis.Period.EndAt) || !frame.Interval.EndAt.Equal(frame.EvaluatedAt) || !frame.Interval.EndAt.Before(frame.Horizon.EndAt) {
		t.Fatalf("context interval %+v horizon %+v, want actuals to the cutoff inside the quarter %+v", frame.Interval, frame.Horizon, basis.Period)
	}
}

// Two deals at 20%, worth 100,000 and 250,000: nothing in about 64% of
// outcomes, so the low end is 0 and the high end is 250,000. Their chances
// carry evidence, so the range is simulated in whole batches.
func TestASnapshotFreezesItsRange(t *testing.T) {
	e := setupForecast(t)
	id := e.freezeTwoTeams(t, 100_000, 250_000)

	var low, high int64
	var runs int
	var quantiles, version string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT range_low_minor, range_high_minor, range_runs, range_quantiles, range_version
		   FROM forecast_snapshot WHERE id = $1`, id).Scan(&low, &high, &runs, &quantiles, &version); err != nil {
		t.Fatalf("reading the frozen range: %v", err)
	}
	if low != 0 || high != 250_000 || runs == 0 || runs%1000 != 0 {
		t.Fatalf("frozen range %d..%d over %d outcomes, want 0..250,000 over whole simulated batches", low, high, runs)
	}
	if quantiles != forecasting.RangeQuantiles || version != forecasting.RangeVersion {
		t.Fatalf("frozen convention %q and rules %q, want %q and %q", quantiles, version, forecasting.RangeQuantiles, forecasting.RangeVersion)
	}
}

// issueFrozenShare freezes the seeded deals and issues a share of that state.
func (e *forecastEnv) issueFrozenShare(t *testing.T) string {
	t.Helper()
	id := e.freezeWorkspace(t)
	_, token := e.issue(e.writerCtx(t), t, NewShare{
		Kind: shareKindSnapshot, Target: "someone@example.test",
		Scope: forecasting.Scope{Kind: forecasting.ScopeWorkspace}, SnapshotID: &id,
		ExpiresAt: clocktest.Now(t).UTC().Add(24 * time.Hour),
	})
	return token
}

// openSharedSnapshot opens a share as reader, over HTTP and as its CSV. Each CSV
// row comes back keyed by its column heading.
func (e *forecastEnv) openSharedSnapshot(
	reader context.Context, t *testing.T, token string,
) (crmcontracts.SharedForecastView, []map[string]string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/forecast/shared/"+token, nil).WithContext(reader)
	rec := httptest.NewRecorder()
	e.shareRoutes().OpenForecastShare(rec, req, token)
	var view crmcontracts.SharedForecastView
	if rec.Code != http.StatusOK {
		t.Fatalf("opening the share = %d %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatalf("decoding the shared view: %v", err)
	}
	status, body := e.exportShare(reader, t, token)
	records, err := csv.NewReader(strings.NewReader(body)).ReadAll()
	if status != http.StatusOK || err != nil || len(records) < 2 {
		t.Fatalf("exporting the share = %d, %v, %d records", status, err, len(records))
	}
	rows := make([]map[string]string, 0, len(records)-1)
	for _, record := range records[1:] {
		row := map[string]string{}
		for index, heading := range records[0] {
			row[heading] = record[index]
		}
		rows = append(rows, row)
	}
	return view, rows
}

// sharedRangeOf returns the range a share states over HTTP and in its CSV. The
// two must agree.
func (e *forecastEnv) sharedRangeOf(reader context.Context, t *testing.T, token string) (low, high int64) {
	t.Helper()
	view, rows := e.openSharedSnapshot(reader, t, token)
	if view.Readings.RangeLowMinor == nil || view.Readings.RangeHighMinor == nil {
		t.Fatal("the shared view states no range")
	}
	low, high = *view.Readings.RangeLowMinor, *view.Readings.RangeHighMinor
	if want := fmt.Sprintf("%d..%d", low, high); rows[0]["range_low_minor"]+".."+rows[0]["range_high_minor"] != want {
		t.Fatalf("the CSV states the range %s..%s, the shared view %d..%d",
			rows[0]["range_low_minor"], rows[0]["range_high_minor"], low, high)
	}
	return low, high
}

// A team-scoped reader sees only their own 100,000 deal. At 20% it is lost in
// eight of ten outcomes, so their high end is 100,000, not the stored 250,000.
func TestARestrictedShareDrawsItsRangeOverTheRowsItsReaderSees(t *testing.T) {
	e := setupForecast(t)
	e.seedDealClosingWithin(t, "Mine", &e.Rep1, 100_000)
	e.seedDealClosingWithin(t, "Theirs", &e.Rep3, 250_000)
	token := e.issueFrozenShare(t)

	if low, high := e.sharedRangeOf(snapshotWriterCtx(e.WS), t, token); low != 0 || high != 250_000 {
		t.Fatalf("a reader who sees every frozen row draws %d..%d, want the frozen 0..250,000", low, high)
	}
	narrow := e.forecastReader(e.dealReadCtx(e.Rep1, []ids.UUID{e.Team1}, principal.RowScopeTeam))
	if low, high := e.sharedRangeOf(narrow, t, token); low != 0 || high != 100_000 {
		t.Fatalf("a team-scoped reader draws %d..%d, want 0..100,000 over their own deal alone", low, high)
	}
}

// With individual performance off the export lists only the reader's own deal.
// The totals still count colleagues' deals, and so does the range beside them,
// so it is the range the reader saw with the switch on.
func TestIndividualPerformanceOffKeepsTheSharedRangeOverTheRowsTheTotalsCount(t *testing.T) {
	e := setupForecast(t)
	members := e.addMembers(t, e.Team2, "two@forecast.test", "three@forecast.test")
	e.seedDealClosingWithin(t, "Mine", &e.Rep1, 100_000)
	for _, owner := range append([]ids.UUID{e.Rep3}, members...) {
		e.seedDealClosingWithin(t, "Theirs", &owner, 250_000)
	}
	token := e.issueFrozenShare(t)
	reader := e.forecastReader(e.dealReadCtx(e.Rep1, nil, principal.RowScopeAll))

	onLow, onHigh := e.sharedRangeOf(reader, t, token)
	e.showIndividualPerformance(t, false)
	offLow, offHigh := e.sharedRangeOf(reader, t, token)
	if offLow != onLow || offHigh != onHigh {
		t.Fatalf("with the switch off the range is %d..%d, with it on %d..%d; the totals count the same rows", offLow, offHigh, onLow, onHigh)
	}
	if onHigh <= 100_000 {
		t.Fatalf("the high end %d covers the reader's own deal alone, want colleagues' deals counted", onHigh)
	}
}

// With individual performance off the export holds the reader's own deal while
// the headline sums the team's. The view and every CSV row say so.
func TestASharedSnapshotSaysWhenItsExportHoldsOnlyTheReadersOwnDeals(t *testing.T) {
	e := setupForecast(t)
	members := e.addMembers(t, e.Team2, "two@forecast.test", "three@forecast.test")
	e.seedDealClosingWithin(t, "Mine", &e.Rep1, 100_000)
	for _, owner := range append([]ids.UUID{e.Rep3}, members...) {
		e.seedDealClosingWithin(t, "Theirs", &owner, 250_000)
	}
	token := e.issueFrozenShare(t)
	reader := e.forecastReader(e.dealReadCtx(e.Rep1, nil, principal.RowScopeAll))

	for _, c := range []struct {
		switchOn bool
		want     bool
		rows     int
	}{{true, false, 4}, {false, true, 1}} {
		e.showIndividualPerformance(t, c.switchOn)
		view, rows := e.openSharedSnapshot(reader, t, token)
		if got := view.Readings.ContributionsLimited; got == nil || *got != c.want {
			t.Errorf("switch on %v: the view says contributions_limited %v, want %v", c.switchOn, got, c.want)
		}
		if len(rows) != c.rows {
			t.Errorf("switch on %v: the export holds %d rows, want %d", c.switchOn, len(rows), c.rows)
		}
		for _, row := range rows {
			if row["contributions_limited"] != fmt.Sprint(c.want) {
				t.Errorf("switch on %v: a CSV row says contributions_limited %q, want %v", c.switchOn, row["contributions_limited"], c.want)
			}
		}
		if view.Readings.OpenMinor != 850_000 {
			t.Errorf("switch on %v: the headline sums %d open, want the team's 850,000", c.switchOn, view.Readings.OpenMinor)
		}
	}
}

// A reader who owns none of the frozen deals gets one summary row, so the file
// still says the headline counts deals withheld from them.
func TestASharedSnapshotWithEveryRowWithheldExportsOneSummaryRow(t *testing.T) {
	e := setupForecast(t)
	members := e.addMembers(t, e.Team2, "two@forecast.test", "three@forecast.test")
	for _, owner := range append([]ids.UUID{e.Rep3}, members...) {
		e.seedDealClosingWithin(t, "Theirs", &owner, 250_000)
	}
	token := e.issueFrozenShare(t)
	reader := e.forecastReader(e.dealReadCtx(e.Rep1, nil, principal.RowScopeAll))
	e.showIndividualPerformance(t, false)

	view, rows := e.openSharedSnapshot(reader, t, token)
	if got := view.Readings.ContributionsLimited; got == nil || !*got {
		t.Errorf("the view says contributions_limited %v, want true", got)
	}
	if len(rows) != 1 || rows[0]["deal_id"] != "" || rows[0]["deal_name"] != "" ||
		rows[0]["contributions_limited"] != "true" || rows[0]["range_high_minor"] == "" {
		t.Fatalf("the export rows are %v, want one summary row with no deal, the range and contributions_limited true", rows)
	}
}
