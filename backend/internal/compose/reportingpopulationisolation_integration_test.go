// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
)

func TestReportingPopulationErrorsDoNotPoisonLaterMetricSavepoints(t *testing.T) {
	e := setupForecast(t)
	counts := traceReportingPopulation(t, e)
	ctx := auth.WithIndividualPerformance(reportingActor(e), true)
	err := database.WithWorkspaceSnapshot(ctx, e.Pool, func(snapshot context.Context) error {
		return database.WithWorkspaceTx(snapshot, e.Pool, func(tx pgx.Tx) error {
			cached, release := withReportingPopulationReads(snapshot, tx)
			defer release()
			counts.reset()
			failed := readerThat(func(ctx context.Context, child pgx.Tx) error {
				_, err := readReportingPopulationFact(ctx, child, reportingPopulationKey{kind: ScopeKindOwner, subject: e.Rep1}, func() (reportingPopulationFact, error) {
					var label string
					err := child.QueryRow(ctx, `SELECT (1 / 0)::text`).Scan(&label)
					return reportingPopulationFact{label: label}, err
				})
				return err
			})
			_, coverage, err := (metricEvaluator{}).readIsolated(cached, tx, failed, crmcontracts.ReportingContext{}, crmcontracts.ReportingFramework{})
			if err != nil {
				return err
			}
			if coverage.Status != reportingFailed {
				t.Fatalf("failed population query coverage=%s", coverage.Status)
			}
			healthy := readerThat(func(ctx context.Context, child pgx.Tx) error {
				label, err := analyticsUserLabel(ctx, child, e.Rep1)
				if err == nil && label != "Rep" {
					t.Fatalf("a failed read supplied a label: %q", label)
				}
				return err
			})
			for range 2 {
				_, coverage, err := (metricEvaluator{}).readIsolated(cached, tx, healthy, crmcontracts.ReportingContext{}, crmcontracts.ReportingFramework{})
				if err != nil {
					return err
				}
				if coverage.Status != "unsupported" {
					t.Fatalf("healthy metric coverage=%s", coverage.Status)
				}
			}
			if counts.count("owner") != 1 || counts.count("isolation") != 1 {
				t.Fatalf("savepoints read owner=%d isolation=%d, want one each", counts.count("owner"), counts.count("isolation"))
			}
			return nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestReportingPopulationSuccessSurvivesAMetricRollback(t *testing.T) {
	e := setupForecast(t)
	counts := traceReportingPopulation(t, e)
	ctx := auth.WithIndividualPerformance(reportingActor(e), true)
	err := InstallationDB(e.Pool).TxIsolated(ctx, pgx.RepeatableRead, func(tx pgx.Tx) error {
		cached, release := withReportingPopulationReads(ctx, tx)
		defer release()
		counts.reset()
		failed := readerThat(func(ctx context.Context, child pgx.Tx) error {
			if _, err := analyticsTeamLabel(ctx, child, e.Team1); err != nil {
				return err
			}
			_, err := child.Exec(ctx, `SELECT 1 / 0`)
			return err
		})
		_, coverage, err := (metricEvaluator{}).readIsolated(cached, tx, failed, crmcontracts.ReportingContext{}, crmcontracts.ReportingFramework{})
		if err != nil {
			return err
		}
		if coverage.Status != reportingFailed {
			t.Fatal("database error did not isolate the metric")
		}
		if _, err := analyticsTeamLabel(cached, tx, e.Team1); err != nil {
			return err
		}
		if counts.count("team") != 1 {
			t.Fatalf("rollback lost successful snapshot fact: %d reads", counts.count("team"))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestReportingPopulationWarmFactsDoNotHideCancellation(t *testing.T) {
	e := setupForecast(t)
	ctx := auth.WithIndividualPerformance(reportingActor(e), true)
	err := InstallationDB(e.Pool).TxIsolated(ctx, pgx.RepeatableRead, func(tx pgx.Tx) error {
		cached, release := withReportingPopulationReads(ctx, tx)
		defer release()
		if _, err := analyticsUserLabel(cached, tx, e.Rep1); err != nil {
			return err
		}
		cancelled, cancel := context.WithCancel(cached)
		cancel()
		_, err := analyticsUserLabel(cancelled, tx, e.Rep1)
		return err
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("warm subject hid cancellation: %v", err)
	}
}
