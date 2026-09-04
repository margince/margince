// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The onboarding conversation's hand-written copy, per language.
//
// Every line here used to be an `if locale == "de"` with English underneath,
// which is a shape that reads as a choice and behaves as a floor: a third
// shipped language took the English branch, silently, and nothing anywhere
// failed. A Vietnamese reader was onboarded in English inside an otherwise
// Vietnamese product, and the only way to find out was to run it.
//
// A table instead, keyed by the language, with the shipped set as the census.
// TestEveryShippedLanguageHasOnboardingCopy fails when the product gains a
// language this file has not learned — before a reader does.
//
// The model-written half of the conversation needs none of this: it is
// instructed through promptlang.Rule, which already answers for every shipped
// language. What lives here is the copy this product wrote itself.

import (
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// onboardingCopy is one language's set. Every field is required — a partial
// set is the same silent English fallback in miniature, and the census below
// refuses one.
type onboardingCopy struct {
	// entityQuestion asks which of several legal entities on the site is the
	// reader's own company.
	entityQuestion string
	// addressQuestion asks the same about several registered addresses.
	addressQuestion string
	// conflictQuestion asks which value to keep where the saved one and the
	// site's disagree. %s is the field's key, which is NOT translated: it
	// names a field the reader can see on their own screen.
	conflictQuestion string
	// keepLabel / keepDetail present the saved value as the choice.
	keepLabel  string
	keepDetail string
	// takeLabel / takeDetail present the site's value as the choice.
	takeLabel  string
	takeDetail string
	// statusConfirmed answers "is it working" once the profile is saved.
	statusConfirmed string
	// statusFailed answers it when the research stopped without completing.
	// %d is how many required details are still missing.
	statusFailed string
	// statusResearching answers it while the research is still running.
	statusResearching string
	// statusMissing answers it when the research works and details remain.
	// %d is how many.
	statusMissing string
	// selectionRecorded is what the conversation says when a clicked clarify
	// option is recorded without the model — the choice is already a verified
	// fact, so it lands whether or not prose could be written about it.
	selectionRecorded string
	// selectionReason is that change's provenance on the record: the human
	// picked it, which is a stronger source than any page.
	selectionReason string
}

// onboardingCopyByLang is the census. Keyed by textlang.Lang rather than by a
// string, so the gate can walk textlang.Shipped and ask this map directly.
var onboardingCopyByLang = map[textlang.Lang]onboardingCopy{
	textlang.English: {
		entityQuestion:    "The legal notice names more than one legal entity. Which one is your company?",
		addressQuestion:   "The website states more than one registered address. Which one belongs to your company?",
		conflictQuestion:  "Your saved value for %s differs from what the website states. Which value should I use?",
		keepLabel:         "Keep my value",
		keepDetail:        "Entered by a human; confirming keeps it unchanged (keep_current).",
		takeLabel:         "Use the website's value",
		takeDetail:        "Read from the website; confirming takes this value (accept_proposal).",
		statusConfirmed:   "Yes. I saved the confirmed company profile.",
		statusFailed:      "My website research has stopped without completing. We can fill the %d missing required details together here; nothing is saved yet.",
		statusResearching: "Yes. I'm still researching and will add grounded findings to the company draft as they arrive. Nothing is saved yet.",
		statusMissing:     "Yes. The company workspace is working. %d required details remain; nothing is saved yet.",
		selectionRecorded: "Recorded — your choice is on the draft. The assistant did not answer this turn, so there is nothing more to read here.",
		selectionReason:   "You chose this.",
	},
	textlang.German: {
		entityQuestion:    "Die rechtlichen Angaben der Website nennen mehrere juristische Personen. Welche ist Ihr Unternehmen?",
		addressQuestion:   "Die Website nennt mehrere Geschäftsanschriften. Welche gehört zu Ihrem Unternehmen?",
		conflictQuestion:  "Ihr gespeicherter Wert für %s unterscheidet sich von der Website. Welchen Wert soll ich verwenden?",
		keepLabel:         "Meinen Wert behalten",
		keepDetail:        "Von einem Menschen eingetragen; bleibt bei der Bestätigung unverändert (keep_current).",
		takeLabel:         "Wert der Website übernehmen",
		takeDetail:        "Von der Website gelesen; die Bestätigung übernimmt diesen Wert (accept_proposal).",
		statusConfirmed:   "Ja. Ich habe das bestätigte Unternehmensprofil gespeichert.",
		statusFailed:      "Meine Web-Recherche ist beendet, konnte aber nicht abgeschlossen werden. Wir können die %d fehlenden Pflichtangaben hier gemeinsam manuell ergänzen; gespeichert ist noch nichts.",
		statusResearching: "Ja. Ich recherchiere noch und zeige neue belegte Funde im Unternehmensentwurf. Gespeichert ist noch nichts.",
		statusMissing:     "Ja. Meine Recherche funktioniert. Es fehlen noch %d Pflichtangaben; gespeichert ist noch nichts.",
		selectionRecorded: "Übernommen — Ihre Auswahl steht im Entwurf. Der Assistent hat in dieser Runde nicht geantwortet, deshalb gibt es hier nicht mehr zu lesen.",
		selectionReason:   "Von Ihnen ausgewählt.",
	},
	// Addressed as "bạn", the neutral second person this product's Vietnamese
	// UI already uses. The parenthesised keep_current / accept_proposal stay in
	// English: they are the wire values behind the two buttons, and a reader
	// comparing what they clicked with what the record says needs the same
	// word in both places.
	textlang.Vietnamese: {
		entityQuestion:    "Trang thông tin pháp lý của website nêu nhiều pháp nhân. Pháp nhân nào là công ty của bạn?",
		addressQuestion:   "Website nêu nhiều địa chỉ đăng ký. Địa chỉ nào là của công ty bạn?",
		conflictQuestion:  "Giá trị đã lưu của bạn cho %s khác với thông tin trên website. Tôi nên dùng giá trị nào?",
		keepLabel:         "Giữ giá trị của tôi",
		keepDetail:        "Do một người nhập vào; xác nhận sẽ giữ nguyên giá trị này (keep_current).",
		takeLabel:         "Dùng giá trị của website",
		takeDetail:        "Đọc được từ website; xác nhận sẽ lấy giá trị này (accept_proposal).",
		statusConfirmed:   "Vâng. Tôi đã lưu hồ sơ công ty đã được xác nhận.",
		statusFailed:      "Phần tìm hiểu website của tôi đã dừng mà chưa hoàn tất. Chúng ta có thể cùng điền %d thông tin bắt buộc còn thiếu ngay tại đây; hiện chưa có gì được lưu.",
		statusResearching: "Vâng. Tôi vẫn đang tìm hiểu và sẽ bổ sung vào bản nháp công ty những thông tin có căn cứ khi tìm được. Hiện chưa có gì được lưu.",
		statusMissing:     "Vâng. Không gian làm việc của công ty đang hoạt động. Còn thiếu %d thông tin bắt buộc; hiện chưa có gì được lưu.",
		selectionRecorded: "Đã ghi nhận — lựa chọn của bạn đã nằm trong bản nháp. Trợ lý không trả lời ở lượt này, nên ở đây không còn gì để đọc thêm.",
		selectionReason:   "Bạn đã chọn giá trị này.",
	},
}

// copyFor answers the reader's language, and English for anything else.
//
// The fallback is unreachable for a language the product ships — the census
// holds that — and it is still here because this takes a string off the wire.
// A caller that reached it with a locale the contract admits would be a bug
// this function cannot fix; what it can do is answer in a language rather than
// in nothing.
func copyFor(locale string) onboardingCopy {
	if said, ok := onboardingCopyByLang[textlang.Lang(locale)]; ok {
		return said
	}
	return onboardingCopyByLang[textlang.English]
}

// onboardingLocaleRefusal names the languages the onboarding conversation
// speaks, derived from the shipped set rather than spelled out — the refusal a
// caller reads must not be able to name a different set from the one the
// contract admits.
func onboardingLocaleRefusal() string {
	codes := make([]string, 0, len(textlang.Shipped))
	for _, lang := range textlang.Shipped {
		codes = append(codes, string(lang))
	}
	return fmt.Sprintf("locale must be one of %v", codes)
}
