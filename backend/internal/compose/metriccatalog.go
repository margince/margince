// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

type (
	metricReader func(context.Context, pgx.Tx, crmcontracts.ReportingContext, crmcontracts.ReportingFramework, metricEvaluator) ([]reporting.Fact, crmcontracts.ReportingCoverage, error)
	metricSpec   struct {
		definition crmcontracts.ReportingMetricDefinition
		object     string
		read       metricReader
	}
)

const reportingCountUnit = "count"

var reportingMetrics = []metricSpec{
	{crmcontracts.ReportingMetricDefinition{Id: reportingBookingsWon, Version: "2", Unit: reportingMoney, TemporalBasis: reportingEventPeriod, SupportsTarget: true, Blocks: []crmcontracts.ReportingBlockKind{crmcontracts.ReportingBlockKindBookingsTrend, reportingOwnerAttainment, reportingTargetProgress, reportingMetricReading}, Definition: reportingDefinitionWords[reportingBookingsWon].En}, string(recordTypeDeal), readReportingBookings},
	{crmcontracts.ReportingMetricDefinition{Id: reportingClosedWinRate, Version: "2", Unit: "percent", TemporalBasis: reportingEventPeriod, Blocks: []crmcontracts.ReportingBlockKind{reportingMetricReading}, Definition: reportingDefinitionWords[reportingClosedWinRate].En}, string(recordTypeDeal), readReportingOutcomes},
	{crmcontracts.ReportingMetricDefinition{Id: reportingOpenPipeline, Version: "1", Unit: reportingMoney, TemporalBasis: reportingStateAt, Blocks: []crmcontracts.ReportingBlockKind{crmcontracts.ReportingBlockKindStageDistribution, reportingPipelineMovement, reportingMetricReading}, Definition: reportingDefinitionWords[reportingOpenPipeline].En}, string(recordTypeDeal), readReportingOpen},
	{crmcontracts.ReportingMetricDefinition{Id: reportingStageAge, Version: "1", Unit: "days", TemporalBasis: reportingStateAt, Blocks: []crmcontracts.ReportingBlockKind{reportingStageAge, reportingMetricReading}, Definition: reportingDefinitionWords[reportingStageAge].En}, string(recordTypeDeal), readReportingAge},
	{crmcontracts.ReportingMetricDefinition{Id: reportingQualifiedPipelineCreated, Version: "1", Unit: reportingMoney, TemporalBasis: reportingEventPeriod, SupportsTarget: true, Blocks: []crmcontracts.ReportingBlockKind{reportingOwnerAttainment, reportingTargetProgress, reportingMetricReading}, Definition: reportingDefinitionWords[reportingQualifiedPipelineCreated].En}, string(recordTypeDeal), readReportingQualified},
	{crmcontracts.ReportingMetricDefinition{Id: reportingMeetingsHeld, Version: "2", Unit: reportingCountUnit, TemporalBasis: reportingEventPeriod, SupportsTarget: true, Blocks: []crmcontracts.ReportingBlockKind{reportingSdrOutcomes, reportingTargetProgress, reportingMetricReading}, Definition: reportingDefinitionWords[reportingMeetingsHeld].En}, string(recordTypeActivity), readReportingMeetings},
	{crmcontracts.ReportingMetricDefinition{Id: reportingAcceptedOpportunities, Version: "1", Unit: reportingCountUnit, TemporalBasis: reportingEventPeriod, SupportsTarget: true, Blocks: []crmcontracts.ReportingBlockKind{reportingSdrOutcomes, reportingTargetProgress, reportingMetricReading}, Definition: reportingDefinitionWords[reportingAcceptedOpportunities].En}, reportingCreditObject, readReportingAccepted},
	{crmcontracts.ReportingMetricDefinition{Id: reportingForecastLanding, Version: "1", Unit: reportingMoney, TemporalBasis: reportingStateAt, Blocks: []crmcontracts.ReportingBlockKind{crmcontracts.ReportingBlockKindForecastSupport, reportingMetricReading}, Definition: reportingDefinitionWords[reportingForecastLanding].En}, objectForecast, readReportingForecast},
}

type metricEvaluator struct {
	forecast *forecasting.Store
}

func (metricEvaluator) Catalog(ctx context.Context, language textlang.Lang) (crmcontracts.ReportingCatalog, error) {
	out := crmcontracts.ReportingCatalog{Metrics: []crmcontracts.ReportingMetricDefinition{}}
	for _, spec := range reportingMetrics {
		if auth.Allows(ctx, spec.object, principal.ActionRead) {
			definition := reportingMetricMetadata(spec)
			definition.Definition = reportingDefinitionWords[definition.Id].In(language)
			out.Metrics = append(out.Metrics, definition)
		}
	}
	return out, nil
}

func (e metricEvaluator) Evaluate(ctx context.Context, tx pgx.Tx, selection crmcontracts.ReportingSelection, framework crmcontracts.ReportingFramework, at time.Time) (reporting.Evaluation, error) {
	frame, err := reportingContext(ctx, tx, selection, framework, at)
	if err != nil {
		return reporting.Evaluation{}, err
	}

	selection.Scope = frame.Scope
	out := reporting.Evaluation{Result: crmcontracts.ReportingEvaluation{Context: frame, Selection: selection, Metrics: []crmcontracts.ReportingMetric{}, Charts: []crmcontracts.ReportingChart{}}, Facts: []reporting.Fact{}}
	for _, spec := range reportingMetrics {
		if !slices.Contains(selection.Metrics, spec.definition.Id) {
			continue
		}
		if err := auth.Require(ctx, spec.object, principal.ActionRead); err != nil {
			return out, err
		}
		facts, coverage, err := e.readIsolated(ctx, tx, spec, frame, framework)
		if err != nil {
			return out, err
		}
		if err := markOwnerWithheld(ctx, tx, facts); err != nil {
			return out, err
		}
		out.Facts = append(out.Facts, facts...)
		out.Result.Metrics = append(out.Result.Metrics, projectReportingMetric(spec.definition, frame, facts, coverage))
	}
	if err := projectReportingCharts(&out); err != nil {
		return out, err
	}
	for index, chart := range out.Result.Charts {
		if chart.Kind != reportingPipelineMovement || chart.Coverage.Status == reportingFailed {
			continue
		}
		var projected crmcontracts.ReportingChart
		var facts []reporting.Fact
		failed, err := underSavepoint(ctx, tx, string(chart.Kind), func(savepoint pgx.Tx) error {
			var err error
			projected, facts, err = e.movementChart(ctx, savepoint, frame, chart)
			return err
		})
		if err != nil {
			return out, err
		}
		if failed {
			chart.Coverage = calculationFailed(frame)
			chart.Points = []crmcontracts.ReportingPoint{}
			out.Result.Charts[index] = chart
			continue
		}
		out.Result.Charts[index] = projected
		out.Facts = append(out.Facts, facts...)
	}
	return out, nil
}
