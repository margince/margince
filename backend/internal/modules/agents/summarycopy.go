// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The inbox lines a governed call writes when it stages for a human.
//
// An approval's summary is shared-record text: stored once and read by every
// seat, so it follows the installation's base language, resolved when the call
// is described. The same line reaches the calling agent in StageInfo, and the
// base language is right for that reader too. Record types, field names,
// addresses, subjects and labels are values, and pass through untranslated.

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/baselanguage"
)

// summaryCopy is one language's set. Every field is required, and the census
// (summarycopy_test.go) holds each one to the English sentence's formatting
// verbs, in the English order, because Go's verbs are positional.
type summaryCopy struct {
	// Record writes. Record types, labels, field names and ids pass through.
	archive, createHead, updateHead, logHead, settingFields, moreFields, overwriteHuman, merge,
	enrich, ownDomain string

	// Writes nested under a parent record, named by id.
	onDeal, applyTag, addLineItem, updateLineItem, removeLineItem, retireCustomField,
	customFieldOptions, setStakeholder, removeStakeholder, setCompany, removeCompany,
	confirmFact, updateFact, createFact, deleteFact, confirmProfileField, updateProfileField,
	mergeTags string

	// Lifecycle moves. A deal's target and source are stage semantics and pass
	// through.
	promoteLead, disqualifyLead, demoteLead, projectPhase, dealMove, dealMoveOpen, dealReopen,
	dealChange, dealClose string

	// Correspondence. Addresses, subjects and bodies pass through, quoted where
	// the sentence quotes them.
	sendEmail, cc, sendSubject, sendMessage, draftReply, accountSend, accountSendFiled, booking,
	bookingHost, bookingLinks, noSubject, relinkActivity, relinkThread, relinkActivities string

	// Imports. The counts are the report's; the record type passes through.
	importPreview, importCommit, importCreate, importUpdate, importUnchanged, importSkip,
	importIssues string

	// The agent's own work: decisions, reports, the morning brief and a volume
	// step-up.
	approveWord, rejectWord, decideApproval, decideBundle, runReport, composeReport,
	analyticsQuery, annotateNothing, annotateNarrative, annotateBoth, annotateFindings,
	stepUpRecords, stepUpChanges string
}

// summaryByLang is the census, keyed by textlang.Lang so the test can walk
// textlang.Shipped and ask this map directly.
//
//nolint:dupl // one block per language with the same keys is the table's shape; the census holds them in step
var summaryByLang = map[textlang.Lang]summaryCopy{
	textlang.English: {
		archive:             "Archive %s %s",
		createHead:          "Create a %s",
		updateHead:          "Update a %s",
		logHead:             "Log a %s",
		settingFields:       ", setting %s",
		moreFields:          "+%d more",
		overwriteHuman:      "Update %s %s: overwrite human-edited %s",
		merge:               "Merge %s %s into %s",
		enrich:              "Read %s from %s and propose enrichment of %s",
		ownDomain:           "its own domain",
		onDeal:              "%s on deal %s",
		applyTag:            "Apply tag %s",
		addLineItem:         "Add a line item to offer %s",
		updateLineItem:      "Update line item %s on offer %s",
		removeLineItem:      "Remove line item %s from offer %s",
		retireCustomField:   "Retire custom field %s",
		customFieldOptions:  "Update options for custom field %s",
		setStakeholder:      "Set a stakeholder on project %s",
		removeStakeholder:   "Remove stakeholder %s from project %s",
		setCompany:          "Put a company on project %s",
		removeCompany:       "Take company %s off project %s",
		confirmFact:         "Confirm fact %s on company %s",
		updateFact:          "Update fact %s on company %s",
		createFact:          "State a fact on company %s",
		deleteFact:          "Remove fact %s from company %s",
		confirmProfileField: "Confirm profile field %s on company %s",
		updateProfileField:  "Update profile field %s on company %s",
		mergeTags:           "Fold tag %q into %q, releasing the name %q",
		promoteLead:         "Promote lead %s to a contact (%s)",
		disqualifyLead:      "Disqualify lead %s",
		demoteLead:          "Reverse the promotion of lead %s",
		projectPhase:        "Move project %s to %s",
		dealMove:            "Move deal %s to %s",
		dealMoveOpen:        "Move deal %s to another open stage",
		dealReopen:          "REOPEN deal %s, which is currently %s — this clears its close date, its lost reason and the exchange rate frozen when it closed",
		dealChange:          "Change deal %s from %s to %s",
		dealClose:           "Close deal %s as %s",
		sendEmail:           "Send an email to %s",
		cc:                  ", cc %s",
		sendSubject:         ", subject %q",
		sendMessage:         "Reply on a captured conversation: %q",
		draftReply:          "Draft a reply to activity %s",
		accountSend:         "Start an email conversation with %s",
		accountSendFiled:    ", subject %q, filed under %d record(s)",
		booking:             "Book %q from %s to %s",
		bookingHost:         " on %s's calendar",
		bookingLinks:        ", attached to %d record(s)",
		noSubject:           "(no subject)",
		relinkActivity:      "Re-associate activity %s to %s %s",
		relinkThread:        "Re-associate the conversation %q to %s %s",
		relinkActivities:    "Re-associate %d activities to %s %s",
		importPreview:       "Check a file of %s records against this workspace, writing nothing",
		importCommit:        "Import %d rows as %s records: %s",
		importCreate:        "create %d",
		importUpdate:        "update %d",
		importUnchanged:     "leave %d unchanged",
		importSkip:          "skip %d",
		importIssues:        ". %d row(s) could not be used",
		approveWord:         "Approve",
		rejectWord:          "Reject",
		decideApproval:      "%s staged action %s",
		decideBundle:        "%s every waiting proposal of act %s",
		runReport:           "Run report %s",
		composeReport:       "Compose a report of %d block(s)",
		analyticsQuery:      "Run an analytics query over %s",
		annotateNothing:     "Record that tonight's pass ran and found nothing to say",
		annotateNarrative:   "Write a summary of the night onto your morning brief",
		annotateBoth:        "Write a summary of the night and %d finding(s) onto your morning brief",
		annotateFindings:    "Write %d finding(s) onto your morning brief",
		stepUpRecords:       "This agent has been handed %d records against a limit of %d for this window (most recently through %s). Approve to let it continue for another %d.",
		stepUpChanges:       "This agent has made %d changes against a limit of %d for this window (most recently through %s). Approve to let it continue for another %d.",
	},
	textlang.German: {
		archive:             "Datensatz vom Typ %s archivieren: %s",
		createHead:          "Datensatz vom Typ %s anlegen",
		updateHead:          "Datensatz vom Typ %s ändern",
		logHead:             "Datensatz vom Typ %s erfassen",
		settingFields:       ", Felder: %s",
		moreFields:          "+%d weitere",
		overwriteHuman:      "Datensatz vom Typ %s ändern: %s, überschreibt von Menschen bearbeitete Felder %s",
		merge:               "Datensatz vom Typ %s zusammenführen: %s in %s",
		enrich:              "%s von %s lesen und eine Anreicherung für %s vorschlagen",
		ownDomain:           "seiner eigenen Domain",
		onDeal:              "%s (Deal %s)",
		applyTag:            "Schlagwort %s anwenden",
		addLineItem:         "Position zum Angebot %s hinzufügen",
		updateLineItem:      "Position %s im Angebot %s ändern",
		removeLineItem:      "Position %s aus dem Angebot %s entfernen",
		retireCustomField:   "Benutzerdefiniertes Feld %s stilllegen",
		customFieldOptions:  "Optionen des benutzerdefinierten Felds %s ändern",
		setStakeholder:      "Stakeholder für Projekt %s festlegen",
		removeStakeholder:   "Stakeholder %s aus Projekt %s entfernen",
		setCompany:          "Projekt %s ein Unternehmen zuordnen",
		removeCompany:       "Unternehmen %s von Projekt %s lösen",
		confirmFact:         "Fakt %s zum Unternehmen %s bestätigen",
		updateFact:          "Fakt %s zum Unternehmen %s ändern",
		createFact:          "Fakt zum Unternehmen %s festhalten",
		deleteFact:          "Fakt %s vom Unternehmen %s entfernen",
		confirmProfileField: "Profilfeld %s zum Unternehmen %s bestätigen",
		updateProfileField:  "Profilfeld %s zum Unternehmen %s ändern",
		mergeTags:           "Schlagwort %q in %q überführen und den Namen %q freigeben",
		promoteLead:         "Lead %s zum Kontakt überführen (%s)",
		disqualifyLead:      "Lead %s disqualifizieren",
		demoteLead:          "Überführung von Lead %s rückgängig machen",
		projectPhase:        "Projekt %s in die Phase %s bringen",
		dealMove:            "Deal %s nach %s verschieben",
		dealMoveOpen:        "Deal %s in eine andere offene Phase verschieben",
		dealReopen:          "Deal %s WIEDER ÖFFNEN, aktuell %s. Dabei werden Abschlussdatum, Verlustgrund und der beim Abschluss eingefrorene Wechselkurs gelöscht.",
		dealChange:          "Deal %s von %s auf %s ändern",
		dealClose:           "Deal %s als %s abschließen",
		sendEmail:           "E-Mail senden an %s",
		cc:                  ", Cc: %s",
		sendSubject:         ", Betreff %q",
		sendMessage:         "In einer erfassten Unterhaltung antworten: %q",
		draftReply:          "Antwort auf Aktivität %s entwerfen",
		accountSend:         "E-Mail-Unterhaltung beginnen mit %s",
		accountSendFiled:    ", Betreff %q, zugeordnete Datensätze: %d",
		booking:             "Termin buchen: %q von %s bis %s",
		bookingHost:         " im Kalender von %s",
		bookingLinks:        ", verknüpfte Datensätze: %d",
		noSubject:           "(kein Betreff)",
		relinkActivity:      "Aktivität %s neu zuordnen: %s %s",
		relinkThread:        "Unterhaltung %q neu zuordnen: %s %s",
		relinkActivities:    "%d Aktivitäten neu zuordnen: %s %s",
		importPreview:       "Datei mit Datensätzen vom Typ %s mit diesem Workspace abgleichen, ohne etwas zu schreiben",
		importCommit:        "%d Zeilen als Datensätze vom Typ %s importieren: %s",
		importCreate:        "%d anlegen",
		importUpdate:        "%d ändern",
		importUnchanged:     "%d unverändert lassen",
		importSkip:          "%d überspringen",
		importIssues:        ". %d Zeile(n) konnten nicht verwendet werden",
		approveWord:         "Freigeben",
		rejectWord:          "Ablehnen",
		decideApproval:      "%s: vorgemerkte Aktion %s",
		decideBundle:        "%s: alle wartenden Vorschläge des Vorgangs %s",
		runReport:           "Bericht %s ausführen",
		composeReport:       "Bericht aus %d Baustein(en) erstellen",
		analyticsQuery:      "Analyseabfrage über %s ausführen",
		annotateNothing:     "Festhalten, dass der nächtliche Durchlauf lief und nichts zu berichten hatte",
		annotateNarrative:   "Eine Zusammenfassung der Nacht in deinen Morgenbericht schreiben",
		annotateBoth:        "Eine Zusammenfassung der Nacht und %d Erkenntnis(se) in deinen Morgenbericht schreiben",
		annotateFindings:    "%d Erkenntnis(se) in deinen Morgenbericht schreiben",
		stepUpRecords:       "Dieser Agent hat %d Datensätze erhalten, bei einem Limit von %d für dieses Zeitfenster (zuletzt über %s). Gib frei, damit er für weitere %d weitermachen kann.",
		stepUpChanges:       "Dieser Agent hat %d Änderungen vorgenommen, bei einem Limit von %d für dieses Zeitfenster (zuletzt über %s). Gib frei, damit er für weitere %d weitermachen kann.",
	},
	textlang.Vietnamese: {
		archive:             "Lưu trữ bản ghi loại %s: %s",
		createHead:          "Tạo bản ghi loại %s",
		updateHead:          "Cập nhật bản ghi loại %s",
		logHead:             "Ghi nhận bản ghi loại %s",
		settingFields:       ", các trường: %s",
		moreFields:          "+%d trường khác",
		overwriteHuman:      "Cập nhật bản ghi loại %s %s: ghi đè các trường do người dùng chỉnh sửa %s",
		merge:               "Hợp nhất bản ghi loại %s: %s vào %s",
		enrich:              "Đọc %s từ %s và đề xuất làm giàu dữ liệu cho %s",
		ownDomain:           "tên miền của chính công ty",
		onDeal:              "%s (deal %s)",
		applyTag:            "Gắn thẻ %s",
		addLineItem:         "Thêm một dòng mục vào báo giá %s",
		updateLineItem:      "Cập nhật dòng mục %s trong báo giá %s",
		removeLineItem:      "Xóa dòng mục %s khỏi báo giá %s",
		retireCustomField:   "Ngừng sử dụng trường tùy chỉnh %s",
		customFieldOptions:  "Cập nhật tùy chọn của trường tùy chỉnh %s",
		setStakeholder:      "Đặt một bên liên quan cho dự án %s",
		removeStakeholder:   "Gỡ bên liên quan %s khỏi dự án %s",
		setCompany:          "Gán một công ty cho dự án %s",
		removeCompany:       "Gỡ công ty %s khỏi dự án %s",
		confirmFact:         "Xác nhận dữ kiện %s của công ty %s",
		updateFact:          "Cập nhật dữ kiện %s của công ty %s",
		createFact:          "Nêu một dữ kiện về công ty %s",
		deleteFact:          "Xóa dữ kiện %s khỏi công ty %s",
		confirmProfileField: "Xác nhận trường hồ sơ %s của công ty %s",
		updateProfileField:  "Cập nhật trường hồ sơ %s của công ty %s",
		mergeTags:           "Gộp thẻ %q vào %q, giải phóng tên %q",
		promoteLead:         "Chuyển lead %s thành liên hệ (%s)",
		disqualifyLead:      "Loại lead %s",
		demoteLead:          "Hoàn tác việc chuyển đổi lead %s",
		projectPhase:        "Chuyển dự án %s sang %s",
		dealMove:            "Chuyển deal %s sang %s",
		dealMoveOpen:        "Chuyển deal %s sang một giai đoạn mở khác",
		dealReopen:          "MỞ LẠI deal %s, hiện đang là %s; thao tác này xóa ngày chốt, lý do thua và tỷ giá đã cố định khi deal được chốt",
		dealChange:          "Đổi deal %s từ %s sang %s",
		dealClose:           "Chốt deal %s là %s",
		sendEmail:           "Gửi email tới %s",
		cc:                  ", Cc: %s",
		sendSubject:         ", tiêu đề %q",
		sendMessage:         "Trả lời trong một cuộc trò chuyện đã ghi nhận: %q",
		draftReply:          "Soạn thư trả lời cho hoạt động %s",
		accountSend:         "Bắt đầu cuộc trò chuyện email với %s",
		accountSendFiled:    ", tiêu đề %q, lưu vào %d bản ghi",
		booking:             "Đặt lịch %q từ %s đến %s",
		bookingHost:         " trên lịch của %s",
		bookingLinks:        ", gắn với %d bản ghi",
		noSubject:           "(không có tiêu đề)",
		relinkActivity:      "Liên kết lại hoạt động %s với %s %s",
		relinkThread:        "Liên kết lại cuộc trò chuyện %q với %s %s",
		relinkActivities:    "Liên kết lại %d hoạt động với %s %s",
		importPreview:       "Kiểm tra một tệp bản ghi loại %s với không gian làm việc này, không ghi gì cả",
		importCommit:        "Nhập %d dòng thành bản ghi loại %s: %s",
		importCreate:        "tạo %d",
		importUpdate:        "cập nhật %d",
		importUnchanged:     "giữ nguyên %d",
		importSkip:          "bỏ qua %d",
		importIssues:        ". %d dòng không thể sử dụng",
		approveWord:         "Phê duyệt",
		rejectWord:          "Từ chối",
		decideApproval:      "%s hành động đang chờ %s",
		decideBundle:        "%s mọi đề xuất đang chờ của lượt %s",
		runReport:           "Chạy báo cáo %s",
		composeReport:       "Soạn một báo cáo gồm %d khối",
		analyticsQuery:      "Chạy truy vấn phân tích trên %s",
		annotateNothing:     "Ghi nhận rằng lượt chạy đêm nay đã chạy và không có gì để báo cáo",
		annotateNarrative:   "Viết bản tóm tắt đêm qua vào bản tin buổi sáng của bạn",
		annotateBoth:        "Viết bản tóm tắt đêm qua và %d phát hiện vào bản tin buổi sáng của bạn",
		annotateFindings:    "Viết %d phát hiện vào bản tin buổi sáng của bạn",
		stepUpRecords:       "Agent này đã nhận %d bản ghi so với giới hạn %d cho khung thời gian này (gần nhất qua %s). Phê duyệt để cho phép tiếp tục thêm %d.",
		stepUpChanges:       "Agent này đã thực hiện %d thay đổi so với giới hạn %d cho khung thời gian này (gần nhất qua %s). Phê duyệt để cho phép tiếp tục thêm %d.",
	},
}

// summaryIn answers the set for the installation's base language, and English
// for a language this table has not learned: the language comes off a settings
// row, and a sentence in English beats no sentence.
func summaryIn(ctx context.Context, language baselanguage.Resolver) summaryCopy {
	if said, ok := summaryByLang[language.Resolve(ctx)]; ok {
		return said
	}
	return summaryByLang[textlang.English]
}

// WithBaseLanguage injects the installation's base language every staged
// summary on this surface is written in. A registry composed without one
// writes English.
func WithBaseLanguage(language baselanguage.Resolver) RegistryOption {
	return func(r *Registry) { r.language = language }
}

// BaseLanguage answers the resolver this surface was composed with, so the
// REST door describes a call in the same language the tool door does.
func (r *Registry) BaseLanguage() baselanguage.Resolver {
	return r.language
}
