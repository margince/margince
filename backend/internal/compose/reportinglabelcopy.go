// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import "github.com/margince/margince/backend/internal/shared/kernel/langcopy"

var reportingLabelWords = struct{ OwnerMissing, Unassigned, ForecastContribution, PipelineContribution, Unmapped, Meeting langcopy.Phrase }{
	PipelineContribution: langcopy.Phrase{En: "Captured pipeline contribution", De: "Erfasster Pipelinebeitrag", Vi: "Khoản đóng góp vào pipeline đã ghi nhận"},
	Unmapped:             langcopy.Phrase{En: "Unmapped", De: "Nicht zugeordnet", Vi: "Chưa ánh xạ"},
	Meeting:              langcopy.Phrase{En: "Meeting", De: "Besprechung", Vi: "Cuộc họp"},
	OwnerMissing:         langcopy.Phrase{En: "Owner not recorded", De: "Verantwortliche Person nicht erfasst", Vi: "Chưa ghi nhận người chịu trách nhiệm"},
	Unassigned:           langcopy.Phrase{En: "Unassigned", De: "Nicht zugewiesen", Vi: "Chưa phân công"},
	ForecastContribution: langcopy.Phrase{En: "Forecast contribution", De: "Prognosebeitrag", Vi: "Khoản đóng góp vào dự báo"},
}

var reportingForecastLabels = map[string]langcopy.Phrase{
	"forecast_won":           {En: "Won", De: "Gewonnen", Vi: "Đã thắng"},
	"forecast_supported":     {En: "Supported open", De: "Belegte offene Deals", Vi: "Deal đang mở có bằng chứng hỗ trợ"},
	"forecast_upside":        {En: "Additional upside", De: "Zusätzliches Potenzial", Vi: "Tiềm năng tăng thêm"},
	"forecast_call":          {En: "Recorded forecast", De: "Erfasste Prognose", Vi: "Dự báo đã ghi nhận"},
	reportingForecastLanding: {En: "Expected landing", De: "Erwartetes Ergebnis", Vi: "Kết quả dự kiến"},
}
