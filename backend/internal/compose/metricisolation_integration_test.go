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
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func readerThat(run func(context.Context, pgx.Tx) error) metricSpec {
	return metricSpec{
		definition: reportingMetrics[0].definition,
		read: func(ctx context.Context, tx pgx.Tx, _ crmcontracts.ReportingContext, _ crmcontracts.ReportingFramework, _ metricEvaluator) ([]reporting.Fact, crmcontracts.ReportingCoverage, error) {
			if err := run(ctx, tx); err != nil {
				return nil, crmcontracts.ReportingCoverage{}, err
			}
			return nil, reportingGap("unsupported", "read"), nil
		},
	}
}

// A metric whose query fails is marked failed and the transaction stays
// usable, so the metric after it on the same evaluation still reads.
func TestAFailedMetricQueryLeavesTheOtherMetricsReadable(t *testing.T) {
	e := setupForecast(t)
	ctx := e.Admin()
	failing := readerThat(func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "SELECT 1/0")
		return err
	})
	healthy := readerThat(func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "SELECT 1")
		return err
	})

	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, coverage, err := metricEvaluator{}.readIsolated(ctx, tx, failing, crmcontracts.ReportingContext{}, crmcontracts.ReportingFramework{})
		if err != nil {
			t.Fatalf("a failing query failed the evaluation: %v", err)
		}
		if coverage.Status != reportingFailed || coverage.Reason == nil || *coverage.Reason == "" {
			t.Errorf("coverage = %+v, want failed with a reason", coverage)
		}
		_, coverage, err = metricEvaluator{}.readIsolated(ctx, tx, healthy, crmcontracts.ReportingContext{}, crmcontracts.ReportingFramework{})
		if err != nil {
			t.Fatalf("the metric after a failed one could not read: %v", err)
		}
		if coverage.Status != "unsupported" {
			t.Errorf("coverage after the failure = %+v, want the reader's own answer", coverage)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// Only a database failure is isolated. A refusal is still the caller's
// answer for the whole evaluation.
func TestARefusedMetricStillFailsTheEvaluation(t *testing.T) {
	e := setupForecast(t)
	ctx := e.Admin()
	refused := readerThat(func(context.Context, pgx.Tx) error { return apperrors.ErrPermissionDenied })

	err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, _, err := metricEvaluator{}.readIsolated(ctx, tx, refused, crmcontracts.ReportingContext{}, crmcontracts.ReportingFramework{})
		return err
	})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("err = %v, want the refusal to fail the evaluation", err)
	}
}
