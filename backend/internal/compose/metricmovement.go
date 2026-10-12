// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type reportingCapture struct {
	id ids.UUID
	at time.Time
}

func (e metricEvaluator) movementChart(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, chart crmcontracts.ReportingChart) (crmcontracts.ReportingChart, []reporting.Fact, error) {
	if !auth.Allows(ctx, objectForecast, principal.ActionRead) {
		chart.Coverage = reportingGap("unsupported", "Forecast access is required for captured movement")
		return chart, nil, nil
	}
	if frame.CloseInterval == nil || frame.Scope.Kind == ScopeKindManagedTeams {
		chart.Coverage = reportingGap("unsupported", "Choose a fixed population and fiscal-quarter close window for movement")
		return chart, nil, nil
	}
	status, err := e.forecast.CaptureStatusTx(ctx, tx, forecasting.Scope{Kind: string(frame.Scope.Kind), ID: (*ids.UUID)(frame.Scope.Id)}, (*ids.UUID)(frame.PipelineId))
	if err != nil {
		return chart, nil, err
	}
	chart.CaptureStatus = status
	captures, err := reportingMovementCaptures(ctx, tx, frame)
	if err != nil {
		return chart, nil, err
	}
	if len(captures) > 0 {
		chart.SnapshotId = ptrUUID(captures[0].id)
		chart.StateAt = &captures[0].at
	}
	if len(captures) < 2 {
		return chart, nil, nil
	}
	return e.capturedMovementChart(ctx, tx, captures, chart)
}

func reportingMovementCaptures(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext) ([]reportingCapture, error) {
	period, _, err := ForecastPeriodAt(ctx, tx, forecasting.PeriodQuarter, frame.EvaluatedAt)
	if err != nil {
		return nil, err
	}
	var b reportingBindings
	query := "SELECT id,taken_at FROM forecast_snapshot WHERE scope_kind=" + b.add(frame.Scope.Kind) + " AND scope_id IS NOT DISTINCT FROM " + b.add(frame.Scope.Id) + "::uuid AND pipeline_id IS NOT DISTINCT FROM " + b.add(frame.PipelineId) + "::uuid AND population_fingerprint=" + b.add(frame.PopulationFingerprint) + " AND base_currency=" + b.add(frame.Currency) + " AND definition_version=" + b.add(forecasting.DefinitionVersion) + " AND period_start=" + b.add(period.StartDate) + " AND period_end=" + b.add(period.EndDate) + " AND taken_at<=" + b.add(frame.EvaluatedAt) + " ORDER BY taken_at DESC,id DESC LIMIT " + b.add(32)
	rows, err := tx.Query(ctx, query, b.values...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (reportingCapture, error) {
		var capture reportingCapture
		err := row.Scan(&capture.id, &capture.at)
		return capture, err
	})
}

func movementAnchorFacts(contributions []forecasting.Contribution, contextID string, at time.Time) ([]reporting.Fact, error) {
	facts := []reporting.Fact{}
	for _, contribution := range contributions {
		if !contribution.InOpen {
			continue
		}
		fact, err := movementSourceFact(contribution, contextID, at)
		if err != nil {
			return nil, err
		}
		fact.Money = contribution.BaseMinor
		if fact.Money != nil {
			value := float64(*fact.Money)
			fact.Row.Value = &value
		}
		facts = append(facts, fact)
	}
	return facts, nil
}

func movementSourceFact(contribution forecasting.Contribution, contextID string, at time.Time) (reporting.Fact, error) {
	source, err := ids.Parse(contribution.DealID)
	if err != nil {
		return reporting.Fact{}, err
	}
	owner := ids.Nil
	if contribution.Owner != "" {
		owner, err = ids.Parse(contribution.Owner)
		if err != nil {
			return reporting.Fact{}, err
		}
	}
	fact := reporting.Fact{Metric: reportingOpenPipeline, ContextID: contextID, SourceType: string(recordTypeDeal), SourceID: source, OwnerID: owner}
	fact.Row = crmcontracts.ReportingEvidenceRow{Key: contextID + ":" + contribution.DealID, Label: "Captured pipeline contribution", OccurredAt: &at, SourceType: &fact.SourceType, SourceId: ptrUUID(source), OwnerId: ptrUUID(owner)}
	return fact, nil
}

func movementDeltaFacts(deltas []forecasting.DealDelta, before, after []forecasting.Contribution, at time.Time) ([]reporting.Fact, error) {
	sources := map[string]forecasting.Contribution{}
	for _, source := range before {
		sources[source.DealID] = source
	}
	for _, source := range after {
		sources[source.DealID] = source
	}
	facts := []reporting.Fact{}
	for _, delta := range deltas {
		fact, err := movementSourceFact(sources[delta.DealID], "movement_delta", at)
		if err != nil {
			return nil, err
		}
		value := float64(delta.AmountMinor)
		fact.Money = &delta.AmountMinor
		fact.Row.Value = &value
		fact.GroupKey = delta.Bucket
		facts = append(facts, fact)
	}
	return facts, nil
}

func projectMovement(chart crmcontracts.ReportingChart, movement forecasting.Movement, facts []reporting.Fact, opening, closing reportingCapture) (crmcontracts.ReportingChart, error) {
	for _, value := range []int64{movement.OpeningMinor, movement.ClosingMinor} {
		if value > reportingExactInteger || value < -reportingExactInteger {
			return chart, errors.New("captured movement exceeds exact reporting range")
		}
	}
	chart.ContextId = "movement"
	chart.SnapshotId = ptrUUID(closing.id)
	chart.OpeningSnapshotId = ptrUUID(opening.id)
	chart.Coverage = crmcontracts.ReportingCoverage{Status: "ok"}
	for _, fact := range facts {
		if fact.Money == nil {
			chart.Coverage = reportingGap(reportingPartial, "Some captured deals could not be valued; movement covers priced contributions only")
			break
		}
	}
	chart.Interval = &crmcontracts.ReportingWindow{StartAt: opening.at, EndAt: closing.at}
	chart.StateAt = &closing.at
	a, b := float64(movement.OpeningMinor), float64(movement.ClosingMinor)
	chart.Opening, chart.Closing = &a, &b
	for _, bucket := range movement.Buckets {
		if bucket.AmountMinor == 0 {
			continue
		}
		value := float64(bucket.AmountMinor)
		if math.Abs(value) > float64(reportingExactInteger) {
			return chart, errors.New("movement bucket exceeds exact reporting range")
		}
		group := bucket.Name
		chart.Points = append(chart.Points, crmcontracts.ReportingPoint{Key: group, Label: group, Value: &value, Status: "ok", Evidence: &crmcontracts.ReportingEvidenceRef{Metric: reportingOpenPipeline, ContextId: "movement_delta", GroupKey: &group}})
	}
	return chart, nil
}

func (e metricEvaluator) capturedMovementChart(ctx context.Context, tx pgx.Tx, captures []reportingCapture, chart crmcontracts.ReportingChart) (crmcontracts.ReportingChart, []reporting.Fact, error) {
	closing, opening := captures[0], captures[1]
	for _, candidate := range captures[1:] {
		if !candidate.at.After(closing.at.AddDate(0, 0, -7)) {
			opening = candidate
			break
		}
	}
	before, err := e.forecast.SnapshotSide(ctx, tx, opening.id)
	if err != nil {
		return chart, nil, err
	}
	after, err := e.forecast.SnapshotSide(ctx, tx, closing.id)
	if err != nil {
		return chart, nil, err
	}
	facts, err := movementAnchorFacts(before.Contributions, "movement_opening", opening.at)
	if err != nil {
		return chart, nil, err
	}
	closingFacts, err := movementAnchorFacts(after.Contributions, "movement_closing", closing.at)
	if err != nil {
		return chart, nil, err
	}
	facts = append(facts, closingFacts...)
	_, withheld, err := (reportingAuthority{}).Visible(ctx, tx, facts)
	if err != nil {
		return chart, nil, err
	}
	if withheld || before.Withheld || after.Withheld {
		chart.Coverage = reportingGap(reportingUnavailable, "Some captured contributions are restricted")
		chart.Coverage.Withheld = true
		return chart, nil, nil
	}
	movement, err := e.forecast.MovementTx(ctx, tx, forecasting.ReadingOpen, opening.id, closing.id)
	if err != nil {
		return chart, nil, err
	}
	chart, err = projectMovement(chart, movement, facts, opening, closing)
	if err != nil {
		return chart, nil, err
	}
	deltas, err := movementDeltaFacts(movement.Deals, before.Contributions, after.Contributions, closing.at)
	if err != nil {
		return chart, nil, err
	}
	facts = append(facts, deltas...)
	for i := range facts {
		facts[i].Provenance = "forecast_snapshot:" + opening.id.String() + ":" + closing.id.String()
	}
	return chart, facts, nil
}
