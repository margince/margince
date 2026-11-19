// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A share states the call and landing GET /forecast states for its reader. A
// live share computes both now. A snapshot share lands on the call it froze and
// withholds the record it never kept. One frozen before the landing has none.

import (
	"context"
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

// calledQuarter is one priced deal of 1,000,000 under the manager-call
// measure, with a company call of 3,000,000 when called.
func calledQuarter(t *testing.T, called bool) *forecastEnv {
	t.Helper()
	e := setupForecast(t)
	e.seedDealClosingWithin(t, "Mine", &e.Rep1, 1_000_000)
	admin := reportingActor(e)
	if err := config.Save(admin, StoredConfig(e.Pool), identity.ForecastForwardMeasure, string(forecasting.MeasureManagerCall)); err != nil {
		t.Fatal(err)
	}
	if !called {
		return e
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
	var view crmcontracts.SharedForecastView
	decodeWire(t, rec, http.StatusOK, &view)
	return view
}

func (e *forecastEnv) issueWorkspaceShare(t *testing.T, kind string, snapshot *ids.UUID) string {
	t.Helper()
	_, token := e.issue(e.writerCtx(t), t, NewShare{Kind: kind, Target: "someone@example.test", SnapshotID: snapshot,
		Scope: forecasting.Scope{Kind: forecasting.ScopeWorkspace}, ExpiresAt: clocktest.Now(t).UTC().Add(24 * time.Hour)})
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

// assertShareCall holds the call and landing one reader is given. A reader who
// may see the call gets its record and lands on it. Any other reader gets
// call_withheld and the records' projection with call_absent.
func assertShareCall(t *testing.T, who string, readings crmcontracts.ForecastReadings, seen bool) {
	t.Helper()
	landing, call := readings.Landing, readings.CurrentCall
	if seen {
		if landing == nil || landing.Measure != "manager_call" || landing.AmountMinor != sharedCallMinor ||
			call == nil || call.AmountMinor != sharedCallMinor || readings.CallWithheld != nil {
			t.Fatalf("%s: landing %+v, call %+v, want both on the 3,000,000 call", who, landing, call)
		}
		return
	}
	if landing == nil || landing.Measure != "likely" || landing.Caveat == nil || *landing.Caveat != "call_absent" ||
		call != nil || readings.CallWithheld == nil || !*readings.CallWithheld {
		t.Fatalf("%s: landing %+v, call %+v, want the records' projection and the call withheld", who, landing, call)
	}
}

func TestALiveShareStatesTheCallAndLandingTheForecastStatesForItsReader(t *testing.T) {
	e := calledQuarter(t, true)
	company, teamLens := e.shareReaders()
	token := e.issueWorkspaceShare(t, shareKindLive, nil)
	for _, reader := range []struct {
		name string
		ctx  context.Context
		seen bool
	}{{"the company reader", company, true}, {"the team-lens reader", teamLens, false}} {
		view, door := e.openShare(reader.ctx, t, token).Readings, (&stageCohortFixture{env: e}).forecastOverHTTPAs(reader.ctx, t, clocktest.Now(t))
		if tool := (&stageCohortFixture{env: e}).forecastOverToolAs(reader.ctx, t, clocktest.Now(t)); tool.CallWithheld == reader.seen || (len(tool.CurrentCall) > 0) != reader.seen {
			t.Fatalf("%s: forecast_readings states call %s, withheld %v", reader.name, tool.CurrentCall, tool.CallWithheld)
		}
		assertShareCall(t, reader.name, door, reader.seen)
		assertShareCall(t, reader.name+"'s share", view, reader.seen)
		if !reflect.DeepEqual(view.Landing, door.Landing) || !reflect.DeepEqual(view.CurrentCall, door.CurrentCall) {
			t.Fatalf("%s: the share states %+v and %+v, GET /forecast %+v and %+v", reader.name, view.Landing, view.CurrentCall, door.Landing, door.CurrentCall)
		}
	}
}

// A snapshot keeps the call's amount, not its record: it lands on the call and withholds the record.
func TestASnapshotShareStatesTheLandingItFroze(t *testing.T) {
	e := calledQuarter(t, true)
	company, teamLens := e.shareReaders()
	token := e.issueWorkspaceShare(t, shareKindSnapshot, new(e.freezeMeasured(t)))
	frozen := e.openShare(company, t, token).Readings
	if l := frozen.Landing; l == nil || l.Measure != "manager_call" || l.AmountMinor != sharedCallMinor || frozen.CurrentCall != nil || frozen.CallWithheld == nil {
		t.Fatalf("the company reader's snapshot: landing %+v, call %+v; want it on the frozen call, its record withheld", frozen.Landing, frozen.CurrentCall)
	}
	assertShareCall(t, "the team-lens reader", e.openShare(teamLens, t, token).Readings, false)
}

// A snapshot that froze no call says so only to a reader who may see calls.
func TestASnapshotWithNoCallSaysSoOnlyToAReaderWhoMaySeeCalls(t *testing.T) {
	e := calledQuarter(t, false)
	company, teamLens := e.shareReaders()
	token := e.issueWorkspaceShare(t, shareKindSnapshot, new(e.freezeMeasured(t)))
	if got := e.openShare(company, t, token).Readings; got.CurrentCall != nil || got.CallWithheld != nil {
		t.Fatalf("the company reader's snapshot states call %+v, withheld %v; want neither", got.CurrentCall, got.CallWithheld)
	}
	assertShareCall(t, "the team-lens reader", e.openShare(teamLens, t, token).Readings, false)
}

func TestASnapshotFrozenBeforeTheLandingWasKeptHasNone(t *testing.T) {
	e := calledQuarter(t, true)
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
