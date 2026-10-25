// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The words a staged summary names a record's fields with.
//
// A withheld field reaches a human in the approval card, its email and the
// record's history panel. The history panel reads the frontend's
// history.field.* catalog. backend/gates/approvalfieldlabels_test.go holds each
// label here to that catalog where both name a field, so one field has one name.

import (
	"maps"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// fieldLabelsByLang names every field update_record may change, keyed by the
// contract's wire name. fieldlabels_test.go derives that field set from the
// contract and fails on a field with no label in some language.
//
//nolint:goconst,dupl // one row per wire field per language is the table's shape; the census holds the languages in step
var fieldLabelsByLang = map[textlang.Lang]map[string]string{
	textlang.English: {
		"acquisition_source":       "Acquisition source",
		"active":                   "Active",
		"address":                  "Address",
		"amount_minor":             "Value",
		"assignee_id":              "Assignee",
		"assess_against_entity_id": "Rated against",
		"billing_interval_months":  "Billing interval",
		"billing_model":            "Billing model",
		"body":                     "Notes",
		"buyer_company_id":         "Buyer",
		"candidate_company_key":    "Matched company",
		"origin":                   "Origin",
		"origin_detail":            "Origin note",
		"clear_ended_at":           "Ended",
		"clear_started_at":         "Started",
		"commercial_motion":        "Commercial motion",
		"company_id":               "Company",
		"company_name":             "Company name",
		"currency":                 "Currency",
		"default_tax_rate":         "Default tax rate",
		"description":              "Description",
		"display_name":             "Name",
		"domains":                  "Domains",
		"due_at":                   "Due",
		"email":                    "Email",
		"emails":                   "Email addresses",
		"employment_status":        "Employment status",
		"ended_at":                 "Ended",
		"ended_precision":          "End date precision",
		"expected_arr_minor":       "Expected ARR",
		"expected_close_date":      "Expected close",
		"first_name":               "First name",
		"forecast_category":        "Forecast category",
		"full_name":                "Name",
		"fx_rate_date":             "FX rate date",
		"fx_rate_to_base":          "FX rate",
		"industry":                 "Industry",
		"intro_text":               "Introduction",
		"is_current_primary":       "Primary employer",
		"is_default":               "Default",
		"is_done":                  "Done",
		"last_name":                "Last name",
		"layout":                   "Layout",
		"legal_name":               "Legal name",
		"lifecycle":                "Lifecycle",
		"linkedin_url":             "LinkedIn URL",
		"locale":                   "Language",
		"lost_reason":              "Lost reason",
		"meeting_status":           "Meeting outcome",
		"name":                     "Name",
		"occurred_at":              "Occurred",
		"occurred_precision":       "Time precision",
		"owner_id":                 "Owner",
		"relationship_mark":        "Relationship mark",
		"parent_company_id":        "Parent company",
		"partner_attribution":      "Partner attribution",
		"partner_company_id":       "Partner",
		"phone":                    "Phone",
		"phones":                   "Phone numbers",
		"priority":                 "Priority",
		"project_id":               "Project",
		"relationship_types":       "Relationship types",
		"remind_at":                "Reminder",
		"role":                     "Role",
		"score":                    "Score",
		"score_override_reason":    "Score override reason",
		"size_band":                "Employees",
		"sku":                      "SKU",
		"social":                   "Social profiles",
		"source":                   "Source",
		"started_at":               "Started",
		"started_precision":        "Start date precision",
		"status":                   "Status",
		"subject":                  "Subject",
		"target_end_date":          "Target end",
		"template_id":              "Template",
		"terms_text":               "Terms",
		"title":                    "Job title",
		"unit":                     "Unit",
		"unit_price_minor":         "Unit price",
		"valid_until":              "Valid until",
		"visibility":               "Visibility",
		"wait_until":               "Waiting until",
	},
	textlang.German: {
		"acquisition_source":       "Akquisitionsquelle",
		"active":                   "Aktiv",
		"address":                  "Adresse",
		"amount_minor":             "Wert",
		"assignee_id":              "Zugewiesen an",
		"assess_against_entity_id": "Bewertet gegen",
		"billing_interval_months":  "Abrechnungsintervall",
		"billing_model":            "Abrechnungsmodell",
		"body":                     "Notizen",
		"buyer_company_id":         "Käufer",
		"candidate_company_key":    "Zugeordnetes Unternehmen",
		"origin":                   "Herkunft",
		"origin_detail":            "Notiz zur Herkunft",
		"clear_ended_at":           "Beendet",
		"clear_started_at":         "Beginn",
		"commercial_motion":        "Deal-Typ",
		"company_id":               "Unternehmen",
		"company_name":             "Unternehmensname",
		"currency":                 "Währung",
		"default_tax_rate":         "Standardsteuersatz",
		"description":              "Beschreibung",
		"display_name":             "Name",
		"domains":                  "Domains",
		"due_at":                   "Fällig",
		"email":                    "E-Mail",
		"emails":                   "E-Mail-Adressen",
		"employment_status":        "Beschäftigungsstatus",
		"ended_at":                 "Beendet",
		"ended_precision":          "Genauigkeit des Enddatums",
		"expected_arr_minor":       "Erwarteter ARR",
		"expected_close_date":      "Voraussichtlicher Abschluss",
		"first_name":               "Vorname",
		"forecast_category":        "Prognosekategorie",
		"full_name":                "Name",
		"fx_rate_date":             "Datum des Wechselkurses",
		"fx_rate_to_base":          "Wechselkurs",
		"industry":                 "Branche",
		"intro_text":               "Einleitung",
		"is_current_primary":       "Hauptarbeitgeber",
		"is_default":               "Standard",
		"is_done":                  "Erledigt",
		"last_name":                "Nachname",
		"layout":                   "Layout",
		"legal_name":               "Rechtlicher Name",
		"lifecycle":                "Lebenszyklus",
		"linkedin_url":             "LinkedIn-URL",
		"locale":                   "Sprache",
		"lost_reason":              "Verlustgrund",
		"meeting_status":           "Ergebnis des Termins",
		"name":                     "Name",
		"occurred_at":              "Zeitpunkt",
		"occurred_precision":       "Zeitgenauigkeit",
		"owner_id":                 "Verantwortlich",
		"relationship_mark":        "Beziehungsmarkierung",
		"parent_company_id":        "Muttergesellschaft",
		"partner_attribution":      "Partnerzuordnung",
		"partner_company_id":       "Partner",
		"phone":                    "Telefonnummer",
		"phones":                   "Telefonnummern",
		"priority":                 "Priorität",
		"project_id":               "Projekt",
		"relationship_types":       "Beziehungstypen",
		"remind_at":                "Erinnerung",
		"role":                     "Rolle",
		"score":                    "Score",
		"score_override_reason":    "Grund für die Score-Anpassung",
		"size_band":                "Mitarbeitende",
		"sku":                      "Artikelnummer",
		"social":                   "Social-Profile",
		"source":                   "Quelle",
		"started_at":               "Beginn",
		"started_precision":        "Genauigkeit des Startdatums",
		"status":                   "Status",
		"subject":                  "Betreff",
		"target_end_date":          "Geplantes Ende",
		"template_id":              "Vorlage",
		"terms_text":               "Bedingungen",
		"title":                    "Jobtitel",
		"unit":                     "Einheit",
		"unit_price_minor":         "Stückpreis",
		"valid_until":              "Gültig bis",
		"visibility":               "Sichtbarkeit",
		"wait_until":               "Wartet bis",
	},
	textlang.Vietnamese: {
		"acquisition_source":       "Nguồn tiếp cận",
		"active":                   "Đang hoạt động",
		"address":                  "Địa chỉ",
		"amount_minor":             "Giá trị",
		"assignee_id":              "Giao cho",
		"assess_against_entity_id": "Đánh giá theo",
		"billing_interval_months":  "Chu kỳ thanh toán",
		"billing_model":            "Mô hình thanh toán",
		"body":                     "Ghi chú",
		"buyer_company_id":         "Bên mua",
		"candidate_company_key":    "Công ty khớp",
		"origin":                   "Nguồn gốc",
		"origin_detail":            "Ghi chú nguồn gốc",
		"clear_ended_at":           "Kết thúc",
		"clear_started_at":         "Bắt đầu",
		"commercial_motion":        "Hình thức bán hàng",
		"company_id":               "Công ty",
		"company_name":             "Tên công ty",
		"currency":                 "Loại tiền",
		"default_tax_rate":         "Thuế suất mặc định",
		"description":              "Mô tả",
		"display_name":             "Tên",
		"domains":                  "Tên miền",
		"due_at":                   "Hạn",
		"email":                    "Email",
		"emails":                   "Địa chỉ email",
		"employment_status":        "Tình trạng việc làm",
		"ended_at":                 "Kết thúc",
		"ended_precision":          "Độ chính xác ngày kết thúc",
		"expected_arr_minor":       "ARR dự kiến",
		"expected_close_date":      "Ngày chốt dự kiến",
		"first_name":               "Tên",
		"forecast_category":        "Danh mục dự báo",
		"full_name":                "Tên",
		"fx_rate_date":             "Ngày tỷ giá",
		"fx_rate_to_base":          "Tỷ giá",
		"industry":                 "Ngành",
		"intro_text":               "Lời mở đầu",
		"is_current_primary":       "Nơi làm việc chính",
		"is_default":               "Mặc định",
		"is_done":                  "Xong",
		"last_name":                "Họ",
		"layout":                   "Bố cục",
		"legal_name":               "Tên pháp lý",
		"lifecycle":                "Vòng đời",
		"linkedin_url":             "Liên kết LinkedIn",
		"locale":                   "Ngôn ngữ",
		"lost_reason":              "Lý do thua",
		"meeting_status":           "Kết quả cuộc họp",
		"name":                     "Tên",
		"occurred_at":              "Thời điểm xảy ra",
		"occurred_precision":       "Độ chính xác của thời gian",
		"owner_id":                 "Người chịu trách nhiệm",
		"relationship_mark":        "Nhãn quan hệ",
		"parent_company_id":        "Công ty mẹ",
		"partner_attribution":      "Ghi công đối tác",
		"partner_company_id":       "Đối tác",
		"phone":                    "Số điện thoại",
		"phones":                   "Số điện thoại",
		"priority":                 "Độ ưu tiên",
		"project_id":               "Dự án",
		"relationship_types":       "Loại quan hệ",
		"remind_at":                "Lời nhắc",
		"role":                     "Vai trò",
		"score":                    "Điểm",
		"score_override_reason":    "Lý do chỉnh điểm",
		"size_band":                "Số nhân viên",
		"sku":                      "Mã SKU",
		"social":                   "Hồ sơ mạng xã hội",
		"source":                   "Nguồn",
		"started_at":               "Bắt đầu",
		"started_precision":        "Độ chính xác ngày bắt đầu",
		"status":                   "Trạng thái",
		"subject":                  "Tiêu đề",
		"target_end_date":          "Ngày kết thúc mục tiêu",
		"template_id":              "Mẫu",
		"terms_text":               "Điều khoản",
		"title":                    "Chức danh",
		"unit":                     "Đơn vị",
		"unit_price_minor":         "Đơn giá",
		"valid_until":              "Có hiệu lực đến",
		"visibility":               "Phạm vi hiển thị",
		"wait_until":               "Chờ đến",
	},
}

// FieldLabel is one field as a reader of lang names it. A custom field reads as
// its slug, because its label lives in a catalog this module cannot read. A
// field the table lacks, such as one the server sets, reads as its wire name in
// words. The census keeps that from happening to a field the contract changes.
func FieldLabel(lang textlang.Lang, field string) string {
	labels, ok := fieldLabelsByLang[lang]
	if !ok {
		labels = fieldLabelsByLang[textlang.English]
	}
	if label, ok := labels[field]; ok {
		return label
	}
	words := strings.ReplaceAll(strings.TrimPrefix(field, datasource.CustomFieldPrefix), "_", " ")
	if words == "" {
		return field
	}
	return strings.ToUpper(words[:1]) + words[1:]
}

// FieldLabels joins fields as one list a reader of lang can read.
func FieldLabels(lang textlang.Lang, fields []string) string {
	labels := make([]string, len(fields))
	for i, field := range fields {
		labels[i] = FieldLabel(lang, field)
	}
	return strings.Join(labels, ", ")
}

// LabelledFields lists every field the table names, sorted, for the gate that
// holds these labels to the frontend's.
func LabelledFields() []string {
	return slices.Sorted(maps.Keys(fieldLabelsByLang[textlang.English]))
}
