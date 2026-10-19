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

// readIsolated reads one metric under a savepoint. A database failure marks
// only that metric failed: rolling back to the savepoint leaves the
// transaction usable, so the metrics after it still read. Any other error
// (a refused grant, a cancelled request) still fails the whole evaluation.
func (e metricEvaluator) readIsolated(
	ctx context.Context, tx pgx.Tx, spec metricSpec,
	frame crmcontracts.ReportingContext, framework crmcontracts.ReportingFramework,
) ([]reporting.Fact, crmcontracts.ReportingCoverage, error) {
	savepoint, err := tx.Begin(ctx)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	facts, coverage, err := spec.read(ctx, savepoint, frame, framework, e)
	if err == nil {
		coverage, err = reportingHistoryCoverage(ctx, savepoint, frame, spec.definition.Id, coverage)
	}
	if err == nil {
		return facts, coverage, savepoint.Commit(ctx)
	}
	var database *pgconn.PgError
	if !errors.As(err, &database) || ctx.Err() != nil {
		return nil, crmcontracts.ReportingCoverage{}, errors.Join(err, savepoint.Rollback(ctx))
	}
	if rollback := savepoint.Rollback(ctx); rollback != nil {
		return nil, crmcontracts.ReportingCoverage{}, errors.Join(err, rollback)
	}
	slog.ErrorContext(ctx, "reporting: a metric could not be calculated; the other figures are served",
		"metric", spec.definition.Id, "sqlstate", database.Code, "err", err)
	return nil, reportingGap(reportingFailed, reporting.ExplanationCopy(frame).Say(reportingCoverageWords.CalculationFailed)), nil
}
