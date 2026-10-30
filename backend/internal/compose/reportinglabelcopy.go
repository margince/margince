// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

var reportingLabelWords = struct{ OwnerMissing, Unassigned, ForecastContribution, PipelineContribution, Unmapped, Meeting, MetricRetired langcopy.Phrase }{
	PipelineContribution: langcopy.Phrase{En: "Captured pipeline contribution", De: "Erfasster Pipelinebeitrag", Vi: "Khoản đóng góp vào pipeline đã ghi nhận"},
	Unmapped:             langcopy.Phrase{En: "Unmapped", De: "Nicht zugeordnet", Vi: "Chưa ánh xạ"},
	Meeting:              langcopy.Phrase{En: "Meeting", De: "Besprechung", Vi: "Cuộc họp"},
	MetricRetired:        langcopy.Phrase{En: "Metric no longer available", De: "Kennzahl nicht mehr verfügbar", Vi: "Chỉ số không còn khả dụng"},
	OwnerMissing:         langcopy.Phrase{En: "Owner not recorded", De: "Verantwortliche Person nicht erfasst", Vi: "Chưa ghi nhận người chịu trách nhiệm"},
	Unassigned:           langcopy.Phrase{En: "Unassigned", De: "Nicht zugewiesen", Vi: "Chưa phân công"},
	ForecastContribution: langcopy.Phrase{En: "Forecast contribution", De: "Prognosebeitrag", Vi: "Khoản đóng góp vào dự báo"},
}

var reportingForecastLabels = map[string]langcopy.Phrase{
	"forecast_won":           {En: "Won", De: "Gewonnen", Vi: "Đã thắng"},
	"forecast_supported":     {En: "Supported open", De: "Belegte offene Deals", Vi: "Deal đang mở có bằng chứng hỗ trợ"},
	"forecast_upside":        {En: "Additional upside", De: "Zusätzliches Potenzial", Vi: "Tiềm năng tăng thêm"},
	"forecast_call":          {En: "Recorded forecast", De: "Erfasste Prognose", Vi: "Dự báo đã ghi nhận"},
	forecastOpenContext:      {En: "Open in the quarter", De: "Offene Deals im Quartal", Vi: "Deal đang mở trong quý"},
	forecastQualifiedContext: {En: "Qualified open in the quarter", De: "Qualifizierte offene Deals im Quartal", Vi: "Deal đủ điều kiện đang mở trong quý"},
	reportingForecastLanding: {En: "Expected landing", De: "Erwartetes Ergebnis", Vi: "Kết quả dự kiến"},
}

// reportingBreakdownLabels name the groups a lead or partner breakdown splits a
// figure into. A lead status reads as the lead page shows it.
var reportingBreakdownLabels = struct{ NoOrigin, Lead, PartnerHidden, Sourced, Influenced, Active, Inactive, CompanyAccess langcopy.Phrase }{
	NoOrigin:      langcopy.Phrase{En: "No origin", De: "Keine Herkunft", Vi: "Chưa có nguồn gốc"},
	Lead:          langcopy.Phrase{En: "Unnamed lead", De: "Lead ohne Namen", Vi: "Lead chưa có tên"},
	PartnerHidden: langcopy.Phrase{En: "Partner you cannot see", De: "Nicht sichtbares Partnerunternehmen", Vi: "Đối tác bạn không xem được"},
	Sourced:       langcopy.Phrase{En: "Sourced", De: "Vermittelt", Vi: "Mang deal về"},
	Influenced:    langcopy.Phrase{En: "Influenced", De: "Beeinflusst", Vi: "Hỗ trợ deal"},
	Active:        langcopy.Phrase{En: "Active", De: "Aktiv", Vi: "Đang hoạt động"},
	Inactive:      langcopy.Phrase{En: "Inactive", De: "Inaktiv", Vi: "Không hoạt động"},
	CompanyAccess: langcopy.Phrase{En: "Partners are counted only for readers who may see companies", De: "Partnerunternehmen werden nur gezählt, wenn du Unternehmen sehen darfst.", Vi: "Chỉ đếm đối tác khi bạn được phép xem công ty."},
}

var reportingLeadStatusLabels = map[crmcontracts.LeadStatus]langcopy.Phrase{
	crmcontracts.LeadStatusContacted:    {En: "Contacted", De: "Kontaktiert", Vi: "Đã liên hệ"},
	crmcontracts.LeadStatusEngaged:      {En: "Engaged", De: "Im Gespräch", Vi: "Đang trao đổi"},
	crmcontracts.LeadStatusPromoted:     {En: "Qualified", De: "Qualifiziert", Vi: "Đủ điều kiện"},
	crmcontracts.LeadStatusDisqualified: {En: "Disqualified", De: "Disqualifiziert", Vi: "Đã loại"},
}
