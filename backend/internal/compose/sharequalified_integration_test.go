// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A snapshot shared while the qualified forecast was enforced states the
// qualified figures and landing the forecast stated when it was frozen.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/platform/clocktest"
)

func TestASnapshotShareFrozenUnderEnforcementStatesTheQualifiedForecast(t *testing.T) {
	e := setupForecast(t)
	e.seedDealClosingWithin(t, "Unread", &e.Rep1, 1_000_000)
	// The switch itself, written as its row: the admin door also asks for an
	// input check first, which this environment does not run.
	if _, err := e.owner.Exec(context.Background(), `INSERT INTO setting (key, value) VALUES ($1, 'true')
		ON CONFLICT (key) DO UPDATE SET value = 'true'`, assurance.QualifiedForecastEnforced.Key()); err != nil {
		t.Fatal(err)
	}
	var live forecasting.Readings
	var landing forecasting.Landing
	store := forecasting.NewStore(InstallationDB(e.Pool))
	if err := store.InTx(snapshotWriterCtx(e.WS), func(ctx context.Context, tx pgx.Tx) error {
		period, base, err := ForecastPeriodAt(ctx, tx, forecasting.PeriodQuarter, clocktest.Now(t))
		if err != nil {
			return err
		}
		assembly, err := ForecastAssemble(ctx, tx, period, forecasting.Scope{Kind: forecasting.ScopeWorkspace}, clocktest.Now(t), base)
		if err != nil {
			return err
		}
		live = assembly.Readings
		measure, err := ForecastForwardMeasure(ctx, tx)
		if err != nil {
			return err
		}
		landing, err = forecasting.ProjectLanding(live, measure, nil)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if live.Qualified == nil || live.Qualified.UnknownCount != 1 {
		t.Fatalf("the live forecast reads %+v, want the unread deal unknown", live.Qualified)
	}

	company, _ := e.shareReaders()
	token := e.issueWorkspaceShare(t, shareKindSnapshot, new(e.freezeMeasured(t)))
	view := e.openShare(company, t, token)
	if view.Readings.QualificationMode != "enforcing" || view.Readings.Qualified == nil {
		t.Fatalf("the share states mode %q and block %+v, want enforcing with a block",
			view.Readings.QualificationMode, view.Readings.Qualified)
	}
	q := view.Readings.Qualified
	if q.UnknownCount != live.Qualified.UnknownCount || q.UnknownMinor != live.Qualified.UnknownMinor ||
		q.EvidenceMinor != live.Qualified.EvidenceMinor || q.WeightedMinor != live.Qualified.WeightedMinor {
		t.Errorf("the share states %+v, want the forecast's %+v", q, live.Qualified)
	}
	if got := view.Readings.Landing; got == nil || string(got.Measure) != string(landing.Measure) ||
		got.AmountMinor != landing.AmountMinor {
		t.Errorf("the share lands %+v, want the forecast's %s %d", got, landing.Measure, landing.AmountMinor)
	}
}
