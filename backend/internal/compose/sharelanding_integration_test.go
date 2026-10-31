// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A share states the landing GET /forecast states for its reader: a live share
// computes it now, a snapshot share from the measure and call it froze. The
// call shows only to a reader who may see it, and a snapshot frozen before the
// landing was kept has none.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const sharedCallMinor = 3_000_000

// calledQuarter is one priced deal of 1,000,000 and a company call of
// 3,000,000 under the manager-call measure.
func calledQuarter(t *testing.T) *forecastEnv {
	t.Helper()
	e := setupForecast(t)
	e.seedDealClosingWithin(t, "Mine", &e.Rep1, 1_000_000)
	admin := reportingActor(e)
	if err := config.Save(admin, StoredConfig(e.Pool), identity.ForecastForwardMeasure, string(forecasting.MeasureManagerCall)); err != nil {
		t.Fatal(err)
	}
	calls := forecasting.NewStore(InstallationDB(e.Pool))
	if err := calls.InTx(admin, func(ctx context.Context, tx pgx.Tx) error {
		period, base, err := ForecastPeriodAt(ctx, tx, forecasting.PeriodQuarter, clocktest.Now(t))
		if err != nil {
			return err
		}
		_, err = calls.RecordCallTx(ctx, tx, forecasting.NewCall{Period: period, Scope: forecasting.Scope{Kind: forecasting.ScopeWorkspace}, AmountMinor: sharedCallMinor, Currency: base, Note: "Quarter review"})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return e
}

// The company reader may see the call; the team-lens reader reads the same
// deal but may not assert about the company.
func (e *forecastEnv) shareReaders() (company, teamLens context.Context) {
	company = e.forecastReader(e.dealReadCtx(e.Rep1, nil, principal.RowScopeAll))
	teamLens = e.forecastReader(e.dealReadCtx(e.Rep1, []ids.UUID{e.Team1}, principal.RowScopeTeam))
	return company, teamLens
}

func (e *forecastEnv) openShare(reader context.Context, t *testing.T, token string) crmcontracts.SharedForecastView {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/forecast/shared/"+token, nil).WithContext(reader)
	rec := httptest.NewRecorder()
	e.shareRoutes().OpenForecastShare(rec, req, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("opening the share = %d %s", rec.Code, rec.Body.String())
	}
	var view crmcontracts.SharedForecastView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatalf("decoding the shared view: %v", err)
	}
	return view
}

func (e *forecastEnv) issueWorkspaceShare(t *testing.T, kind string, snapshot *ids.UUID) string {
	t.Helper()
	_, token := e.issue(e.writerCtx(t), t, NewShare{
		Kind: kind, Target: "someone@example.test",
		Scope: forecasting.Scope{Kind: forecasting.ScopeWorkspace}, SnapshotID: snapshot,
		ExpiresAt: clocktest.Now(t).UTC().Add(24 * time.Hour),
	})
	return token
}

// freezeMeasured freezes the seeded deals as the daily capture does, with the
// forward measure in force.
func (e *forecastEnv) freezeMeasured(t *testing.T) ids.UUID {
	t.Helper()
	store := forecasting.NewStore(InstallationDB(e.Pool))
	var id ids.UUID
	if err := store.InTx(snapshotWriterCtx(e.WS), func(ctx context.Context, tx pgx.Tx) error {
		period, base, err := ForecastPeriodAt(ctx, tx, forecasting.PeriodQuarter, clocktest.Now(t))
		if err != nil {
			return err
		}
		workspace := forecasting.Scope{Kind: forecasting.ScopeWorkspace}
		assembly, err := ForecastAssemble(ctx, tx, period, workspace, clocktest.Now(t), base)
		if err != nil {
			return err
		}
		snapshot := forecasting.NewSnapshot{Period: period, Scope: workspace, Trigger: forecasting.TriggerDaily, BaseCurrency: base, Readings: assembly.Readings, TakenAt: clocktest.Now(t)}
		snapshot.StampChances(assembly)
		if err := stampForecastBasis(ctx, tx, &snapshot); err != nil {
			return err
		}
		id, err = store.TakeSnapshot(ctx, tx, snapshot)
		return err
	}); err != nil {
		t.Fatalf("freezing the seeded deals: %v", err)
	}
	return id
}

func assertCallLanding(t *testing.T, who string, landing *crmcontracts.ForecastLanding) {
	t.Helper()
	if landing == nil || landing.Measure != "manager_call" || landing.AmountMinor != sharedCallMinor {
		t.Fatalf("%s: landing %+v, want the 3,000,000 call", who, landing)
	}
}

func assertFallbackLanding(t *testing.T, who string, landing *crmcontracts.ForecastLanding) {
	t.Helper()
	if landing == nil || landing.Measure != "commit_evidence" || landing.Caveat == nil || *landing.Caveat != "call_absent" || landing.AmountMinor == sharedCallMinor {
		t.Fatalf("%s: landing %+v, want the records' projection with call_absent and no call", who, landing)
	}
}

func TestALiveShareStatesTheLandingTheForecastStatesForItsReader(t *testing.T) {
	e := calledQuarter(t)
	company, teamLens := e.shareReaders()
	token := e.issueWorkspaceShare(t, shareKindLive, nil)
	fixture := &stageCohortFixture{env: e}
	for _, reader := range []struct {
		name string
		ctx  context.Context
		call bool
	}{{"the company reader", company, true}, {"the team-lens reader", teamLens, false}} {
		view := e.openShare(reader.ctx, t, token)
		door := fixture.forecastOverHTTPAs(reader.ctx, t, clocktest.Now(t))
		if !reflect.DeepEqual(view.Readings.Landing, door.Landing) {
			t.Fatalf("%s: the share's landing %+v differs from GET /forecast's %+v", reader.name, view.Readings.Landing, door.Landing)
		}
		if reader.call {
			assertCallLanding(t, reader.name, view.Readings.Landing)
		} else {
			assertFallbackLanding(t, reader.name, view.Readings.Landing)
		}
	}
}

func TestASnapshotShareStatesTheLandingItFroze(t *testing.T) {
	e := calledQuarter(t)
	company, teamLens := e.shareReaders()
	token := e.issueWorkspaceShare(t, shareKindSnapshot, new(e.freezeMeasured(t)))
	assertCallLanding(t, "the company reader", e.openShare(company, t, token).Readings.Landing)
	assertFallbackLanding(t, "the team-lens reader", e.openShare(teamLens, t, token).Readings.Landing)
}

func TestASnapshotFrozenBeforeTheLandingWasKeptHasNone(t *testing.T) {
	e := calledQuarter(t)
	company, _ := e.shareReaders()
	token := e.issueWorkspaceShare(t, shareKindSnapshot, new(e.freezeWorkspace(t)))
	view := e.openShare(company, t, token)
	if view.Readings.Landing != nil {
		t.Fatalf("a snapshot frozen with no measure states the landing %+v, want none", view.Readings.Landing)
	}
	if view.Readings.RangeLowMinor == nil {
		t.Fatal("the legacy snapshot lost its range, which the share still states")
	}
}
