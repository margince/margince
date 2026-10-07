// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"encoding/csv"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type reportingExportHandlers struct{ service *reporting.Service }

func (s reportingExportHandlers) ExportReportingEvaluation(w http.ResponseWriter, r *http.Request, params crmcontracts.ExportReportingEvaluationParams) {
	selection, err := reporting.SelectionFromQuery(r.URL.Query())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	evaluation, err := s.service.EvaluatedReceipt(r.Context(), selection, reporting.EvidenceExpectation{Key: params.EvaluationKey, At: params.EvaluatedAt, FrameworkRevision: params.FrameworkRevision})
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	writeReportingCSV(w, r, evaluation)
}

func (s reportingExportHandlers) ExportReportingEdition(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	edition, err := s.service.GetEdition(r.Context(), ids.UUID(id))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	writeReportingCSV(w, r, edition.Evaluation)
}

func reportingCSV(evaluation crmcontracts.ReportingEvaluation) ([]byte, error) {
	rows := [][]string{{"metric", "block", "label", extractionValueKey, reportingTarget, "comparison", "upper", "unit", reportingCurrency, "context", "coverage", "captured_at"}}
	appendRow := func(metric, block, label string, value, target, comparison, upper *float64, unit, context, status string) {
		if unit == reportingMoney || unit == evaluation.Context.Currency {
			unit = "minor_units"
		}
		rows = append(rows, []string{metric, block, guardCSVFormula(label), reportingCSVNumber(value), reportingCSVNumber(target), reportingCSVNumber(comparison), reportingCSVNumber(upper), unit, evaluation.Context.Currency, context, status, evaluation.Context.EvaluatedAt.Format(time.RFC3339Nano)})
	}
	for _, metric := range evaluation.Metrics {
		appendRow(string(metric.Id), "headline", string(metric.Id), metric.Value, metric.Target, nil, nil, metric.Unit, metric.Evidence.ContextId, string(metric.Coverage.Status))
	}
	for _, chart := range evaluation.Charts {
		for _, point := range chart.Points {
			appendRow(string(chart.Metric), string(chart.Kind), point.Label, point.Value, point.Target, point.Comparison, point.Upper, chart.Unit, chart.ContextId, string(point.Status))
		}
		if chart.Opening != nil {
			appendRow(string(chart.Metric), string(chart.Kind), "opening", chart.Opening, nil, nil, nil, chart.Unit, chart.ContextId, string(chart.Coverage.Status))
		}
		if chart.Closing != nil {
			appendRow(string(chart.Metric), string(chart.Kind), "closing", chart.Closing, nil, nil, nil, chart.Unit, chart.ContextId, string(chart.Coverage.Status))
		}
	}
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	if err := writer.WriteAll(rows); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func reportingCSVNumber(value *float64) string {
	if value == nil {
		return ""
	}
	return strconv.FormatFloat(*value, 'f', -1, 64)
}

func writeReportingCSV(w http.ResponseWriter, r *http.Request, evaluation crmcontracts.ReportingEvaluation) {
	raw, err := reportingCSV(evaluation)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	(httperr.Download{ContentType: reportingCSVContentType, Filename: "reporting.csv", Size: int64(len(raw))}).WriteHeaders(w)
	w.Header().Set("Cache-Control", "no-store")
	if _, err := w.Write(raw); err != nil {
		slog.WarnContext(r.Context(), "reporting export connection closed", "error", err)
	}
}
