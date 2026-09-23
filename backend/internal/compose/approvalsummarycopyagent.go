// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The words the REST gate's summary names an agent's act with, per language.
//
// English needs no vocabulary of its own: its headline is the contract's tool
// verb read aloud ("update record" + "deal"), which is how the English cards
// have always read. Every other language carries a word for each verb a staged
// call can name, keyed by the same wire identifiers, and the census derives that
// set from agentPolicies — a contract change that adds a stageable verb fails a
// test before it reaches a German card as English. Record nouns are the tool
// door's own (agents.RecordNoun), so both doors name a record type alike.

// agentActVocabulary is one language's words for an agent's act. A lookup that
// misses falls back to the English reading of the wire verb.
type agentActVocabulary struct {
	// verbs is the whole headline for a tool that names its own object.
	verbs map[string]string
	// recordFrames is the headline for a generic verb, with one %s for the
	// record noun.
	recordFrames map[string]string
	// operations names the acts whose verb alone cannot tell them apart; see
	// opPhrases, which is the English set.
	operations map[string]string
}

var agentActsEnglish = agentActVocabulary{operations: opPhrases}

//nolint:goconst // the keys are the contract's own tool verbs, and a constant per verb would be a second spelling of the generated policy table.
var agentActsGerman = agentActVocabulary{
	verbs: map[string]string{
		"advance_deal":             "Deal weiterbringen",
		"advance_project_phase":    "Projekt in die nächste Phase bringen",
		"annotate_brief":           "Anmerkung zum Morgenbericht hinzufügen",
		"apply_tag":                "Schlagwort vergeben",
		"book_meeting":             "Termin buchen",
		"commit_import":            "Import übernehmen",
		"compose_analytics_report": "Analysebericht erstellen",
		"create_tag":               "Schlagwort anlegen",
		"create_task":              "Aufgabe anlegen",
		"decide_approval":          "Über eine Freigabe entscheiden",
		"decide_approval_bundle":   "Über ein Freigabebündel entscheiden",
		"demote_lead":              "Lead-Überführung rückgängig machen",
		"disqualify_lead":          "Lead disqualifizieren",
		"draft_email":              "E-Mail entwerfen",
		"enrich":                   "Aus dem Web anreichern",
		"log_activity":             "Aktivität erfassen",
		"merge_tags":               "Ein Schlagwort in ein anderes überführen",
		"preview_import":           "Import-Vorschau erstellen",
		"promote_lead":             "Lead überführen",
		"relink_activities":        "Mehrere Aktivitäten neu zuordnen",
		"relink_activity":          "Aktivität neu zuordnen",
		"relink_thread":            "Unterhaltung neu zuordnen",
		"remove_tag":               "Schlagwort entfernen",
		"run_analytics_query":      "Analyseabfrage ausführen",
		"run_report":               "Bericht ausführen",
		"send_company_email":       "E-Mail an ein Unternehmen senden",
		"send_email":               "E-Mail senden",
		"send_message":             "Nachricht senden",
		"update_tag":               "Schlagwort ändern",
	},
	recordFrames: map[string]string{
		toolCreateRecord:  "%s anlegen",
		toolUpdateRecord:  "%s ändern",
		toolArchiveRecord: "%s archivieren",
		toolMergeRecords:  "Datensätze zusammenführen: %s",
	},

	operations: map[string]string{
		opScrapeCompany:            "Website dieses Unternehmens lesen",
		opDeepReadCompany:          "Gesamte Website dieses Unternehmens lesen",
		opTechnicalEnrichCompany:   "Nachsehen, welche Technik dieses Unternehmen öffentlich einsetzt",
		opRetireCustomField:        "Benutzerdefiniertes Feld stilllegen",
		opUpdateCustomFieldOptions: "Optionen eines benutzerdefinierten Felds ändern",
		opMergeTags:                "Ein Schlagwort in ein anderes überführen und seinen Namen freigeben",
	},
}

var agentActsVietnamese = agentActVocabulary{
	verbs: map[string]string{
		"advance_deal":             "Chuyển giai đoạn của deal",
		"advance_project_phase":    "Chuyển dự án sang giai đoạn kế tiếp",
		"annotate_brief":           "Thêm ghi chú vào bản tóm tắt",
		"apply_tag":                "Gắn thẻ",
		"book_meeting":             "Đặt một lịch họp",
		"commit_import":            "Xác nhận nhập dữ liệu",
		"compose_analytics_report": "Soạn báo cáo phân tích",
		"create_tag":               "Tạo thẻ",
		"create_task":              "Tạo công việc",
		"decide_approval":          "Quyết định một yêu cầu phê duyệt",
		"decide_approval_bundle":   "Quyết định một nhóm yêu cầu phê duyệt",
		"demote_lead":              "Hoàn tác chuyển đổi lead",
		"disqualify_lead":          "Loại một lead",
		"draft_email":              "Soạn một email",
		"enrich":                   "Bổ sung thông tin từ web",
		"log_activity":             "Ghi lại một hoạt động",
		"merge_tags":               "Gộp một thẻ vào thẻ khác",
		"preview_import":           "Xem trước dữ liệu nhập",
		"promote_lead":             "Chuyển đổi một lead",
		"relink_activities":        "Gán lại nhiều hoạt động",
		"relink_activity":          "Gán lại một hoạt động",
		"relink_thread":            "Gán lại một cuộc trò chuyện",
		"remove_tag":               "Gỡ thẻ",
		"run_analytics_query":      "Chạy truy vấn phân tích",
		"run_report":               "Chạy báo cáo",
		"send_company_email":       "Gửi email cho một công ty",
		"send_email":               "Gửi một email",
		"send_message":             "Gửi một tin nhắn",
		"update_tag":               "Cập nhật thẻ",
	},
	recordFrames: map[string]string{
		toolCreateRecord:  "Tạo %s",
		toolUpdateRecord:  "Cập nhật %s",
		toolArchiveRecord: "Lưu trữ %s",
		toolMergeRecords:  "Gộp bản ghi: %s",
	},

	operations: map[string]string{
		opScrapeCompany:            "Đọc website của công ty này",
		opDeepReadCompany:          "Đọc toàn bộ website của công ty này",
		opTechnicalEnrichCompany:   "Tra cứu những gì công ty này vận hành công khai",
		opRetireCustomField:        "Ngừng sử dụng một trường tùy chỉnh",
		opUpdateCustomFieldOptions: "Thay đổi các tùy chọn của một trường tùy chỉnh",
		opMergeTags:                "Gộp một thẻ vào thẻ khác và giải phóng tên của nó",
	},
}
