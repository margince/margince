// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/reportdoc"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type reportMetricReader struct {
	service *reporting.Service
	cache   map[string]crmcontracts.ReportingEvaluation
}

func (r *reportMetricReader) read(ctx context.Context, tx pgx.Tx, cell reportdoc.Cell) (RenderedValue, error) {
	if r.service == nil {
		return RenderedValue{}, apperrors.ErrNotFound
	}
	evaluation, metric, err := r.evaluate(ctx, tx, cell)
	if err != nil {
		return RenderedValue{}, err
	}
	for _, reading := range evaluation.Metrics {
		if reading.Id != metric {
			continue
		}
		value := RenderedValue{Withheld: reading.Coverage.Withheld || reading.Coverage.Status == "suppressed", Coverage: &reading.Coverage, Context: &evaluation.Context, Unit: &reading.Unit, DefinitionVersion: &reading.Version}
		if reading.Value != nil && !value.Withheld {
			value.Value = *reading.Value
		}
		return value, nil
	}
	return RenderedValue{}, apperrors.ErrNotFound
}

func (r *reportMetricReader) evaluate(ctx context.Context, tx pgx.Tx, cell reportdoc.Cell) (crmcontracts.ReportingEvaluation, crmcontracts.ReportingMetricID, error) {
	if ref := cell.EditionRef; ref != nil {
		key := "edition:" + ref.EditionId.String()
		if evaluation, ok := r.cache[key]; ok {
			return evaluation, ref.Metric, nil
		}
		edition, err := r.service.GetEditionTx(ctx, tx, ids.UUID(ref.EditionId))
		if err == nil {
			r.cache[key] = edition.Evaluation
		}
		return edition.Evaluation, ref.Metric, err
	}
	if ref := cell.MetricRef; ref != nil {
		if !slices.Contains(ref.Selection.Metrics, ref.Metric) {
			return crmcontracts.ReportingEvaluation{}, ref.Metric, apperrors.ErrInvalidArgument
		}
		raw, err := json.Marshal(ref.Selection)
		if err != nil {
			return crmcontracts.ReportingEvaluation{}, ref.Metric, err
		}
		key := "live:" + string(raw)
		if evaluation, ok := r.cache[key]; ok {
			return evaluation, ref.Metric, nil
		}
		evaluation, err := r.service.EvaluateTx(ctx, tx, ref.Selection)
		if err == nil {
			r.cache[key] = evaluation
		}
		return evaluation, ref.Metric, err
	}
	return crmcontracts.ReportingEvaluation{}, "", apperrors.ErrInvalidArgument
}
