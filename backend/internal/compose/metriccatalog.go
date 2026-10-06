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
)

type (
	metricReader func(context.Context, pgx.Tx, crmcontracts.ReportingContext, crmcontracts.ReportingFramework, metricEvaluator) ([]reporting.Fact, crmcontracts.ReportingCoverage, error)
	metricSpec   struct {
		definition crmcontracts.ReportingMetricDefinition
		object     string
		read       metricReader
	}
)

var reportingMetrics = []metricSpec{
	{crmcontracts.ReportingMetricDefinition{Id: reportingBookingsWon, Version: "2", Unit: reportingMoney, TemporalBasis: reportingEventPeriod, SupportsTarget: true, Blocks: []crmcontracts.ReportingBlockKind{"bookings_trend", reportingOwnerAttainment, reportingTargetProgress, reportingMetricReading}, Definition: "Won deals in the selected close interval, using their current valid closing and frozen base value. Results follow the current deal owner, including after reassignment. Saved editions retain the owner and values captured then."}, string(recordTypeDeal), readReportingBookings},
	{crmcontracts.ReportingMetricDefinition{Id: reportingClosedWinRate, Version: "2", Unit: "percent", TemporalBasis: reportingEventPeriod, Blocks: []crmcontracts.ReportingBlockKind{reportingMetricReading}, Definition: "Won ÷ (won + lost) × 100 in the same current closing cohort, scoped to the current deal owner. Reopened deals no longer count. Suppressed when fewer than five outcomes or when contributions are withheld."}, string(recordTypeDeal), readReportingOutcomes},
	{crmcontracts.ReportingMetricDefinition{Id: reportingOpenPipeline, Version: "1", Unit: reportingMoney, TemporalBasis: reportingStateAt, Blocks: []crmcontracts.ReportingBlockKind{"stage_distribution", reportingPipelineMovement, reportingMetricReading}, Definition: "Open deals now, valued in the reporting base currency, with a separate expected-close window. A past event interval does not reconstruct past pipeline."}, string(recordTypeDeal), readReportingOpen},
	{crmcontracts.ReportingMetricDefinition{Id: reportingStageAge, Version: "1", Unit: "days", TemporalBasis: reportingStateAt, Blocks: []crmcontracts.ReportingBlockKind{reportingStageAge, reportingMetricReading}, Definition: "Median and 75th percentile of completed days since latest entry into each open deal's current stage. Creation date is the fallback when no entry history exists. At least five observations are required."}, string(recordTypeDeal), readReportingAge},
	{crmcontracts.ReportingMetricDefinition{Id: reportingQualifiedPipelineCreated, Version: "1", Unit: reportingMoney, TemporalBasis: reportingEventPeriod, SupportsTarget: true, Blocks: []crmcontracts.ReportingBlockKind{reportingOwnerAttainment, reportingTargetProgress, reportingMetricReading}, Definition: "The first non-reversed entry into a stage configured as qualified at the transition. Credit and valuation belong to that event. Later regression and re-entry do not create additional qualified pipeline."}, string(recordTypeDeal), readReportingQualified},
	{crmcontracts.ReportingMetricDefinition{Id: reportingMeetingsHeld, Version: "2", Unit: "count", TemporalBasis: reportingEventPeriod, SupportsTarget: true, Blocks: []crmcontracts.ReportingBlockKind{reportingSdrOutcomes, reportingTargetProgress, reportingMetricReading}, Definition: "Distinct eligible customer meetings scheduled in the interval and confirmed held by capture time. Recorded held transitions retain their host and customer eligibility. Older meetings without attribution history use their current host and customer links, with partial coverage. Future and unresolved meetings do not count."}, string(recordTypeActivity), readReportingMeetings},
	{crmcontracts.ReportingMetricDefinition{Id: reportingAcceptedOpportunities, Version: "1", Unit: "count", TemporalBasis: reportingEventPeriod, SupportsTarget: true, Blocks: []crmcontracts.ReportingBlockKind{reportingSdrOutcomes, reportingTargetProgress, reportingMetricReading}, Definition: "Distinct opportunities first credited through an accepted handoff in the interval. The first acceptance wins across repeated submissions. Credit remains with the originating SDR after transfer; restricted source details remain hidden."}, reportingCreditObject, readReportingAccepted},
	{crmcontracts.ReportingMetricDefinition{Id: reportingForecastLanding, Version: "1", Unit: reportingMoney, TemporalBasis: reportingStateAt, Blocks: []crmcontracts.ReportingBlockKind{"forecast_support", reportingMetricReading}, Definition: "The existing forecast landing for this fiscal quarter and supported scope. Won plus supported open is evidence-based landing; the manager call is a whole-period replacement, never another additive segment."}, objectForecast, readReportingForecast},
}

type metricEvaluator struct{ forecast *forecasting.Store }

func (metricEvaluator) Catalog(ctx context.Context) (crmcontracts.ReportingCatalog, error) {
	out := crmcontracts.ReportingCatalog{Metrics: []crmcontracts.ReportingMetricDefinition{}}
	for _, spec := range reportingMetrics {
		if auth.Allows(ctx, spec.object, principal.ActionRead) {
			out.Metrics = append(out.Metrics, reportingMetricMetadata(spec))
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
		facts, coverage, err := spec.read(ctx, tx, frame, framework, e)
		if err != nil {
			return out, err
		}
		coverage, err = reportingHistoryCoverage(ctx, tx, frame, spec.definition.Id, coverage)
		if err != nil {
			return out, err
		}
		out.Facts = append(out.Facts, facts...)
		out.Result.Metrics = append(out.Result.Metrics, projectReportingMetric(spec.definition, frame, facts, coverage))
	}
	if err := projectReportingCharts(&out); err != nil {
		return out, err
	}
	for index, chart := range out.Result.Charts {
		if chart.Kind != reportingPipelineMovement {
			continue
		}
		projected, facts, err := e.movementChart(ctx, tx, frame, chart)
		if err != nil {
			return out, err
		}
		out.Result.Charts[index] = projected
		out.Facts = append(out.Facts, facts...)
	}
	return out, nil
}
