// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestReportingPipelineCaptureCannotBecomeAWholeTeamOpeningOrShare(t *testing.T) {
	f := reportingBusiness(t)
	f.at = time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	scope := crmcontracts.ReportingScope{Kind: "team", Id: ptrUUID(f.env.Team1)}
	worker := &forecastSnapshotSweepWorker{pool: f.env.Pool, now: func() time.Time { return f.at }, log: slog.New(slog.DiscardHandler)}
	if err := worker.freezeContext(f.writer, crmcontracts.ReportingCaptureContext{Scope: scope}); err != nil {
		t.Fatal(err)
	}
	f.at = f.at.Add(time.Hour)
	if err := worker.freezeContext(f.writer, crmcontracts.ReportingCaptureContext{Scope: scope, PipelineId: ptrUUID(f.env.pipeline)}); err != nil {
		t.Fatal(err)
	}
	err := InstallationDB(f.env.Pool).Tx(f.human, func(tx pgx.Tx) error {
		period, _, err := ForecastPeriodAt(f.human, tx, forecasting.PeriodQuarter, f.at)
		if err != nil {
			return err
		}
		requested := forecasting.Scope{Kind: "team", ID: &f.env.Team1}
		opening, found, err := (&WeeklyForecast{}).openingSnapshot(f.human, tx, period, requested, period.LocalDay(f.at))
		if err != nil {
			return err
		}
		if !found {
			t.Fatal("whole-team opening absent")
		}
		var whole, pipeline ids.UUID
		if err := tx.QueryRow(f.human, "SELECT id FROM forecast_snapshot WHERE scope_id=$1 AND pipeline_id IS NULL", f.env.Team1).Scan(&whole); err != nil {
			return err
		}
		if err := tx.QueryRow(f.human, "SELECT id FROM forecast_snapshot WHERE scope_id=$1 AND pipeline_id=$2", f.env.Team1, f.env.pipeline).Scan(&pipeline); err != nil {
			return err
		}
		if opening != whole {
			t.Fatal("pipeline capture replaced whole-team opening")
		}
		err = checkSnapshotScope(f.human, tx, NewShare{Kind: shareKindSnapshot, SnapshotID: &pipeline, Scope: forecasting.Scope{Kind: "team", ID: &f.env.Team1}})
		if !errors.Is(err, apperrors.ErrNotFound) {
			t.Fatalf("pipeline share lost its context: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDailyCaptureRecordsPopulationAndRemainsIdempotent(t *testing.T) {
	e := setupSnapshotJob(t)
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	_, id := e.dailySnapshots(t)
	var fingerprint string
	if err := e.Pool.QueryRow(context.Background(), "SELECT population_fingerprint FROM forecast_snapshot WHERE id=$1", id).Scan(&fingerprint); err != nil {
		t.Fatal(err)
	}
	if fingerprint == "" {
		t.Fatal("default capture omitted the reporting population")
	}
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	if count, _ := e.dailySnapshots(t); count != 1 {
		t.Fatalf("duplicate daily captures: %d", count)
	}
}

func TestFrameworkFailureDoesNotLoseWorkspaceCapture(t *testing.T) {
	f := reportingBusiness(t)
	if _, err := f.env.owner.Exec(context.Background(), `UPDATE reporting_framework_revision SET definition='{"template":7}'::jsonb`); err != nil {
		t.Fatal(err)
	}
	worker := &forecastSnapshotSweepWorker{pool: f.env.Pool, now: func() time.Time { return f.at }, log: slog.New(slog.DiscardHandler)}
	if err := worker.freeze(f.writer, f.env.WS); err == nil {
		t.Fatal("framework corruption must be reported")
	}
	var captures int
	if err := f.env.owner.QueryRow(context.Background(), "SELECT count(*) FROM forecast_snapshot WHERE scope_kind='workspace' AND pipeline_id IS NULL").Scan(&captures); err != nil {
		t.Fatal(err)
	}
	if captures != 1 {
		t.Fatalf("workspace capture lost with framework failure: %d", captures)
	}
}
