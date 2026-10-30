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
	{crmcontracts.ReportingMetricDefinition{Id: reportingLeadsCreated, Version: "1", Unit: reportingCountUnit, TemporalBasis: reportingEventPeriod, Blocks: []crmcontracts.ReportingBlockKind{reportingBreakdown, reportingMetricReading}, Definition: reportingDefinitionWords[reportingLeadsCreated].En}, string(recordTypeLead), readReportingLeadsCreated},
	{crmcontracts.ReportingMetricDefinition{Id: reportingLeadStatusMoves, Version: "1", Unit: reportingCountUnit, TemporalBasis: reportingEventPeriod, Blocks: []crmcontracts.ReportingBlockKind{reportingBreakdown, reportingMetricReading}, Definition: reportingDefinitionWords[reportingLeadStatusMoves].En}, string(recordTypeLead), readReportingLeadStatusMoves},
	{crmcontracts.ReportingMetricDefinition{Id: reportingLeadsPromoted, Version: "1", Unit: reportingCountUnit, TemporalBasis: reportingEventPeriod, SupportsTarget: true, Blocks: []crmcontracts.ReportingBlockKind{reportingTargetProgress, reportingMetricReading}, Definition: reportingDefinitionWords[reportingLeadsPromoted].En}, string(recordTypeLead), readReportingLeadsPromoted},
	{crmcontracts.ReportingMetricDefinition{Id: reportingLeadFirstResponse, Version: "1", Unit: "hours", TemporalBasis: reportingEventPeriod, Blocks: []crmcontracts.ReportingBlockKind{reportingMetricReading}, Definition: reportingDefinitionWords[reportingLeadFirstResponse].En}, string(recordTypeLead), readReportingLeadFirstResponse},
	{crmcontracts.ReportingMetricDefinition{Id: reportingPartnerPipeline, Version: "1", Unit: reportingMoney, TemporalBasis: reportingEventPeriod, SupportsTarget: true, Blocks: []crmcontracts.ReportingBlockKind{reportingBreakdown, reportingTargetProgress, reportingMetricReading}, Definition: reportingDefinitionWords[reportingPartnerPipeline].En}, string(recordTypeDeal), readReportingPartnerPipeline},
	{crmcontracts.ReportingMetricDefinition{Id: reportingPartnerWon, Version: "1", Unit: reportingMoney, TemporalBasis: reportingEventPeriod, Blocks: []crmcontracts.ReportingBlockKind{reportingBreakdown, reportingMetricReading}, Definition: reportingDefinitionWords[reportingPartnerWon].En}, string(recordTypeDeal), readReportingPartnerWon},
	{crmcontracts.ReportingMetricDefinition{Id: reportingPartnerCommission, Version: "1", Unit: reportingMoney, TemporalBasis: reportingEventPeriod, Blocks: []crmcontracts.ReportingBlockKind{reportingBreakdown, reportingMetricReading}, Definition: reportingDefinitionWords[reportingPartnerCommission].En}, objectCommission, readReportingPartnerCommission},
	{crmcontracts.ReportingMetricDefinition{Id: reportingPartnerActivity, Version: "1", Unit: reportingCountUnit, TemporalBasis: reportingStateAt, Blocks: []crmcontracts.ReportingBlockKind{reportingBreakdown, reportingMetricReading}, Definition: reportingDefinitionWords[reportingPartnerActivity].En}, objectPartner, readReportingPartnerActivity},
}

// reportingPresets are the selections the Analytics screen opens on. A preset
// names metrics and charts only: each metric's meaning and read grant stay in
// reportingMetrics.
var reportingPresets = []crmcontracts.ReportingPreset{
	{Id: crmcontracts.ReportingPresetIdSales, Metrics: []crmcontracts.ReportingMetricID{reportingBookingsWon, reportingClosedWinRate, reportingOpenPipeline, reportingStageAge, reportingQualifiedPipelineCreated}, Blocks: []crmcontracts.ReportingBlockKind{crmcontracts.ReportingBlockKindBookingsTrend, crmcontracts.ReportingBlockKindStageDistribution, reportingOwnerAttainment, reportingStageAge}},
	{Id: crmcontracts.ReportingPresetIdSdr, Metrics: []crmcontracts.ReportingMetricID{reportingMeetingsHeld, reportingAcceptedOpportunities}, Blocks: []crmcontracts.ReportingBlockKind{reportingSdrOutcomes, reportingTargetProgress}},
}

// openingSelection is what the reporting doors answer a request that names no
// metrics with: the sales preset, which is what the Analytics screen opens on
// and what the daily benchmark measures.
//
// The doors took a copy of this before, and it had drifted: the preset gained
// closed_win_rate and qualified_pipeline_created, and the copy did not.
//
// An absent sales preset yields an empty selection, which the reporting door
// refuses as naming no metric. TestTheOpeningSelectionIsTheSalesPreset holds
// that it is present and that the two agree.
func openingSelection() reporting.OpeningSelection {
	for _, preset := range reportingPresets {
		if preset.Id == crmcontracts.ReportingPresetIdSales {
			return reporting.OpeningSelection{Metrics: preset.Metrics, Blocks: preset.Blocks}
		}
	}
	return reporting.OpeningSelection{}
}

// readablePresets narrows each preset to the offered metrics and the charts
// they draw, so the screen's opening request is one the reader may ask.
func readablePresets(offered []crmcontracts.ReportingMetricDefinition) []crmcontracts.ReportingPreset {
	out := []crmcontracts.ReportingPreset{}
	for _, preset := range reportingPresets {
		readable := crmcontracts.ReportingPreset{Id: preset.Id, Metrics: []crmcontracts.ReportingMetricID{}, Blocks: []crmcontracts.ReportingBlockKind{}}
		var drawn []crmcontracts.ReportingBlockKind
		for _, metric := range offered {
			if slices.Contains(preset.Metrics, metric.Id) {
				readable.Metrics = append(readable.Metrics, metric.Id)
				drawn = append(drawn, metric.Blocks...)
			}
		}
		for _, block := range preset.Blocks {
			if slices.Contains(drawn, block) {
				readable.Blocks = append(readable.Blocks, block)
			}
		}
		if len(readable.Metrics) > 0 {
			out = append(out, readable)
		}
	}
	return out
}

type metricEvaluator struct {
	forecast *forecasting.Store
}

func (metricEvaluator) Catalog(ctx context.Context, language textlang.Lang) (crmcontracts.ReportingCatalog, error) {
	out := crmcontracts.ReportingCatalog{Metrics: []crmcontracts.ReportingMetricDefinition{}}
	for _, spec := range reportingMetrics {
		if auth.Allows(ctx, spec.object, principal.ActionRead) {
			out.Metrics = append(out.Metrics, reportingMetricMetadata(spec, language))
		}
	}
	out.Presets = readablePresets(out.Metrics)
	return out, nil
}

func (e metricEvaluator) Evaluate(ctx context.Context, tx pgx.Tx, selection crmcontracts.ReportingSelection, framework crmcontracts.ReportingFramework, at time.Time) (reporting.Evaluation, error) {
	owners, err := readerMeasurableOwners(ctx, tx)
	if err != nil {
		return reporting.Evaluation{}, err
	}
	frame, err := reportingContext(ctx, tx, selection, framework, owners, at)
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
		facts, coverage, err = withholdColleagues(ctx, tx, frame, owners, facts, coverage)
		if err != nil {
			return out, err
		}
		out.Facts = append(out.Facts, facts...)
		out.Result.Metrics = append(out.Result.Metrics, projectReportingMetric(spec.definition, frame, facts, coverage))
	}
	if err := projectReportingCharts(&out); err != nil {
		return out, err
	}
	if err := explainWithheldOwners(ctx, tx, &out); err != nil {
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
