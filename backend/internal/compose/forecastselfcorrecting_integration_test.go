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
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/clocktest"
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
	return f.forecastOverHTTPAs(t, f.forecastReader(), today)
}

func (f *stageCohortFixture) forecastOverHTTPAs(t *testing.T, reader context.Context, today time.Time) crmcontracts.ForecastReadings {
	t.Helper()
	handlers := newForecastHandlers(f.env.Pool, func() time.Time { return today })
	workspace, with := crmcontracts.GetForecastParamsScopeKindWorkspace, true
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/forecast", nil).WithContext(reader)
	handlers.GetForecast(rec, req, crmcontracts.GetForecastParams{
		AsOf: &openapi_types.Date{Time: today}, ScopeKind: &workspace, Contributions: &with,
	})
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
	return f.forecastOverToolAs(t, f.forecastReader(), today)
}

func (f *stageCohortFixture) forecastOverToolAs(t *testing.T, reader context.Context, today time.Time) agents.ForecastReadingsResult {
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

	overTool := f.forecastOverTool(t, today)
	if overTool.WeightedMinor != overHTTP.WeightedMinor || overTool.OpenMinor != overHTTP.OpenMinor {
		t.Fatalf("the tool answered weighted %d open %d, the HTTP door %d and %d", overTool.WeightedMinor, overTool.OpenMinor, overHTTP.WeightedMinor, overHTTP.OpenMinor)
	}
	if wire := asToolRows(t, *overHTTP.Contributions); !reflect.DeepEqual(wire, overTool.Contributions) {
		t.Fatalf("the tool's rows differ from the HTTP door's:\n tool %+v\n http %+v", overTool.Contributions, wire)
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

	overHTTP := f.forecastOverHTTPAs(t, reader, today)
	overTool := f.forecastOverToolAs(t, reader, today)
	for door, rows := range map[string][]agents.ForecastContributionResult{
		"http": asToolRows(t, *overHTTP.Contributions), "tool": overTool.Contributions,
	} {
		if len(rows) != 1 || rows[0].DealID != own.String() || rows[0].OwnerID != nil {
			t.Fatalf("%s: rows %+v, want only the reader's deal %s and no owner on it", door, rows, own)
		}
	}
	if overHTTP.OpenMinor != int64(1_000_000*(1+len(colleagues))) {
		t.Fatalf("open = %d, want every deal in the total", overHTTP.OpenMinor)
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
