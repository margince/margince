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
	// allocatable says the reader values a won deal, so a product or value
	// basis replaces the deal's amount with its allocations.
	allocatable bool
	// binds are the values the reader's SQL names as `@name`, bound in the
	// reader's own argument list so no placeholder is numbered twice.
	binds map[string]any
	// group renders the breakdown key and label columns; nil for a metric
	// with no breakdown.
	group func(context.Context, *reportingBindings) (key, label string, err error)
}

func readDealMetricFacts(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, metric crmcontracts.ReportingMetricID, spec metricFactQuery) ([]reporting.Fact, crmcontracts.ReportingCoverage, error) {
	var b reportingBindings
	for name, value := range spec.binds {
		placeholder := b.add(value)
		spec.from = strings.ReplaceAll(spec.from, "@"+name, placeholder)
		spec.value = strings.ReplaceAll(spec.value, "@"+name, placeholder)
	}
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
	dimensions, err := dealDimensionSQL(ctx, frame, &b, "t")
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	where := spec.where + " AND " + population + " AND " + rowScope + " AND " + fieldScope
	where += reportingFactFilters(frame, metric, spec, &b) + dimensions
	position := spec.stagePosition
	if position == "" {
		position = "0"
	}
	said := reporting.ExplanationCopy(frame)
	stageLabel := "COALESCE(" + spec.stageLabel + "," + b.add(said.Say(reportingLabelWords.Unmapped)) + ")"
	groupKey, groupLabel := "''", "''"
	if spec.group != nil {
		if groupKey, groupLabel, err = spec.group(ctx, &b); err != nil {
			return nil, crmcontracts.ReportingCoverage{}, err
		}
	}
	query := "SELECT " + spec.key + ",t.id,t.name," + spec.owner + ",COALESCE(u.display_name," + b.add(said.Say(reportingLabelWords.Unassigned)) + ")," + spec.at + "," + value + "," + spec.stage + "," + stageLabel + "," + spec.outcome + "," + spec.provenance + "," + position + "," + groupKey + "," + groupLabel + " FROM deal t " + from + " LEFT JOIN app_user u ON u.id=" + spec.owner + " WHERE " + where + " ORDER BY " + spec.key + " LIMIT " + b.add(reportingFactLimit+1)
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

// scanDealMetricFact scans the fourteen columns every deal reader selects, then
// any columns a reader adds after them into extra.
//
//craft:ignore naked-any pgx scans into destinations of the caller's own types.
func scanDealMetricFact(row pgx.CollectableRow, metric crmcontracts.ReportingMetricID, money bool, extra ...any) (reporting.Fact, error) {
	fact := reporting.Fact{Metric: metric, SourceType: string(recordTypeDeal)}
	var owner *ids.UUID
	var value *float64
	var amount *int64
	measure := any(&value)
	if money {
		measure = &amount
	}
	err := row.Scan(append([]any{&fact.Row.Key, &fact.SourceID, &fact.Row.Label, &owner, &fact.OwnerLabel, &fact.Row.OccurredAt, measure, &fact.StageID, &fact.StageLabel, &fact.Outcome, &fact.Provenance, &fact.StagePosition, &fact.GroupKey, &fact.GroupLabel}, extra...)...)
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
	query := metricFactQuery{from: deals.CurrentClosingJoin("t", "closing"), where: "t.archived_at IS NULL AND t.status <> 'open'", owner: colOwnerID, at: "t.closed_at", key: "COALESCE(closing.id,t.id)::text", stage: "''", stageLabel: "''", outcome: "t.status", provenance: "'current_closing'", money: money, allocatable: money}
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
	query := metricFactQuery{stagePosition: "COALESCE(s.position,0)", from: "LEFT JOIN stage s ON s.id=t.stage_id", where: "t.archived_at IS NULL AND t.status='open'", owner: colOwnerID, at: "NULL::timestamptz", key: "t.id::text", stage: "t.stage_id::text", stageLabel: "s.name", outcome: "'open'", provenance: "'current_state'", money: !age}
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
		if value == "" && spec.allocatable && (frame.ProductId != nil || reportingValueBasis(frame) != reportingValueBasisAll) {
			value = allocationValueSQL(frame, b, "t")
		}
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

// readReportingBookingsMix reads one row per allocation of a won deal, each
// under its component kind. The deal's unallocated remainder is a group of
// its own. A product selection keeps that product's allocations alone and
// drops the remainder, which is not that product's.
func readReportingBookingsMix(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, _ crmcontracts.ReportingFramework, _ metricEvaluator) ([]reporting.Fact, crmcontracts.ReportingCoverage, error) {
	binds := map[string]any{"currency": frame.Currency}
	parts := "SELECT dp.product_id::text AS part, COALESCE(dp.component_kind,'unclassified') AS kind, dp.amount_minor AS amount, dp.currency FROM deal_product dp WHERE dp.deal_id=t.id AND dp.amount_minor IS NOT NULL"
	if frame.ProductId != nil {
		binds["product"] = *frame.ProductId
		parts += " AND dp.product_id=@product"
	} else {
		parts += " UNION ALL SELECT 'unallocated', 'unallocated', t.amount_minor-(SELECT COALESCE(SUM(x.amount_minor),0) FROM deal_product x WHERE x.deal_id=t.id AND x.currency=t.currency), t.currency"
	}
	if basis := reportingValueBasis(frame); basis != reportingValueBasisAll {
		binds["basis"] = string(basis)
		parts = "SELECT * FROM (" + parts + ") kinds WHERE kinds.kind=@basis"
	}
	said := reporting.ExplanationCopy(frame)
	query := closingMetricQuery(true)
	query.allocatable = false
	query.binds = binds
	query.from += " JOIN LATERAL (" + parts + ") part ON part.amount>0"
	query.key = "COALESCE(closing.id,t.id)::text||':'||part.part"
	query.value = allocationConvertedSQL("part.amount", "part.currency IS DISTINCT FROM t.currency", "t", "@currency")
	query.group = func(_ context.Context, b *reportingBindings) (string, string, error) {
		var label strings.Builder
		label.WriteString("CASE part.kind")
		for kind, phrase := range reportingComponentKindLabels {
			label.WriteString(" WHEN '" + kind + "' THEN " + b.add(said.Say(phrase)))
		}
		label.WriteString(" END")
		return "'kind:'||part.kind", label.String(), nil
	}
	return readDealMetricFacts(ctx, tx, frame, reportingBookingsMix, query)
}
