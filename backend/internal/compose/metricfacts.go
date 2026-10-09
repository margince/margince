// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	reportingFactLimit    = 100000
	reportingExactInteger = int64(1<<53 - 1)
)

type metricFactQuery struct {
	pipeline                                                                   string
	stagePosition                                                              string
	revision                                                                   int64
	from, where, owner, at, value, key, stage, stageLabel, outcome, provenance string
	money                                                                      bool
}

func readDealMetricFacts(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, metric crmcontracts.ReportingMetricID, spec metricFactQuery) ([]reporting.Fact, crmcontracts.ReportingCoverage, error) {
	var b reportingBindings
	_, population, err := analyticsPopulationExpression(ctx, tx, reportingRequested(frame.Scope), spec.owner, b.arg, unownedIsExcluded)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	if population == "" {
		population = sqlUnnarrowed
	}
	rowScope, err := auth.ScopeClauseFor(ctx, string(recordTypeDeal), "t", b.arg)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	if rowScope == "" {
		rowScope = sqlUnnarrowed
	}
	value, err := reportingFactValue(ctx, frame, spec, &b)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	from := spec.from
	if strings.Contains(from, "@revision") {
		from = strings.ReplaceAll(from, "@revision", b.add(spec.revision))
	}
	fieldScope, withheld, err := reportingFieldScope(ctx, string(recordTypeDeal), "t", reportingDealFields(metric), &b)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	where := spec.where + " AND " + population + " AND " + rowScope + " AND " + fieldScope
	where += reportingFactFilters(frame, metric, spec, &b)
	position := spec.stagePosition
	if position == "" {
		position = "0"
	}
	query := "SELECT " + spec.key + ",t.id,t.name," + spec.owner + ",COALESCE(u.display_name,'Unassigned')," + spec.at + "," + value + "," + spec.stage + "," + spec.stageLabel + "," + spec.outcome + "," + spec.provenance + "," + position + " FROM deal t " + from + " LEFT JOIN app_user u ON u.id=" + spec.owner + " WHERE " + where + " ORDER BY " + spec.key + " LIMIT " + b.add(reportingFactLimit+1)
	rows, err := tx.Query(ctx, query, b.values...)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	facts, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (reporting.Fact, error) {
		return scanDealMetricFact(row, metric, spec.money)
	})
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	if len(facts) > reportingFactLimit {
		return nil, crmcontracts.ReportingCoverage{}, fmt.Errorf("narrow the reporting population before evaluating: %w", apperrors.ErrInvalidArgument)
	}
	coverage := crmcontracts.ReportingCoverage{Status: "ok", Withheld: withheld}
	return reportingCohorts(frame, facts), coverage, nil
}

func scanDealMetricFact(row pgx.CollectableRow, metric crmcontracts.ReportingMetricID, money bool) (reporting.Fact, error) {
	fact := reporting.Fact{Metric: metric, SourceType: string(recordTypeDeal)}
	var owner *ids.UUID
	var value *float64
	var amount *int64
	var err error
	if money {
		err = row.Scan(&fact.Row.Key, &fact.SourceID, &fact.Row.Label, &owner, &fact.OwnerLabel, &fact.Row.OccurredAt, &amount, &fact.StageID, &fact.StageLabel, &fact.Outcome, &fact.Provenance, &fact.StagePosition)
	} else {
		err = row.Scan(&fact.Row.Key, &fact.SourceID, &fact.Row.Label, &owner, &fact.OwnerLabel, &fact.Row.OccurredAt, &value, &fact.StageID, &fact.StageLabel, &fact.Outcome, &fact.Provenance, &fact.StagePosition)
	}
	if err != nil {
		return fact, err
	}
	if amount != nil {
		if *amount > reportingExactInteger || *amount < -reportingExactInteger {
			return fact, fmt.Errorf("reporting amount is outside the exact display range: %w", apperrors.ErrInvalidArgument)
		}
		number := float64(*amount)
		value = &number
		fact.Money = amount
	}
	fact.Row.Value = value
	fact.Row.SourceId = ptrUUID(fact.SourceID)
	fact.Row.SourceType = &fact.SourceType
	if owner != nil {
		fact.Row.OwnerId = ptrUUID(*owner)
	}
	if owner != nil {
		fact.OwnerID = *owner
	}
	return fact, nil
}

func readReportingBookings(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, _ crmcontracts.ReportingFramework, _ metricEvaluator) ([]reporting.Fact, crmcontracts.ReportingCoverage, error) {
	return readDealMetricFacts(ctx, tx, frame, reportingBookingsWon, closingMetricQuery(true))
}

func readReportingOutcomes(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, _ crmcontracts.ReportingFramework, _ metricEvaluator) ([]reporting.Fact, crmcontracts.ReportingCoverage, error) {
	return readDealMetricFacts(ctx, tx, frame, reportingClosedWinRate, closingMetricQuery(false))
}

func closingMetricQuery(money bool) metricFactQuery {
	query := metricFactQuery{from: deals.CurrentClosingJoin("t", "closing"), where: "t.archived_at IS NULL AND t.status <> 'open'", owner: colOwnerID, at: "t.closed_at", key: "COALESCE(closing.id,t.id)::text", stage: "''", stageLabel: "''", outcome: "t.status", provenance: "'current_closing'", money: money}
	if money {
		query.where += " AND t.status='won'"
	} else {
		query.value = "1::float8"
	}
	return query
}

func reportingClosePredicate(frame crmcontracts.ReportingContext, b *reportingBindings) string {
	if frame.CloseInterval == nil {
		return ""
	}
	return " AND t.expected_close_date >= (timezone(" + b.add(frame.Timezone) + "," + b.add(frame.CloseInterval.StartAt) + "::timestamptz))::date AND t.expected_close_date < (timezone(" + b.add(frame.Timezone) + "," + b.add(frame.CloseInterval.EndAt) + "::timestamptz))::date"
}

func readReportingOpen(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, _ crmcontracts.ReportingFramework, _ metricEvaluator) ([]reporting.Fact, crmcontracts.ReportingCoverage, error) {
	return readDealMetricFacts(ctx, tx, frame, reportingOpenPipeline, openMetricQuery(false))
}

func readReportingAge(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, _ crmcontracts.ReportingFramework, _ metricEvaluator) ([]reporting.Fact, crmcontracts.ReportingCoverage, error) {
	return readDealMetricFacts(ctx, tx, frame, reportingStageAge, openMetricQuery(true))
}

func openMetricQuery(age bool) metricFactQuery {
	query := metricFactQuery{stagePosition: "COALESCE(s.position,0)", from: "LEFT JOIN stage s ON s.id=t.stage_id", where: "t.archived_at IS NULL AND t.status='open'", owner: colOwnerID, at: "NULL::timestamptz", key: "t.id::text", stage: "t.stage_id::text", stageLabel: "COALESCE(s.name,'Unmapped')", outcome: "'open'", provenance: "'current_state'", money: !age}
	if age {
		query.from += " " + currentStageEntryJoin
		query.value = "current_stage_age"
	}
	return query
}

func reportingFactValue(ctx context.Context, frame crmcontracts.ReportingContext, spec metricFactQuery, b *reportingBindings) (string, error) {
	value := spec.value
	if value == "current_stage_age" {
		value = currentStageAgeSQL(b.add(frame.EvaluatedAt) + "::timestamptz")
	}
	if spec.money {
		if value == "" {
			value = deals.BaseValueSQL(b.add(frame.EvaluatedAt), b.add(frame.Currency), "t")
		}
		var err error
		value, err = auth.MaskedExpressionSQL(ctx, string(recordTypeDeal), "amount_minor", "t", value, b.arg)
		if err != nil {
			return "", err
		}
	}
	return value, nil
}

func reportingFactFilters(frame crmcontracts.ReportingContext, metric crmcontracts.ReportingMetricID, spec metricFactQuery, b *reportingBindings) string {
	where := ""
	if frame.PipelineId != nil {
		pipeline := spec.pipeline
		if pipeline == "" {
			pipeline = "t.pipeline_id"
		}
		where += " AND " + pipeline + "=" + b.add(*frame.PipelineId)
	}
	// The filter starts early enough for every chart subcontext; projection
	// assigns interval, target, and previous cohorts from the same fact read.
	if spec.at != "NULL::timestamptz" {
		where += " AND " + spec.at + " >= " + b.add(reportingEarliest(frame)) + " AND " + spec.at + " < " + b.add(frame.EvaluatedAt)
	}
	if metric == reportingOpenPipeline || metric == reportingStageAge {
		where += reportingClosePredicate(frame, b)
	}
	return where
}
