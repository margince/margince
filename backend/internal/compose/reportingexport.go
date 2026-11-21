// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"encoding/csv"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/margince/margince/backend/internal/compose/reportingcsv"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/formulacell"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

// reportingHandlers names the module's handler set for the Server embed, since
// briefs.Handlers already holds the unqualified name.
type reportingHandlers = reporting.Handlers

type reportingExportHandlers struct{ service *reporting.Service }

func (s reportingExportHandlers) ExportReportingEvaluation(w http.ResponseWriter, r *http.Request, params crmcontracts.ExportReportingEvaluationParams) {
	selection, err := reporting.SelectionFromQuery(r.URL.Query(), openingSelection())
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
	writeReportingCSV(w, r, reportingcsv.Filename(evaluation.Context), raw, err)
}

func (s reportingExportHandlers) ExportReportingEdition(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	edition, err := s.service.GetEdition(r.Context(), ids.UUID(id))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	names, err := s.service.ReaderCopy(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	raw, err := editionCSV(edition, names)
	writeReportingCSV(w, r, reportingcsv.Filename(edition.Evaluation.Context), raw, err)
}

func reportingCSV(evaluation crmcontracts.ReportingEvaluation) ([]byte, error) {
	return encodeReportingCSV(reportingCSVRows(evaluation, reporting.ExplanationCopy(evaluation.Context)))
}

// editionCSV is the edition's readings, then each question card's kept
// answer in page order, as GetEdition judged it for this reader. Catalog
// names follow the reader's language; text frozen with the edition stays.
func editionCSV(edition crmcontracts.ReportingEdition, names langcopy.Spoken) ([]byte, error) {
	return encodeReportingCSV(append(reportingCSVRows(edition.Evaluation, names), questionCSVRows(edition)...))
}

// reportingCSVRows writes metric and label names from the catalog in names.
func reportingCSVRows(evaluation crmcontracts.ReportingEvaluation, say langcopy.Spoken) [][]string {
	rows := [][]string{{reportingMetric, "block", "label", extractionValueKey, reportingTarget, "comparison", "upper", "unit", reportingCurrency, "context", "coverage", reportingWithheld, "coverage_reason", "captured_at"}}
	appendRow := func(metric, block, label string, value, target, comparison, upper *float64, unit, context string, coverage csvCoverage) {
		// A count or a duration has no currency; naming one would read as money.
		currency, number := "", reportingCSVNumber
		if unit == reportingMoney || unit == evaluation.Context.Currency {
			unit, currency = reportingMoney, evaluation.Context.Currency
			number = func(minor *float64) string { return reportingcsv.Major(minor, currency) }
		}
		rows = append(rows, []string{metric, block, formulacell.Guard(label), number(value), number(target), number(comparison), number(upper), unit, currency, context, coverage.status, strconv.FormatBool(coverage.withheld), coverage.reason, evaluation.Context.EvaluatedAt.Format(time.RFC3339Nano)})
	}
	for _, metric := range evaluation.Metrics {
		covered := csvCoverageOf(metric.Coverage)
		appendRow(string(metric.Id), "headline", reportingMetricName(say, metric.Id), metric.Value, metric.Target, nil, nil, metric.Unit, metric.Evidence.ContextId, covered)
		if comparison := metric.TargetComparison; comparison != nil {
			for _, figure := range targetComparisonCSV(*comparison, metric.Unit) {
				appendRow(string(metric.Id), reportingTargetComparisonBlock, say.Say(reportingcsv.Labels[figure.label]), figure.value, nil, nil, nil, figure.unit, figure.context, covered.as(figure.status))
			}
		}
		for _, figure := range forecastBasisCSV(metric) {
			appendRow(string(metric.Id), reportingForecastBasisBlock, say.Say(reportingcsv.Labels[figure.label]), figure.value, nil, nil, nil, metric.Unit, figure.context, covered)
		}
	}
	for _, chart := range evaluation.Charts {
		covered := csvCoverageOf(chart.Coverage)
		for _, point := range chart.Points {
			appendRow(string(chart.Metric), string(chart.Kind), point.Label, point.Value, point.Target, point.Comparison, point.Upper, chart.Unit, chart.ContextId, covered.as(string(point.Status)))
		}
		if chart.Opening != nil {
			appendRow(string(chart.Metric), string(chart.Kind), say.Say(reportingcsv.Labels["opening"]), chart.Opening, nil, nil, nil, chart.Unit, chart.ContextId, covered)
		}
		if chart.Closing != nil {
			appendRow(string(chart.Metric), string(chart.Kind), say.Say(reportingcsv.Labels["closing"]), chart.Closing, nil, nil, nil, chart.Unit, chart.ContextId, covered)
		}
	}
	// The page writes the assessment from the figures above, so the file names
	// the card instead of keeping a second copy of its sentence rules.
	// Written whole, since it holds no figure for appendRow to give a unit.
	if placesAssessment(evaluation.Selection) {
		rows = append(rows, []string{assessmentRow, assessmentRow, say.Say(reportingcsv.Labels[assessmentRow]), "", "", "", "", "", "", "", "not_exported", strconv.FormatBool(false), "", evaluation.Context.EvaluatedAt.Format(time.RFC3339Nano)})
	}
	return rows
}

const assessmentRow = string(crmcontracts.ReportingCardKindAssessment)

func placesAssessment(selection crmcontracts.ReportingSelection) bool {
	if selection.Layout == nil {
		return false
	}
	for _, section := range *selection.Layout {
		if slices.ContainsFunc(section.Cards, func(card crmcontracts.ReportingCard) bool {
			return card.Kind == crmcontracts.ReportingCardKindAssessment
		}) {
			return true
		}
	}
	return false
}

// csvCoverage is what one row says of its figure's coverage. The withheld flag
// and reason are the metric's or chart's, so an owner chart holding only the
// reader's row says why on every row.
type csvCoverage struct {
	status, reason string
	withheld       bool
}

func csvCoverageOf(coverage crmcontracts.ReportingCoverage) csvCoverage {
	out := csvCoverage{status: string(coverage.Status), withheld: coverage.Withheld}
	if coverage.Reason != nil {
		out.reason = *coverage.Reason
	}
	return out
}

// as is the same coverage for a figure that carries its own status.
func (c csvCoverage) as(status string) csvCoverage {
	c.status = status
	return c
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

func writeReportingCSV(w http.ResponseWriter, r *http.Request, filename string, raw []byte, err error) {
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	(httperr.Download{ContentType: reportingCSVContentType, Filename: filename, Size: int64(len(raw))}).WriteHeaders(w)
	w.Header().Set("Cache-Control", "no-store")
	if _, err := w.Write(raw); err != nil { // #nosec G705 -- a CSV attachment, never rendered as a page
		slog.WarnContext(r.Context(), "reporting export connection closed", "error", err)
	}
}

const (
	reportingForecastBasisBlock    = "forecast_basis"
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
	return out
}

// forecastBasisCSV is the measure the landing used, the figures it rests on
// and the modelled range, each on its own row. A legacy reading without a
// basis has none of the basis rows, so no blank reads as a measure.
func forecastBasisCSV(metric crmcontracts.ReportingMetric) []comparisonFigure {
	out := []comparisonFigure{}
	add := func(label, context string, minor *int64) {
		figure := comparisonFigure{label: label, context: context}
		if minor != nil {
			figure.value = new(float64(*minor))
		}
		out = append(out, figure)
	}
	if basis := metric.ForecastBasis; basis != nil {
		landing := basis.Landing
		add("measure", string(landing.Measure), nil)
		add("won", reportingForecastBasisBlock, &landing.WonMinor)
		add("remaining", reportingForecastBasisBlock, &landing.RemainingMinor)
		if landing.SystemMinor != nil {
			add("records_projection", reportingForecastBasisBlock, landing.SystemMinor)
		}
		if basis.CallMinor != nil {
			add("manager_call", reportingForecastBasisBlock, basis.CallMinor)
		}
		if landing.Caveat != nil {
			add("caveat", string(*landing.Caveat), nil)
		}
	}
	if metric.RangeLowMinor != nil && metric.RangeHighMinor != nil {
		add("range_low", reportingForecastBasisBlock, metric.RangeLowMinor)
		add("range_high", reportingForecastBasisBlock, metric.RangeHighMinor)
	}
	return out
}
