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
	"github.com/margince/margince/backend/internal/shared/kernel/formulacell"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// reportingHandlers names the module's handler set for the Server embed, since
// briefs.Handlers already holds the unqualified name.
type reportingHandlers = reporting.Handlers

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
	raw, err := reportingCSV(evaluation)
	writeReportingCSV(w, r, raw, err)
}

func (s reportingExportHandlers) ExportReportingEdition(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	edition, err := s.service.GetEdition(r.Context(), ids.UUID(id))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	raw, err := editionCSV(edition)
	writeReportingCSV(w, r, raw, err)
}

func reportingCSV(evaluation crmcontracts.ReportingEvaluation) ([]byte, error) {
	return encodeReportingCSV(reportingCSVRows(evaluation))
}

// editionCSV is the edition's readings, then each question card's kept
// answer in page order, as GetEdition judged it for this reader.
func editionCSV(edition crmcontracts.ReportingEdition) ([]byte, error) {
	return encodeReportingCSV(append(reportingCSVRows(edition.Evaluation), questionCSVRows(edition)...))
}

func reportingCSVRows(evaluation crmcontracts.ReportingEvaluation) [][]string {
	rows := [][]string{{"metric", "block", "label", extractionValueKey, reportingTarget, "comparison", "upper", "unit", reportingCurrency, "context", "coverage", "captured_at"}}
	appendRow := func(metric, block, label string, value, target, comparison, upper *float64, unit, context, status string) {
		// A count or a duration has no currency; naming one would read as money.
		currency := ""
		if unit == reportingMoney || unit == evaluation.Context.Currency {
			unit, currency = reportingMinorUnits, evaluation.Context.Currency
		}
		rows = append(rows, []string{metric, block, formulacell.Guard(label), reportingCSVNumber(value), reportingCSVNumber(target), reportingCSVNumber(comparison), reportingCSVNumber(upper), unit, currency, context, status, evaluation.Context.EvaluatedAt.Format(time.RFC3339Nano)})
	}
	for _, metric := range evaluation.Metrics {
		appendRow(string(metric.Id), "headline", reportingMetricName(evaluation.Context, metric.Id), metric.Value, metric.Target, nil, nil, metric.Unit, metric.Evidence.ContextId, string(metric.Coverage.Status))
		if comparison := metric.TargetComparison; comparison != nil {
			for _, figure := range targetComparisonCSV(*comparison, metric.Unit) {
				appendRow(string(metric.Id), reportingTargetComparisonBlock, figure.label, figure.value, nil, nil, nil, figure.unit, figure.context, figure.status)
			}
		}
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
	return rows
}

func encodeReportingCSV(rows [][]string) ([]byte, error) {
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

func writeReportingCSV(w http.ResponseWriter, r *http.Request, raw []byte, err error) {
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	(httperr.Download{ContentType: reportingCSVContentType, Filename: "reporting.csv", Size: int64(len(raw))}).WriteHeaders(w)
	w.Header().Set("Cache-Control", "no-store")
	if _, err := w.Write(raw); err != nil { // #nosec G705 -- a CSV attachment, never rendered as a page
		slog.WarnContext(r.Context(), "reporting export connection closed", "error", err)
	}
}

const (
	reportingTargetComparisonBlock = "target_comparison"
	comparisonStatusRow            = "status"
)

type comparisonFigure struct {
	label, unit, context, status string
	value                        *float64
}

// targetComparisonCSV is a comparison's status, then each figure it carries.
// A figure it does not carry has no row, so no blank reads as a zero.
func targetComparisonCSV(comparison crmcontracts.ReportingTargetComparison, unit string) []comparisonFigure {
	status := string(comparison.Status)
	why := ""
	if comparison.Incompatibility != nil {
		why = string(*comparison.Incompatibility)
	}
	out := []comparisonFigure{{label: comparisonStatusRow, context: why, status: status}}
	add := func(label, unit, status string, value *float64) {
		if value != nil {
			out = append(out, comparisonFigure{label: label, unit: unit, context: reportingTargetComparisonBlock, status: status, value: value})
		}
	}
	add("target", unit, status, comparison.Target)
	add("landing", unit, status, comparison.Landing)
	add("won", unit, status, comparison.Won)
	add("gap", unit, status, comparison.Gap)
	add("remaining", unit, status, comparison.Remaining)
	add("open_pipeline", unit, status, comparison.OpenPipeline)
	add("coverage", "ratio", status, comparison.Coverage)
	if qualified := comparison.Qualified; qualified != nil {
		out = append(out, comparisonFigure{label: "qualified_status", context: reportingTargetComparisonBlock, status: string(qualified.Status)})
		add("qualified_open_pipeline", unit, string(qualified.Status), qualified.OpenPipeline)
		add("qualified_coverage", "ratio", string(qualified.Status), qualified.Coverage)
	}
	return out
}
