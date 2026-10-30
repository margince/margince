// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A manager's call keeps the reading it was made against, and the input check
// raises one finding when the pipeline under the call moved materially. Calls
// go through the real forecast handler, deals change through the deals
// store, and the check runs through the job a human starts.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type callEnv struct {
	*assuranceJobEnv
	handlers forecasting.Handlers
	deal     ids.UUID
}

// setupCall starts the check on a workspace whose fixture deal closes inside
// this quarter, so the call's reading counts it.
func setupCall(t *testing.T) *callEnv {
	t.Helper()
	e := &callEnv{assuranceJobEnv: setupAssuranceJob(t)}
	e.handlers = newForecastHandlers(e.Pool, func() time.Time { return e.at })
	if err := e.Pool.QueryRow(e.Admin(), `SELECT id FROM deal WHERE name = 'Assurance Fixture'`).Scan(&e.deal); err != nil {
		t.Fatalf("finding the fixture deal: %v", err)
	}
	var period forecasting.Period
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		var err error
		period, _, err = ForecastPeriodAt(e.Admin(), tx, forecasting.PeriodQuarter, e.at)
		return err
	}); err != nil {
		t.Fatalf("resolving the quarter: %v", err)
	}
	e.updateDeal(t, deals.UpdateDealInput{ExpectedClose: new(period.EndDate)})
	if err := e.run(t); err != nil {
		t.Fatalf("starting the check: %v", err)
	}
	return e
}

func (e *callEnv) updateDeal(t *testing.T, in deals.UpdateDealInput) {
	t.Helper()
	if _, err := e.Deals.UpdateDeal(e.Admin(), ids.From[ids.DealKind](e.deal), in); err != nil {
		t.Fatalf("editing the deal: %v", err)
	}
}

// recordCall calls the quarter through the forecast handler.
func (e *callEnv) recordCall(t *testing.T, amount int64) crmcontracts.ForecastCall {
	t.Helper()
	body, err := json.Marshal(crmcontracts.NewForecastCall{AmountMinor: amount, Currency: "EUR"})
	if err != nil {
		t.Fatal(err)
	}
	return e.postCall(t, body)
}

func (e *callEnv) postCall(t *testing.T, body []byte) crmcontracts.ForecastCall {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/forecast/calls", bytes.NewReader(body)).WithContext(e.Admin())
	w := httptest.NewRecorder()
	e.handlers.RecordForecastCall(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("recording the call answered %d: %s", w.Code, w.Body.String())
	}
	var call crmcontracts.ForecastCall
	if err := json.Unmarshal(w.Body.Bytes(), &call); err != nil {
		t.Fatal(err)
	}
	return call
}

// callFindings are the call findings the list endpoint serves an admin.
func (e *callEnv) callFindings(t *testing.T) []assurance.Exception {
	t.Helper()
	return e.callFindingsAs(e.Admin(), t)
}

func (e *callEnv) callFindingsAs(reader context.Context, t *testing.T) []assurance.Exception {
	t.Helper()
	var out []assurance.Exception
	if err := assurance.NewStore(InstallationDB(e.Pool)).InTx(reader, func(ctx context.Context, tx pgx.Tx) error {
		found, err := AssuranceExceptionsIn(ctx, tx, assurance.Population{})
		for _, f := range found {
			if f.SubjectKind == assurance.SubjectKindForecastCall {
				out = append(out, f)
			}
		}
		return err
	}); err != nil {
		t.Fatalf("reading the findings: %v", err)
	}
	return out
}

func (e *callEnv) check(t *testing.T) {
	t.Helper()
	if err := e.run(t); err != nil {
		t.Fatalf("the check: %v", err)
	}
}

// The call keeps its baseline. An edit the forecast does not count raises
// nothing, and nor do two days. A real change raises one finding naming the
// deal, however often it is seen. A new call clears it.
func TestAMaterialChangeSinceACallRaisesOneFinding(t *testing.T) {
	e := setupCall(t)
	call := e.recordCall(t, 5_000_000)
	if call.Baseline == nil || *call.Baseline != crmcontracts.ForecastCallBaselineCaptured {
		t.Fatalf("the call answered baseline %v, want captured", call.Baseline)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM forecast_snapshot WHERE call_id = $1 AND trigger = 'call'
		AND input_fingerprint IS NOT NULL`, ids.UUID(call.Id)); n != 1 {
		t.Fatalf("the call kept %d baselines, want 1", n)
	}

	e.check(t)
	e.updateDeal(t, deals.UpdateDealInput{Name: new("Assurance Fixture renamed")})
	later := e.at.Add(48 * time.Hour)
	e.worker.now = func() time.Time { return later }
	e.check(t)
	if found := e.callFindings(t); len(found) != 0 {
		t.Fatalf("a rename and two days raised %d call findings, want none", len(found))
	}

	e.updateDeal(t, deals.UpdateDealInput{AmountMinor: new(int64(9_900_000)), Currency: new("EUR")})
	e.check(t)
	e.check(t)
	found := e.callFindings(t)
	if len(found) != 1 || found[0].SubjectID != ids.UUID(call.Id) || found[0].Type != assurance.TypeForecastCallMoved {
		t.Fatalf("after an amount change the call findings are %+v, want one about the call", found)
	}
	var observed struct {
		DealIDs []string `json:"deal_ids"`
	}
	if err := json.Unmarshal(found[0].Observed, &observed); err != nil || !slices.Contains(observed.DealIDs, e.deal.String()) {
		t.Fatalf("the finding names %v (%v), want the edited deal", observed.DealIDs, err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM assurance_exception WHERE subject_id = $1`, ids.UUID(call.Id)); n != 1 {
		t.Fatalf("two nights stored %d findings about the call, want 1", n)
	}

	e.recordCall(t, 7_000_000)
	e.check(t)
	if found := e.callFindings(t); len(found) != 0 {
		t.Fatalf("a new call left %d call findings open, want none", len(found))
	}
}

// A call recorded before calls kept a baseline says its history is
// unavailable and raises nothing.
func TestACallWithNoBaselineHasUnavailableHistory(t *testing.T) {
	e := setupCall(t)
	legacy := forecasting.NewHandlers(newForecastStoreFor(e.Pool), ForecastDeals, ForecastPeriodAt,
		ForecastWritableScope, ForecastConversionHistory, ForecastForwardMeasure, ForecastScopeHoldsDeals,
		func() time.Time { return e.at })
	e.handlers = legacy
	call := e.recordCall(t, 5_000_000)
	if call.Baseline == nil || *call.Baseline != crmcontracts.ForecastCallBaselineUnavailable {
		t.Fatalf("a call with no baseline answered %v, want unavailable", call.Baseline)
	}
	e.updateDeal(t, deals.UpdateDealInput{AmountMinor: new(int64(9_900_000)), Currency: new("EUR")})
	e.check(t)
	if found := e.callFindings(t); len(found) != 0 {
		t.Fatalf("a call with no baseline raised %+v; nothing can be said about what moved", found)
	}
}

// A call whose window the calendar no longer has is a limitation, not a delta.
func TestACallWhoseQuarterMovedIsALimitation(t *testing.T) {
	e := setupCall(t)
	call := e.recordCall(t, 5_000_000)
	if _, err := integration.OwnerConn(t).Exec(context.Background(), `
		INSERT INTO setting (key, value) VALUES ('installation.fiscal_year_start_month', '2'::jsonb)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`); err != nil {
		t.Fatalf("moving the financial year: %v", err)
	}
	e.check(t)
	found := e.callFindings(t)
	if len(found) != 1 || found[0].SubjectID != ids.UUID(call.Id) {
		t.Fatalf("call findings = %+v, want one about the call", found)
	}
	var observed map[string]any
	if err := json.Unmarshal(found[0].Observed, &observed); err != nil {
		t.Fatal(err)
	}
	if observed["limitation"] != string(forecasting.LimitationPeriod) || observed["open_change_minor"] != nil {
		t.Fatalf("observed = %v, want the period limitation and no change", observed)
	}
}

// A call replaced while the pass reads the calls closes its finding. The pass
// raised that finding from a read taken before the new call landed.
func TestACallSupersededDuringThePassLeavesNoOpenFinding(t *testing.T) {
	e := setupCall(t)
	first := e.recordCall(t, 5_000_000)
	e.updateDeal(t, deals.UpdateDealInput{AmountMinor: new(int64(9_900_000)), Currency: new("EUR")})
	calls := assuranceCallSubjects(e.Pool)
	duringRead := func(ctx context.Context, tx pgx.Tx, now time.Time) ([]assurance.CallSubject, error) {
		subjects, err := calls(ctx, tx, now)
		e.recordCall(t, 7_000_000)
		return subjects, err
	}
	scanner := assuranceScanner(e.Pool, assurance.NewStore(InstallationDB(e.Pool))).WithCallSubjects(duringRead)
	if _, err := scanner.Scan(e.passCtxFor(), e.at, nil); err != nil {
		t.Fatalf("the pass: %v", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM assurance_exception WHERE subject_id = $1 AND status = 'open'`,
		ids.UUID(first.Id)); n != 0 {
		t.Fatal("the replaced call kept an open finding")
	}
}

// A call made for another day keeps that day's window. Its baseline is
// stamped at the server's clock. It is never listed as a capture a movement
// could start from.
func TestACallForAnotherDayKeepsItsWindowAndTheServersClock(t *testing.T) {
	e := setupCall(t)
	var next, last forecasting.Period
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		this, _, err := ForecastPeriodAt(e.Admin(), tx, forecasting.PeriodQuarter, e.at)
		if err != nil {
			return err
		}
		if next, _, err = ForecastPeriodAt(e.Admin(), tx, forecasting.PeriodQuarter, this.End.Add(time.Hour)); err != nil {
			return err
		}
		last, _, err = ForecastPeriodAt(e.Admin(), tx, forecasting.PeriodQuarter, this.Start.Add(-time.Hour))
		return err
	}); err != nil {
		t.Fatalf("resolving the quarters: %v", err)
	}
	for _, period := range []forecasting.Period{next, last} {
		call := e.recordCallFor(t, 5_000_000, period.StartDate)
		if call.PeriodStart.Format(time.DateOnly) != period.StartDate.Format(time.DateOnly) {
			t.Fatalf("a call for %s was filed for %s", period.StartDate, call.PeriodStart)
		}
		var takenAt time.Time
		if err := e.Pool.QueryRow(e.Admin(), `SELECT taken_at FROM forecast_snapshot
			WHERE call_id = $1 AND period_start = $2`, ids.UUID(call.Id), period.StartDate).Scan(&takenAt); err != nil {
			t.Fatalf("the call for %s kept no baseline over its own window: %v", period.StartDate, err)
		}
		if d := takenAt.Sub(e.at); d > time.Second || d < -time.Second {
			t.Fatalf("the baseline is stamped %s, want the server's clock %s", takenAt, e.at)
		}
		if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
			refs, err := newForecastStoreFor(e.Pool).SnapshotRefsTx(e.Admin(), tx, period,
				forecasting.Scope{Kind: forecasting.ScopeWorkspace}, "EUR")
			if err == nil && len(refs) != 0 {
				t.Errorf("a call's baseline is listed as a capture: %+v", refs)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	e.check(t)
	if found := e.callFindings(t); len(found) != 0 {
		t.Fatalf("a call for next quarter read as a limitation: %+v", found)
	}
}

func (e *callEnv) passCtxFor() context.Context {
	return principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.WS), assuranceActor)
}

// recordCallFor calls the period holding day.
func (e *callEnv) recordCallFor(t *testing.T, amount int64, day time.Time) crmcontracts.ForecastCall {
	t.Helper()
	body, err := json.Marshal(crmcontracts.NewForecastCall{
		AmountMinor: amount, Currency: "EUR", AsOf: &openapi_types.Date{Time: day},
	})
	if err != nil {
		t.Fatal(err)
	}
	return e.postCall(t, body)
}

// A call finding is read through the reader's own deal scope and amount mask.
// A team call is the case where they bite: a seat that sees every row is
// shown every amount. The masked manager sees the deal and no money; a seat
// that may not read deals sees only the comparison.
func TestACallFindingIsReadThroughTheReadersScopeAndMask(t *testing.T) {
	e := setupCall(t)
	team := crmcontracts.NewForecastCallScopeKindTeam
	body, err := json.Marshal(crmcontracts.NewForecastCall{
		AmountMinor: 5_000_000, Currency: "EUR", ScopeKind: &team, ScopeId: (*openapi_types.UUID)(&e.Team1),
	})
	if err != nil {
		t.Fatal(err)
	}
	e.postCall(t, body)
	// An unpriced deal joins the team's period: a mask decided from today's
	// value would read its null amount as nothing to hide.
	var pipeline, stage ids.UUID
	var closes time.Time
	if err := e.Pool.QueryRow(e.Admin(), `SELECT pipeline_id, stage_id, expected_close_date FROM deal WHERE id = $1`,
		e.deal).Scan(&pipeline, &stage, &closes); err != nil {
		t.Fatal(err)
	}
	owner := ids.From[ids.UserKind](e.Rep1)
	if _, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Unpriced newcomer", PipelineID: ids.From[ids.PipelineKind](pipeline),
		StageID: ids.From[ids.StageKind](stage), OwnerID: &owner, Source: "manual", ExpectedClose: &closes,
	}); err != nil {
		t.Fatalf("adding the unpriced deal: %v", err)
	}
	e.check(t)

	manager := func(objects map[string]principal.ObjectGrant) context.Context {
		return e.As(e.Rep2, []ids.UUID{e.Team1}, principal.Permissions{
			RoleKeys: []string{"manager"}, Objects: objects, RowScope: principal.RowScopeTeam,
			FieldMasks: []principal.FieldMask{{Object: "deal", Field: "amount_minor", Condition: principal.MaskAlways}},
		})
	}
	grants := map[string]principal.ObjectGrant{
		"deal": {Read: true}, "forecast": {Read: true, Create: true}, "installation_settings": {Read: true},
	}
	observed := e.observedAs(manager(grants), t)
	if ids, _ := observed["deal_ids"].([]any); len(ids) != 1 || observed["open_change_minor"] != nil {
		t.Fatalf("a masked manager read %v, want the deal and no money", observed)
	}
	delete(grants, "deal")
	observed = e.observedAs(manager(grants), t)
	if ids, _ := observed["deal_ids"].([]any); len(ids) != 0 || observed["withheld"] != true || len(observed) != 3 {
		t.Fatalf("a seat that may not read deals read %v, want only the comparison, no deal and withheld", observed)
	}
}

func (e *callEnv) observedAs(reader context.Context, t *testing.T) map[string]any {
	t.Helper()
	found := e.callFindingsAs(reader, t)
	if len(found) != 1 {
		t.Fatalf("the reader sees %d call findings, want 1", len(found))
	}
	var observed map[string]any
	if err := json.Unmarshal(found[0].Observed, &observed); err != nil {
		t.Fatal(err)
	}
	return observed
}

// The forecast tool tells a model what the review panel tells a manager: the
// call kept its baseline, and the pipeline moved away from it.
func TestTheForecastToolCarriesTheCallsReview(t *testing.T) {
	e := setupCall(t)
	e.recordCall(t, 5_000_000)
	e.updateDeal(t, deals.UpdateDealInput{AmountMinor: new(int64(9_900_000)), Currency: new("EUR")})
	e.check(t)

	raw, err := forecastToolReader(e.Pool)(e.Admin(), agents.ForecastRequest{ScopeKind: forecasting.ScopeWorkspace})
	if err != nil {
		t.Fatalf("the forecast tool: %v", err)
	}
	var answer struct {
		CurrentCall struct {
			Baseline string `json:"baseline"`
			Review   struct {
				Type string `json:"type"`
			} `json:"review"`
		} `json:"current_call"`
	}
	if err := json.Unmarshal(raw, &answer); err != nil {
		t.Fatal(err)
	}
	if answer.CurrentCall.Baseline != "captured" || answer.CurrentCall.Review.Type != assurance.TypeForecastCallMoved {
		t.Fatalf("the tool's call = %+v, want its baseline and its review", answer.CurrentCall)
	}
}
