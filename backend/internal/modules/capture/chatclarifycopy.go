// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// What a company account says to a colleague about forwarded messages, in the
// language the colleague reads Margince in. A colleague with no language set
// reads English.

import (
	"fmt"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture/whatsapp"
	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
)

// clarifyWords is one language's copy.
type clarifyWords struct {
	// ask is the question, and askCard the one for a network whose customers are
	// named by account, which only a contact card carries.
	ask, askCard string
	// pickHead opens the question that lists the batches, and pickLine is one of
	// them: its place, when it was forwarded, how many and an excerpt.
	pickHead, pickLine string
	// hint and hintCard tell a colleague who wrote something that is no answer
	// how to answer.
	hint, hintCard string
}

var clarifyCatalog = map[string]clarifyWords{
	"en": {
		ask:      "Who wrote this? Reply with their contact card or phone number.",
		askCard:  "Who wrote this? Reply with their contact card.",
		pickHead: "Several sets of forwarded messages are waiting for an author. Reply to the question about the set you mean, with the contact card of who wrote it:",
		pickLine: "%d. %s, %d message(s): \"%s\"",
		hint:     "Please reply with the customer's contact card or phone number, so I can file the messages you forwarded.",
		hintCard: "Please reply with the customer's contact card, so I can file the messages you forwarded.",
	},
	"de": {
		ask:      "Von wem ist das? Antworte mit der Kontaktkarte oder der Telefonnummer.",
		askCard:  "Von wem ist das? Antworte mit der Kontaktkarte.",
		pickHead: "Es warten mehrere Gruppen weitergeleiteter Nachrichten darauf, dass du sagst, von wem sie sind. Antworte auf die Frage zur gemeinten Gruppe mit der Kontaktkarte des Kunden, von dem sie stammen:",
		pickLine: "%d. %s, %d Nachricht(en): \"%s\"",
		hint:     "Bitte antworte mit der Kontaktkarte oder der Telefonnummer des Kunden, damit ich die weitergeleiteten Nachrichten ablegen kann.",
		hintCard: "Bitte antworte mit der Kontaktkarte des Kunden, damit ich die weitergeleiteten Nachrichten ablegen kann.",
	},
	"vi": {
		ask:      "Ai đã viết tin này? Hãy trả lời bằng danh thiếp hoặc số điện thoại của họ.",
		askCard:  "Ai đã viết tin này? Hãy trả lời bằng danh thiếp của họ.",
		pickHead: "Có nhiều nhóm tin nhắn chuyển tiếp đang chờ người viết. Hãy trả lời câu hỏi về nhóm bạn muốn nói, kèm danh thiếp của người viết:",
		pickLine: "%d. %s, %d tin nhắn: \"%s\"",
		hint:     "Hãy trả lời bằng danh thiếp hoặc số điện thoại của khách hàng để tôi lưu các tin nhắn bạn đã chuyển tiếp.",
		hintCard: "Hãy trả lời bằng danh thiếp của khách hàng để tôi lưu các tin nhắn bạn đã chuyển tiếp.",
	},
}

// wordsFor is the copy for a language, English when it has none.
func wordsFor(locale string) clarifyWords {
	if words, ok := clarifyCatalog[locale]; ok {
		return words
	}
	return clarifyCatalog["en"]
}

// phoneAnswers says whether a provider names its customers by phone
// number, so a phone number answers the question.
func phoneAnswers(provider string) bool { return provider == whatsapp.Provider }

// clarifyQuestion is the question about one batch.
func clarifyQuestion(locale, provider string) string {
	words := wordsFor(locale)
	if phoneAnswers(provider) {
		return words.ask
	}
	return words.askCard
}

// hintQuestion is the one hint a colleague gets when they wrote something that
// answers nothing.
func hintQuestion(locale, provider string) string {
	words := wordsFor(locale)
	if phoneAnswers(provider) {
		return words.hint
	}
	return words.hintCard
}

// pickItem is one batch the colleague is asked to choose between.
type pickItem struct {
	At      time.Time
	Count   int
	Excerpt string
}

// maxExcerptRunes bounds the words of a message quoted back to its forwarder.
const maxExcerptRunes = 40

// followUpQuestion lists the batches waiting for an answer.
func followUpQuestion(locale string, items []pickItem) string {
	words := wordsFor(locale)
	lines := []string{words.pickHead}
	for i, item := range items {
		lines = append(lines, fmt.Sprintf(words.pickLine, i+1, item.At.UTC().Format("2006-01-02 15:04 UTC"), item.Count, excerptOf(item.Excerpt)))
	}
	return strings.Join(lines, "\n")
}

func excerptOf(body string) string {
	body = strings.Join(strings.Fields(body), " ")
	return textcut.RunesMarked(body, maxExcerptRunes, "…")
}
