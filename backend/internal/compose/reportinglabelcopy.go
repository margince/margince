// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import "github.com/margince/margince/backend/internal/shared/kernel/langcopy"

var reportingForecastLabels = map[string]langcopy.Phrase{
	"forecast_won":           {En: "Won", De: "Gewonnen", Vi: "Đã thắng"},
	"forecast_supported":     {En: "Supported open", De: "Belegte offene Deals", Vi: "Deal đang mở có bằng chứng hỗ trợ"},
	"forecast_upside":        {En: "Additional upside", De: "Zusätzliches Potenzial", Vi: "Tiềm năng tăng thêm"},
	"forecast_call":          {En: "Recorded forecast", De: "Erfasste Prognose", Vi: "Dự báo đã ghi nhận"},
	forecastOpenContext:      {En: "Open in the quarter", De: "Offene Deals im Quartal", Vi: "Deal đang mở trong quý"},
	reportingForecastLanding: {En: "Expected landing", De: "Erwartetes Ergebnis", Vi: "Kết quả dự kiến"},
	forecastRangeLowContext:  {En: "Low end of 8 in 10 outcomes", De: "Untere Grenze von 8 aus 10 Ergebnissen", Vi: "Mức thấp của 8 trên 10 kết quả"},
	forecastRangeHighContext: {En: "High end of 8 in 10 outcomes", De: "Obere Grenze von 8 aus 10 Ergebnissen", Vi: "Mức cao của 8 trên 10 kết quả"},
}
