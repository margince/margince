// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"time"

	"github.com/margince/margince/backend/internal/compose/salesassessment"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

var assessmentForecastLabels = map[string]langcopy.Phrase{
	"range_low":       {En: "Modelled quarter range, low", De: "Modellierte Quartalsspanne, unten", Vi: "Khoảng dự báo quý, thấp"},
	"range_high":      {En: "Modelled quarter range, high", De: "Modellierte Quartalsspanne, oben", Vi: "Khoảng dự báo quý, cao"},
	"target_status":   {En: "Target comparison", De: "Zielvergleich", Vi: "So sánh mục tiêu"},
	"target":          {En: "Sales target", De: "Umsatzziel", Vi: "Mục tiêu doanh số"},
	"remaining":       {En: "Sales still needed to reach target", De: "Noch fehlender Umsatz zum Ziel", Vi: "Doanh số còn thiếu để đạt mục tiêu"},
	"gap":             {En: "Target less forecast total", De: "Ziel abzüglich Prognose", Vi: "Mục tiêu trừ dự báo"},
	"period_pipeline": {En: "Open pipeline eligible to close this quarter", De: "Offene Pipeline mit Abschluss in diesem Quartal", Vi: "Pipeline mở dự kiến chốt trong quý"},
	"basis":           {En: "Forecast total basis", De: "Grundlage der Prognose", Vi: "Cơ sở dự báo"},
	"call":            {En: "Manager forecast", De: "Managementprognose", Vi: "Dự báo của quản lý"},
	"eligible":        {En: "Deals eligible this quarter", De: "Für dieses Quartal berücksichtigte Deals", Vi: "Cơ hội trong quý"},
	"included":        {En: "Deals included in totals", De: "In Summen enthaltene Deals", Vi: "Cơ hội được tính vào tổng"},
	"fx_missing":      {En: "Deals missing an exchange rate", De: "Deals ohne Wechselkurs", Vi: "Cơ hội thiếu tỷ giá"},
}

func assessmentForecastCells(evaluation crmcontracts.ReportingEvaluation, zone *time.Location) []salesassessment.CellIn {
	cells := []salesassessment.CellIn{}
	for _, metric := range evaluation.Metrics {
		if metric.Id != reportingForecastLanding || metric.Value == nil || metric.Coverage.Withheld {
			continue
		}
		cells = append(cells, assessmentForecastMetricCells(evaluation, metric, zone)...)
	}
	return cells
}

func assessmentForecastMetricCells(evaluation crmcontracts.ReportingEvaluation, metric crmcontracts.ReportingMetric, zone *time.Location) []salesassessment.CellIn {
	cells := []salesassessment.CellIn{}
	words := reporting.ExplanationCopy(evaluation.Context)
	label := func(key string) string { return words.Say(assessmentForecastLabels[key]) }
	window := evaluation.Context.Interval
	if metric.ForecastBasis != nil {
		window = metric.ForecastBasis.Period
	}
	add := func(id string, value *float64) {
		if value != nil {
			cells = append(cells, assessmentCell("forecast:"+id, label(id), value, evaluation.Context.Currency, assessmentWindow(window, zone), metric.Coverage))
		}
	}
	if metric.RangeLowMinor != nil {
		value := float64(*metric.RangeLowMinor)
		add("range_low", &value)
	}
	if metric.RangeHighMinor != nil {
		value := float64(*metric.RangeHighMinor)
		add("range_high", &value)
	}
	if comparison := metric.TargetComparison; comparison != nil {
		cells = append(cells, salesassessment.CellIn{ID: "forecast:target_status", Label: label("target_status"), Value: string(comparison.Status), Coverage: "ok"})
		if comparison.Status == "compared" || comparison.Status == "target_reached" {
			add("target", comparison.Target)
			add("remaining", comparison.Remaining)
			add("gap", comparison.Gap)
		}
	}
	if basis := metric.ForecastBasis; basis != nil {
		cells = append(cells, salesassessment.CellIn{ID: "forecast:basis", Label: label("basis"), Value: string(basis.Landing.Measure), Coverage: "ok"})
		if basis.OpenPipelineMinor != nil {
			v := float64(*basis.OpenPipelineMinor)
			add("period_pipeline", &v)
		}
		if basis.CallMinor != nil {
			v := float64(*basis.CallMinor)
			add("call", &v)
		}
		for _, count := range []struct {
			key   string
			value *int64
		}{{"eligible", &basis.EligibleCount}, {"included", basis.IncludedCount}, {"fx_missing", &basis.FxMissingCount}} {
			if count.value != nil {
				v := float64(*count.value)
				cells = append(cells, assessmentCell("forecast:"+count.key, label(count.key), &v, "count", assessmentWindow(window, zone), metric.Coverage))
			}
		}
	}
	return cells
}
