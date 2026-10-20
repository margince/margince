// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
)

// readIsolated reads one metric under a savepoint, so a database failure
// marks only that metric failed and the metrics after it still read.
func (e metricEvaluator) readIsolated(
	ctx context.Context, tx pgx.Tx, spec metricSpec,
	frame crmcontracts.ReportingContext, framework crmcontracts.ReportingFramework,
) ([]reporting.Fact, crmcontracts.ReportingCoverage, error) {
	var facts []reporting.Fact
	var coverage crmcontracts.ReportingCoverage
	failed, err := underSavepoint(ctx, tx, string(spec.definition.Id), func(savepoint pgx.Tx) error {
		var err error
		facts, coverage, err = spec.read(ctx, savepoint, frame, framework, e)
		if err != nil {
			return err
		}
		coverage, err = reportingHistoryCoverage(ctx, savepoint, frame, spec.definition.Id, coverage)
		return err
	})
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	if failed {
		return nil, calculationFailed(frame), nil
	}
	return facts, coverage, nil
}

func calculationFailed(frame crmcontracts.ReportingContext) crmcontracts.ReportingCoverage {
	return reportingGap(reportingFailed, reporting.ExplanationCopy(frame).Say(reportingCoverageWords.CalculationFailed))
}

// underSavepoint runs read under a savepoint. A database error is rolled
// back to the savepoint, logged, and reported as failed, which leaves the
// transaction usable. Any other error (a refused grant, a cancelled request)
// is returned and still fails the whole evaluation.
func underSavepoint(ctx context.Context, tx pgx.Tx, figure string, read func(pgx.Tx) error) (bool, error) {
	savepoint, err := tx.Begin(ctx)
	if err != nil {
		return false, err
	}
	err = read(savepoint)
	if err == nil {
		return false, savepoint.Commit(ctx)
	}
	var database *pgconn.PgError
	if !errors.As(err, &database) || ctx.Err() != nil {
		return false, errors.Join(err, savepoint.Rollback(ctx))
	}
	if rollback := savepoint.Rollback(ctx); rollback != nil {
		return false, errors.Join(err, rollback)
	}
	slog.ErrorContext(ctx, "reporting: a figure could not be calculated; the other figures are served",
		"figure", figure, "sqlstate", database.Code, "err", err)
	return true, nil
}
