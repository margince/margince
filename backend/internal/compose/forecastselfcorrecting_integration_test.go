// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A stage's chance in use starts at the typed chance and moves only with the
// outcomes of deals recorded entering the stage. The HTTP door and the tool read
// one assembly, and the installation setting turns it all off.

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// forecastReader may read every deal, the forecast and the settings it is cut by.
func (f *stageCohortFixture) forecastReader() context.Context {
	read := principal.ObjectGrant{Read: true}
	ctx := principal.WithWorkspaceID(context.Background(), f.env.WS)
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + f.env.Rep1.String(), UserID: f.env.Rep1, SeatType: principal.SeatFull,
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll, Objects: map[string]principal.ObjectGrant{
			"deal": read, "forecast": read, "installation_settings": read, "pipeline": read,
		}},
	})
}

// openDeal is the deal the forecast weighs: the reader's own, priced and due today.
func (f *stageCohortFixture) openDeal(t *testing.T, stage ids.UUID, today time.Time) ids.UUID {
	t.Helper()
	return f.openDealOf(t, f.env.Rep1, stage, today)
}

func (f *stageCohortFixture) openDealOf(t *testing.T, seat, stage ids.UUID, today time.Time) ids.UUID {
	t.Helper()
	owner, amount, currency := ids.From[ids.UserKind](seat), int64(1_000_000), "EUR"
	created, err := f.store.CreateDeal(f.writer, deals.CreateDealInput{
		Name: "Weighed deal", PipelineID: ids.From[ids.PipelineKind](f.env.pipeline), StageID: ids.From[ids.StageKind](stage),
		OwnerID: &owner, AmountMinor: &amount, Currency: &currency, ExpectedClose: &today, Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	f.at = f.at.Add(time.Hour)
	return ids.UUID(created.Id)
}

// forecastOverHTTP reads GET /forecast with its contributions.
func (f *stageCohortFixture) forecastOverHTTP(t *testing.T, today time.Time) crmcontracts.ForecastReadings {
	t.Helper()
	return f.forecastOverHTTPAs(f.forecastReader(), t, today)
}

func (f *stageCohortFixture) forecastOverHTTPAs(reader context.Context, t *testing.T, today time.Time) crmcontracts.ForecastReadings {
	t.Helper()
	handlers := newForecastHandlers(f.env.Pool, func() time.Time { return today })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/forecast?as_of="+today.Format(time.DateOnly)+"&scope_kind=workspace&contributions=true", nil).WithContext(reader)
	crmcontracts.Handler(Server{forecastHandlers: handlers}).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /forecast answered %d: %s", rec.Code, rec.Body.String())
	}
	var out crmcontracts.ForecastReadings
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// forecastOverTool reads forecast_readings with its contributions.
func (f *stageCohortFixture) forecastOverTool(t *testing.T, today time.Time) agents.ForecastReadingsResult {
	t.Helper()
	return f.forecastOverToolAs(f.forecastReader(), t, today)
}

func (f *stageCohortFixture) forecastOverToolAs(reader context.Context, t *testing.T, today time.Time) agents.ForecastReadingsResult {
	t.Helper()
	raw, err := forecastToolReader(f.env.Pool)(reader, agents.ForecastRequest{
		AsOf: today.Format(time.DateOnly), ScopeKind: "workspace", Contributions: true,
	})
	if err != nil {
		t.Fatalf("forecast_readings: %v", err)
	}
	var out agents.ForecastReadingsResult
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func contributionOfDeal(t *testing.T, rows []crmcontracts.ForecastContribution, deal ids.UUID) crmcontracts.ForecastContribution {
	t.Helper()
	for _, row := range rows {
		if ids.UUID(row.DealId) == deal {
			return row
		}
	}
	t.Fatalf("deal %s is not among the %d contributions", deal, len(rows))
	return crmcontracts.ForecastContribution{}
}

// With no recorded outcome from its stage, a deal is weighted by the typed
// chance: today's forecast, unchanged.
func TestWithNoRecordedOutcomeTheForecastUsesTheTypedChance(t *testing.T) {
	f := newStageCohortFixture(t)
	today := clocktest.Now(t)
	deal := f.openDeal(t, f.stage(0), today)

	got := f.forecastOverHTTP(t, today)
	row := contributionOfDeal(t, *got.Contributions, deal)
	if row.TypedProbability == nil || *row.TypedProbability != 20 || row.ChanceInUse != 20 || row.AppliedProbability != 20 {
		t.Fatalf("chances = typed %v, in use %d, applied %d; want 20 each", row.TypedProbability, row.ChanceInUse, row.AppliedProbability)
	}
	if row.EvidenceN == nil || *row.EvidenceN != 0 || row.Effect != nil {
		t.Fatalf("evidence %v and effect %v, want 0 and none", row.EvidenceN, row.Effect)
	}
	if got.WeightedMinor != 200_000 || row.WeightedMinor != 200_000 {
		t.Fatalf("weighted = %d (row %d), want the typed 20%% of 1,000,000", got.WeightedMinor, row.WeightedMinor)
	}
	if got.SelfCorrecting == nil || !*got.SelfCorrecting {
		t.Fatalf("self_correcting = %v, want true by default", got.SelfCorrecting)
	}
	if got.ContributionsLimited == nil || *got.ContributionsLimited {
		t.Fatalf("contributions_limited = %v, want false while individual performance is on", got.ContributionsLimited)
	}
}

// Twenty recorded wins from a stage typed at 20% move its chance to 60%. A move
// an import made into the stage, and one saved before moves kept their origin,
// count for nothing even though both ended lost. The tool answers the same
// numbers as the HTTP door.
func TestRecordedOutcomesMoveTheChanceAndImportedOrLegacyMovesDoNot(t *testing.T) {
	f := newStageCohortFixture(t)
	s0, s1 := f.stage(0), f.stage(1)
	for range 20 {
		f.deal(t, f.env.Rep1, s0, f.won)
	}
	importedEntry := f.deal(t, f.env.Rep1, s1)
	f.importMove(f.importer(t), t, importedEntry, s0)
	f.lose(t, importedEntry)
	legacy := f.deal(t, f.env.Rep1, s0, f.lost)
	f.forgetProvenance(t, legacy, &s0)

	today := clocktest.Now(t)
	deal := f.openDeal(t, s0, today)
	overHTTP := f.forecastOverHTTP(t, today)
	row := contributionOfDeal(t, *overHTTP.Contributions, deal)
	if row.ChanceInUse != 60 || row.EvidenceN == nil || *row.EvidenceN != 20 {
		t.Fatalf("chance in use %d from %v outcomes, want 60 from the 20 recorded wins alone", row.ChanceInUse, row.EvidenceN)
	}
	if row.WeightedMinor != 600_000 {
		t.Fatalf("weighted = %d, want 60%% of 1,000,000", row.WeightedMinor)
	}
	var stages forecasting.StageChances
	if err := database.WithWorkspaceTx(f.forecastReader(), f.env.Pool, func(tx pgx.Tx) error {
		var err error
		stages, err = ForecastStageChances(f.forecastReader(), tx, today)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	read := slices.IndexFunc(stages.Stages, func(stage forecasting.StageChance) bool { return stage.StageID == s0 })
	if read < 0 {
		t.Fatal("the stage chances leave out the first stage")
	}
	if stage := stages.Stages[read]; stage.ChanceInUse != 60 || stage.Evidence.Resolved != 20 || stage.Typed != 20 {
		t.Fatalf("Settings reads stage %+v, want typed 20, 20 outcomes and 60 in use as the forecast does", stage)
	}

	overTool := f.forecastOverTool(t, today)
	if overTool.WeightedMinor != overHTTP.WeightedMinor || overTool.OpenMinor != overHTTP.OpenMinor {
		t.Fatalf("the tool answered weighted %d open %d, the HTTP door %d and %d", overTool.WeightedMinor, overTool.OpenMinor, overHTTP.WeightedMinor, overHTTP.OpenMinor)
	}
	if wire := asToolRows(t, *overHTTP.Contributions); !reflect.DeepEqual(wire, overTool.Contributions) {
		for index := range wire {
			if index >= len(overTool.Contributions) {
				break
			}
			if reflect.DeepEqual(wire[index], overTool.Contributions[index]) {
				continue
			}
			tool, err := json.Marshal(overTool.Contributions[index])
			if err != nil {
				t.Fatal(err)
			}
			http, err := json.Marshal(wire[index])
			if err != nil {
				t.Fatal(err)
			}
			t.Fatalf("forecast row %d differs: tool %s; HTTP %s", index, tool, http)
		}
		t.Fatalf("forecast contribution lengths differ: tool %d; HTTP %d", len(overTool.Contributions), len(wire))
	}
}

// asToolRows reads the HTTP door's rows in the tool's shape, so the two compare
// field by field.
func asToolRows(t *testing.T, rows []crmcontracts.ForecastContribution) []agents.ForecastContributionResult {
	t.Helper()
	encoded, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	var out []agents.ForecastContributionResult
	if err := json.Unmarshal(encoded, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// While individual performance is off, both doors return only the reader's
// own deals, with no owner on them. The colleagues' deals still count in the
// totals.
func TestIndividualPerformanceOffKeepsOnlyTheReadersOwnRows(t *testing.T) {
	f := newStageCohortFixture(t)
	s0 := f.stage(0)
	today := clocktest.Now(t)
	others := f.env.addMembers(t, f.env.Team2, "fourth@forecast.test")
	own := f.openDeal(t, s0, today)
	var colleagues []ids.UUID
	for _, owner := range append(others, f.env.Rep3) {
		colleagues = append(colleagues, f.openDealOf(t, owner, s0, today))
	}
	reader := auth.WithIndividualPerformance(f.forecastReader(), false)

	overHTTP := f.forecastOverHTTPAs(reader, t, today)
	overTool := f.forecastOverToolAs(reader, t, today)
	for door, rows := range map[string][]agents.ForecastContributionResult{
		"http": asToolRows(t, *overHTTP.Contributions), "tool": overTool.Contributions,
	} {
		if len(rows) != 1 || rows[0].DealID != own.String() || rows[0].OwnerID != nil {
			t.Fatalf("%s: rows %+v, want only the reader's deal %s and no owner on it", door, rows, own)
		}
	}
	// The rows and the total differ, and both doors say why.
	if overHTTP.OpenMinor != int64(1_000_000*(1+len(colleagues))) {
		t.Fatalf("open = %d, want every deal in the total", overHTTP.OpenMinor)
	}
	if overHTTP.ContributionsLimited == nil || !*overHTTP.ContributionsLimited ||
		overTool.ContributionsLimited == nil || !*overTool.ContributionsLimited {
		t.Fatalf("contributions_limited = %v over HTTP and %v over the tool, want true on both",
			overHTTP.ContributionsLimited, overTool.ContributionsLimited)
	}
}

// A seller reading their own forecast with the switch off loses no row, so
// neither door says the rows are limited.
func TestIndividualPerformanceOffLimitsNoRowsOfASellersOwnForecast(t *testing.T) {
	f := newStageCohortFixture(t)
	s0 := f.stage(0)
	today := clocktest.Now(t)
	f.openDeal(t, s0, today)
	f.openDealOf(t, f.env.Rep3, s0, today)
	reader := auth.WithIndividualPerformance(f.forecastReader(), false)

	owner, with := crmcontracts.GetForecastParamsScopeKindOwner, true
	seller := openapi_types.UUID(f.env.Rep1)
	rec := httptest.NewRecorder()
	newForecastHandlers(f.env.Pool, func() time.Time { return today }).GetForecast(rec,
		httptest.NewRequest(http.MethodGet, "/forecast", nil).WithContext(reader),
		crmcontracts.GetForecastParams{AsOf: &openapi_types.Date{Time: today}, ScopeKind: &owner, ScopeId: &seller, Contributions: &with})
	var overHTTP crmcontracts.ForecastReadings
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &overHTTP) != nil {
		t.Fatalf("GET /forecast for the seller answered %d: %s", rec.Code, rec.Body.String())
	}
	raw, err := forecastToolReader(f.env.Pool)(reader, agents.ForecastRequest{
		AsOf: today.Format(time.DateOnly), ScopeKind: "owner", ScopeID: f.env.Rep1, Contributions: true,
	})
	var overTool agents.ForecastReadingsResult
	if err != nil || json.Unmarshal(raw, &overTool) != nil {
		t.Fatalf("forecast_readings for the seller: %v", err)
	}
	if overHTTP.Contributions == nil || len(*overHTTP.Contributions) != 1 || len(overTool.Contributions) != 1 {
		t.Fatalf("the seller's rows: %v over HTTP, %d over the tool, want their one deal", overHTTP.Contributions, len(overTool.Contributions))
	}
	if overHTTP.ContributionsLimited == nil || *overHTTP.ContributionsLimited ||
		overTool.ContributionsLimited == nil || *overTool.ContributionsLimited {
		t.Fatalf("contributions_limited = %v over HTTP and %v over the tool, want false on both",
			overHTTP.ContributionsLimited, overTool.ContributionsLimited)
	}
}

// Off, every deal is weighted by its typed chance whatever was recorded.
func TestSelfCorrectionOffWeighsByTheTypedChance(t *testing.T) {
	f := newStageCohortFixture(t)
	s0 := f.stage(0)
	for range 20 {
		f.deal(t, f.env.Rep1, s0, f.won)
	}
	if _, err := f.env.owner.Exec(context.Background(), `INSERT INTO setting (key, value)
		VALUES ('installation.forecast_self_correcting', 'false'::jsonb)`); err != nil {
		t.Fatal(err)
	}
	today := clocktest.Now(t)
	deal := f.openDeal(t, s0, today)

	got := f.forecastOverHTTP(t, today)
	row := contributionOfDeal(t, *got.Contributions, deal)
	if row.ChanceInUse != 20 || row.EvidenceN != nil || len(row.Signals) != 0 || row.WeightedMinor != 200_000 {
		t.Fatalf("off: chance %d, evidence %v, signals %v, weighted %d; want the typed 20%% and nothing else", row.ChanceInUse, row.EvidenceN, row.Signals, row.WeightedMinor)
	}
	if got.SelfCorrecting == nil || *got.SelfCorrecting {
		t.Fatalf("self_correcting = %v, want false", got.SelfCorrecting)
	}
}

// lose closes a deal lost by hand.
func (f *stageCohortFixture) lose(t *testing.T, deal ids.DealID) {
	t.Helper()
	reason := "price"
	if _, err := f.store.AdvanceDeal(f.writer, deal, deals.AdvanceDealInput{ToStageID: ids.From[ids.StageKind](f.lost), LostReason: &reason}); err != nil {
		t.Fatal(err)
	}
	f.at = f.at.Add(time.Hour)
}

// A signal earns an effect only from frozen forecasts whose deals then closed.
// Ten frozen with the customer quiet were lost and ten frozen without it won,
// so quiet lowers the odds. A deal still open, one lost after the cutoff, and a
// row frozen under other signal rules count for nothing. A signal frozen on no
// deal stays at 1.
func TestARiskEffectIsLearnedFromSettledFrozenForecasts(t *testing.T) {
	f := newStageCohortFixture(t)
	s0 := f.stage(0)
	var quiet, calm []ids.DealID
	for range 10 {
		quiet = append(quiet, f.deal(t, f.env.Rep1, s0))
		calm = append(calm, f.deal(t, f.env.Rep1, s0))
	}
	rows := make([]forecasting.Contribution, 0, 20)
	for i := range quiet {
		rows = append(rows,
			forecasting.Contribution{DealID: quiet[i].String(), InOpen: true, Signals: []forecasting.Signal{{Code: assurance.SignalQuiet}}},
			forecasting.Contribution{DealID: calm[i].String(), InOpen: true, Signals: []forecasting.Signal{}})
	}
	stillOpen := f.deal(t, f.env.Rep1, s0)
	rows = append(rows, forecasting.Contribution{DealID: stillOpen.String(), InOpen: true, Signals: []forecasting.Signal{{Code: assurance.SignalQuiet}}})
	f.freezeSignals(t, rows, forecasting.TriggerDaily, assurance.RiskSignalsVersion)
	f.freezeSignals(t, []forecasting.Contribution{
		{DealID: calm[0].String(), InOpen: true, Signals: []forecasting.Signal{{Code: assurance.SignalQuiet}}},
	}, forecasting.TriggerRecheck, "v0")
	lateLoss := f.deal(t, f.env.Rep1, s0)
	f.freezeSignals(t, []forecasting.Contribution{
		{DealID: lateLoss.String(), InOpen: true, Signals: []forecasting.Signal{{Code: assurance.SignalQuiet}}},
	}, forecasting.TriggerPeriodClose, assurance.RiskSignalsVersion)
	for i := range quiet {
		f.lose(t, quiet[i])
		f.win(t, calm[i])
	}
	cutoff := f.at
	f.lose(t, lateLoss)

	var effects map[string]float64
	if err := database.WithWorkspaceTx(f.writer, f.env.Pool, func(tx pgx.Tx) error {
		var err error
		effects, err = forecastRiskEffects(f.writer, tx, cutoff)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	want := forecasting.RiskEffect(forecasting.RiskTally{PresentLost: 10, AbsentWon: 10})
	if got := effects[assurance.SignalQuiet]; math.Abs(got-want) > 1e-12 || got >= 1 {
		t.Fatalf("quiet effect = %v, want %v from ten lost against ten won", got, want)
	}
	if got := effects[assurance.SignalStuck]; got != 1 {
		t.Fatalf("stuck effect = %v, want 1: no frozen deal carried it", got)
	}
}

// freezeSignals takes a daily snapshot of the rows through the snapshot writer.
func (f *stageCohortFixture) freezeSignals(t *testing.T, rows []forecasting.Contribution, trigger, version string) {
	t.Helper()
	period, err := forecasting.ResolvePeriod(forecasting.PeriodQuarter, f.at, 1, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	store := newForecastStoreFor(f.env.Pool)
	if err := store.InTx(f.writer, func(ctx context.Context, tx pgx.Tx) error {
		_, err := store.TakeSnapshot(ctx, tx, forecasting.NewSnapshot{
			Period: period, Scope: forecasting.Scope{Kind: forecasting.ScopeWorkspace},
			Trigger: trigger, BaseCurrency: "EUR", TakenAt: f.at,
			Readings: forecasting.Readings{Contributions: rows}, RiskSignalsVersion: version,
		})
		return err
	}); err != nil {
		t.Fatalf("freezing the signals: %v", err)
	}
	f.at = f.at.Add(time.Hour)
}

// win closes a deal won by hand.
func (f *stageCohortFixture) win(t *testing.T, deal ids.DealID) {
	t.Helper()
	reason := "purchase_order"
	if _, err := f.store.AdvanceDeal(f.writer, deal, deals.AdvanceDealInput{ToStageID: ids.From[ids.StageKind](f.won), WonWithoutContractReason: &reason}); err != nil {
		t.Fatal(err)
	}
	f.at = f.at.Add(time.Hour)
}

// A deal won before the cutoff and reopened after it still counts as won: the
// evidence reads how each deal stood at the cutoff.
func TestStageEvidenceReadsHowADealStoodAtTheCutoff(t *testing.T) {
	f := newStageCohortFixture(t)
	s0, s1 := f.stage(0), f.stage(1)
	var won []ids.DealID
	for range 20 {
		won = append(won, f.deal(t, f.env.Rep1, s0, f.won))
	}
	cutoff := f.at
	for _, deal := range won[:10] {
		if _, err := f.store.AdvanceDeal(f.writer, deal, deals.AdvanceDealInput{ToStageID: ids.From[ids.StageKind](s1)}); err != nil {
			t.Fatalf("reopening: %v", err)
		}
		f.at = f.at.Add(time.Hour)
	}

	var evidence map[string]forecasting.StageEvidence
	if err := database.WithWorkspaceTx(f.writer, f.env.Pool, func(tx pgx.Tx) error {
		var err error
		evidence, err = forecastStageEvidence(f.writer, tx, cutoff)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got := evidence[s0.String()]; got.Resolved != 20 || got.WonShare != 1 {
		t.Fatalf("first stage evidence %+v, want 20 resolved and all won as they stood at the cutoff", got)
	}
}

// The stuck pace learns from recorded stints only, and an archived win still
// counts. Five won deals meet the floor; one of them archived keeps it met,
// and one whose move predates provenance takes it below.
func TestTheStuckPaceReadsRecordedStintsAndKeepsArchivedWins(t *testing.T) {
	for name, tc := range map[string]struct {
		archive, forget bool
		paced           bool
	}{
		"an archived win counts":       {archive: true, paced: true},
		"a legacy move does not count": {forget: true, paced: false},
	} {
		t.Run(name, func(t *testing.T) {
			f := newStageCohortFixture(t)
			s0 := f.stage(0)
			var won []ids.DealID
			for range 5 {
				won = append(won, f.deal(t, f.env.Rep1, s0, f.won))
			}
			if tc.archive {
				if _, err := f.env.owner.Exec(context.Background(), `UPDATE deal SET archived_at = now() WHERE id = $1`, won[0].UUID); err != nil {
					t.Fatal(err)
				}
			}
			if tc.forget {
				f.forgetProvenance(t, won[0], &s0)
			}
			var pace map[string]time.Duration
			if err := database.WithWorkspaceTx(f.writer, f.env.Pool, func(tx pgx.Tx) error {
				var err error
				pace, err = deals.StuckAfter(f.writer, tx, f.env.pipeline, analyticsquery.PercentileSampleFloor)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if _, ok := pace[s0.String()]; ok != tc.paced {
				t.Fatalf("first stage paced = %v, want %v", ok, tc.paced)
			}
		})
	}
}

// The forecast report's weighted total, drawn beside the headline, weights each
// deal by the chance the headline uses, recorded evidence included. Off, both
// weight by the typed chance.
func TestTheForecastReportWeightsByTheChanceTheHeadlineUses(t *testing.T) {
	f := newStageCohortFixture(t)
	s0 := f.stage(0)
	for range 20 {
		f.deal(t, f.env.Rep1, s0, f.won)
	}
	today := clocktest.Now(t)
	f.openDeal(t, s0, today)
	reportWeighted := func() int64 {
		t.Helper()
		result := f.env.runReport(f.env.Admin(), t, "forecast", `{"group_by":["currency"]}`)
		for _, row := range result.Rows {
			if row["currency"] == "EUR" {
				return wireInt(t, row, "weighted_minor")
			}
		}
		t.Fatalf("no EUR row in the forecast report: %+v", result.Rows)
		return 0
	}

	headline := f.forecastOverHTTP(t, today).WeightedMinor
	if got := reportWeighted(); got != headline || headline != 600_000 {
		t.Fatalf("report weighted %d, headline %d; want both 600000 from the chance in use", got, headline)
	}
	if _, err := f.env.owner.Exec(context.Background(), `INSERT INTO setting (key, value)
		VALUES ('installation.forecast_self_correcting', 'false'::jsonb)`); err != nil {
		t.Fatal(err)
	}
	headline = f.forecastOverHTTP(t, today).WeightedMinor
	if got := reportWeighted(); got != headline || headline != 200_000 {
		t.Fatalf("off: report weighted %d, headline %d; want both the typed 200000", got, headline)
	}
}
