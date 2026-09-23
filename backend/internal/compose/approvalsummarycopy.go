// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The sentences this product writes onto an approval for a human to decide.
//
// approval.summary is shared-record text: stored once, read by every seat that
// can see the card. So it follows the installation's base language, resolved
// when the card is staged, exactly as a signal summary does (signalsummarycopy.go)
// and for the same reason — a sentence in one hardcoded language reads correctly
// only on the installations that happened to choose it. A card already stored
// keeps the words it was written in.
//
// Only product-written copy lives here. A summary a model wrote is governed by
// promptlang.Rule; a subject line, a headline, a name or a URL is quoted from
// its source and passes through every language untouched.

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// approvalSummaryCopy is one language's set. Every string field is required,
// and the census walks the struct itself, so a field added here is covered
// without anybody remembering to list it.
type approvalSummaryCopy struct {
	// draftedReplyWaiting: a follow-up reply is drafted. %q is its subject.
	draftedReplyWaiting string
	// counterpartyWorthKeeping asks about an unresolved sender. %s is the address.
	counterpartyWorthKeeping string
	// companyRename: %s is the current name, then the proposed one.
	companyRename string
	// siteLeadFound: %s is the site's host, then the contact's name, then role.
	siteLeadFound string
	// enrichmentOf: %s is the URL the enrichment reads.
	enrichmentOf string
	// contractEndedMove: %s is the account's current stage, then the proposed one.
	contractEndedMove string
	// overwriteHumanEdited: %s is the withheld fields, then the call's summary.
	overwriteHumanEdited string
	// linkedInLooksLike: %s is the connection, their employer, then the contact.
	linkedInLooksLike string
	// vcardResemblesContact: %s is the card's name, after the qualifier.
	vcardResemblesContact string
	// unnamedEmployer stands in for an employer the export left blank, inside
	// linkedInLooksLike's second hole.
	unnamedEmployer string

	// heldNotSent: %q is the message's subject, %s one of the held* reasons.
	heldNotSent          string
	heldConsentWithdrawn string
	heldSenderInactive   string
	heldPassportRevoked  string
	heldMissedWindow     string
	heldTimerExhausted   string
	heldSendRefused      string

	// reviewSendRefused: %s is reviewOneRecipient or reviewRecipients, then the
	// refusal's reason code, which is an identifier and stays untranslated.
	reviewSendRefused  string
	reviewOneRecipient string
	reviewRecipients   string

	// The cold-start read-back names its source; %s is the URL.
	coldStartFromURL             string
	coldStartFromText            string
	coldStartFromSelfDescription string

	// fxRateChanged: %s is the currency, the base, the new rate, then the prior
	// rate or fxNoRateInForce.
	fxRateChanged   string
	fxNoRateInForce string
	// modelRateChanged: %s is the provider, the model, the new input price, then
	// the prior one or modelRateNew.
	modelRateChanged string
	modelRateNew     string

	// The REST gate's structural summary: the pseudo-field naming a nested
	// create's parent, and the count that stands in for a nested object. The
	// count of fields left out is the tool door's (agents.MoreFields).
	createdUnder string
	nestedFields string

	// acts names what an agent's staged call does (approvalsummarycopyagent.go).
	acts agentActVocabulary
	// lang is the set's own language, stamped by approvalSummaryCopyFor for the
	// words this set borrows from the tool door.
	lang textlang.Lang
}

// approvalSummaryByLang is the census, keyed by textlang.Lang so the test can
// walk textlang.Shipped and ask this map directly.
var approvalSummaryByLang = map[textlang.Lang]approvalSummaryCopy{
	textlang.English: {
		draftedReplyWaiting: "A reply to %q is drafted and waiting to be sent — " +
			"the conversation left no next step planned",
		counterpartyWorthKeeping: "Is %s a contact worth keeping?",
		companyRename:            "Rename %s to %s?",
		siteLeadFound:            "Found on %s: %s — %s",
		enrichmentOf:             "Enrichment of %s",
		contractEndedMove:        "Their mail says the contract ended. Move this account from %s to %s?",
		overwriteHumanEdited:     "overwrite human-edited %s — %s",
		linkedInLooksLike:        "%s at %s looks like %s",
		unnamedEmployer:          "an unnamed employer",
		vcardResemblesContact:    "This card resembles an existing contact. Create %s anyway?",

		heldNotSent:          "%q was not sent: %s",
		heldConsentWithdrawn: "a recipient withdrew consent for this purpose after it was scheduled",
		heldSenderInactive:   "the sending account or its mailbox is no longer active",
		heldPassportRevoked: "the agent credential it was scheduled under has been revoked or expired — " +
			"your account is fine, so send it yourself if it should still go",
		heldMissedWindow:   "its moment passed while nothing was running, and it is too late to be the message that was written",
		heldTimerExhausted: "the job that wakes it ran out of attempts",
		heldSendRefused:    "a gate refused it at send time",

		reviewSendRefused:  "A send was refused for %s (%s) and somebody is asking whether it may go anyway",
		reviewOneRecipient: "1 recipient",
		reviewRecipients:   "%d recipients",

		coldStartFromURL:             "Cold-start read-back of %s",
		coldStartFromText:            "Cold-start read-back of pasted text",
		coldStartFromSelfDescription: "Cold-start read-back of a self-description",

		fxRateChanged:    "%s → %s %s (was %s)",
		fxNoRateInForce:  "none in force today",
		modelRateChanged: "%s/%s input %s (was %s)",
		modelRateNew:     "(new)",

		createdUnder: "under",
		nestedFields: "{%d fields}",

		acts: agentActsEnglish,
	},
	textlang.German: {
		draftedReplyWaiting: "Eine Antwort auf %q ist entworfen und wartet auf den Versand: " +
			"Das Gespräch endete ohne geplanten nächsten Schritt",
		counterpartyWorthKeeping: "Lohnt es sich, %s als Kontakt zu behalten?",
		companyRename:            "%s in %s umbenennen?",
		siteLeadFound:            "Gefunden auf %s: %s (%s)",
		enrichmentOf:             "Anreicherung aus %s",
		contractEndedMove:        "Laut ihrer E-Mail ist der Vertrag beendet. Diesen Account von %s nach %s verschieben?",
		overwriteHumanEdited:     "Manuell bearbeitete Felder überschreiben (%s): %s",
		linkedInLooksLike:        "%s bei %s scheint %s zu sein",
		unnamedEmployer:          "einem nicht genannten Arbeitgeber",
		vcardResemblesContact:    "Diese Visitenkarte ähnelt einem bestehenden Kontakt. %s trotzdem anlegen?",

		heldNotSent:          "%q wurde nicht gesendet: %s",
		heldConsentWithdrawn: "ein Empfänger hat seine Einwilligung für diesen Zweck nach der Planung widerrufen",
		heldSenderInactive:   "das sendende Konto oder sein Postfach ist nicht mehr aktiv",
		heldPassportRevoked: "die Agent-Zugangsdaten, unter denen die Nachricht geplant wurde, wurden widerrufen oder sind abgelaufen. " +
			"Dein Konto ist in Ordnung, sende sie also selbst, wenn sie noch rausgehen soll",
		heldMissedWindow: "der geplante Zeitpunkt ist verstrichen, während nichts lief, " +
			"und für die Nachricht, wie sie geschrieben wurde, ist es jetzt zu spät",
		heldTimerExhausted: "der Job, der den Versand auslöst, hat alle Versuche aufgebraucht",
		heldSendRefused:    "eine Prüfung hat den Versand zum Sendezeitpunkt abgelehnt",

		reviewSendRefused:  "Ein Versand an %s wurde abgelehnt (%s), und jemand fragt, ob er trotzdem rausgehen darf",
		reviewOneRecipient: "1 Empfänger",
		reviewRecipients:   "%d Empfänger",

		coldStartFromURL:             "Cold-Start-Auslesung aus %s",
		coldStartFromText:            "Cold-Start-Auslesung aus eingefügtem Text",
		coldStartFromSelfDescription: "Cold-Start-Auslesung aus einer Selbstbeschreibung",

		fxRateChanged:    "%s → %s %s (bisher %s)",
		fxNoRateInForce:  "kein heute gültiger Kurs",
		modelRateChanged: "%s/%s Input %s (bisher %s)",
		modelRateNew:     "(neu)",

		createdUnder: "unter",
		nestedFields: "{%d Felder}",

		acts: agentActsGerman,
	},
	textlang.Vietnamese: {
		draftedReplyWaiting: "Một thư trả lời cho %q đã được soạn và đang chờ gửi: " +
			"cuộc trò chuyện không để lại bước tiếp theo nào được lên kế hoạch",
		counterpartyWorthKeeping: "%s có phải là một liên hệ đáng giữ lại không?",
		companyRename:            "Đổi tên %s thành %s?",
		siteLeadFound:            "Tìm thấy trên %s: %s (%s)",
		enrichmentOf:             "Bổ sung thông tin từ %s",
		contractEndedMove:        "Email của họ cho biết hợp đồng đã kết thúc. Chuyển công ty này từ %s sang %s?",
		overwriteHumanEdited:     "Ghi đè các trường đã được chỉnh sửa thủ công (%s): %s",
		linkedInLooksLike:        "%s tại %s có vẻ là %s",
		unnamedEmployer:          "một nơi làm việc không rõ tên",
		vcardResemblesContact:    "Danh thiếp này giống một liên hệ đã có. Vẫn tạo %s chứ?",

		heldNotSent:          "%q chưa được gửi: %s",
		heldConsentWithdrawn: "một người nhận đã rút lại sự đồng ý cho mục đích này sau khi tin nhắn được lên lịch",
		heldSenderInactive:   "tài khoản gửi hoặc hộp thư của nó không còn hoạt động",
		heldPassportRevoked: "thông tin xác thực của agent dùng để lên lịch đã bị thu hồi hoặc hết hạn. " +
			"Tài khoản của bạn vẫn ổn, hãy tự gửi nếu tin nhắn vẫn cần được gửi",
		heldMissedWindow: "thời điểm gửi đã trôi qua khi không có gì đang chạy, " +
			"và giờ đã quá muộn để tin nhắn còn đúng như khi được viết",
		heldTimerExhausted: "tác vụ kích hoạt việc gửi đã hết số lần thử",
		heldSendRefused:    "một bước kiểm tra đã từ chối tin nhắn khi gửi",

		reviewSendRefused:  "Một lần gửi tới %s đã bị từ chối (%s) và có người đang hỏi liệu vẫn có thể gửi đi không",
		reviewOneRecipient: "1 người nhận",
		reviewRecipients:   "%d người nhận",

		coldStartFromURL:             "Đọc dữ liệu khởi tạo từ %s",
		coldStartFromText:            "Đọc dữ liệu khởi tạo từ văn bản đã dán",
		coldStartFromSelfDescription: "Đọc dữ liệu khởi tạo từ một bản tự mô tả",

		fxRateChanged:    "%s → %s %s (trước đây %s)",
		fxNoRateInForce:  "không có tỷ giá nào hiệu lực hôm nay",
		modelRateChanged: "%s/%s đầu vào %s (trước đây %s)",
		modelRateNew:     "(mới)",

		createdUnder: "thuộc",
		nestedFields: "{%d trường}",

		acts: agentActsVietnamese,
	},
}

// approvalSummaryCopyFor answers the installation's language, and English for
// anything else — the language comes off a settings row an admin can edit, and
// answering in a language beats answering in none.
func approvalSummaryCopyFor(lang textlang.Lang) approvalSummaryCopy {
	said, ok := approvalSummaryByLang[lang]
	if !ok {
		said, lang = approvalSummaryByLang[textlang.English], textlang.English
	}
	said.lang = lang
	return said
}

// approvalSummaryCopyIn is the set for a stager already inside a transaction.
func approvalSummaryCopyIn(ctx context.Context, tx pgx.Tx) approvalSummaryCopy {
	return approvalSummaryCopyFor(baseLanguageForSummary(ctx, tx))
}

// approvalSummaryCopyOver is the set for a stager holding only the pool.
func approvalSummaryCopyOver(ctx context.Context, pool *pgxpool.Pool) approvalSummaryCopy {
	return approvalSummaryCopyFor(installationLanguage(pool).Resolve(ctx))
}
