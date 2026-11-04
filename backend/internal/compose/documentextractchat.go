// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The chat-screenshot question: the messages a screenshot shows, and the name
// or number in its header that says who the chat is with.
//
// It is a second question on the one document reading, not a second reader.
// The run, the image lane, the carriage check and the three outcomes are the
// deal-facts reading's own. Nothing here writes a record: the reading stores
// what it grounded, and a human files it.

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/compose/claims"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/ports/extraction"
)

// The four fields a chat reading may report.
const (
	chatFieldText    = "chat_text"
	chatFieldName    = "counterparty_name"
	chatFieldPhone   = "counterparty_phone"
	chatFieldNetwork = "network"
)

// maxChatText bounds the messages one reading may report. It sits under what
// the reading's output budget can carry, so a reply at the bound still closes
// its JSON. A longer reply is refused, not cut: a cut chat would read as whole.
const maxChatText = 12000

// minPhoneDigits is the fewest digits a header number may carry. Fewer is a
// time, a count or a badge, not a number anybody can be reached on.
const minPhoneDigits = 5

const chatSystem = `You read ONE screenshot of a chat in a messenger or a social network's direct
messages, and report only what it SHOWS. Report the messages as they are printed, in the order
they appear, each on its own line, starting with who sent it when the screenshot says so. Report
the name and the phone number shown in the chat's header, which say who the chat is with. Report
the app's name only when the screenshot prints it. Report nothing you infer, translate or
complete: a value the screenshot does not show is not_stated, and saying so is the correct
answer. Quote the exact text each value was read from.`

// chatFieldOrder is the closed chat field set, in asking order.
func chatFieldOrder() []string {
	return []string{chatFieldText, chatFieldName, chatFieldPhone, chatFieldNetwork}
}

// sameField is the model's vocabulary for a chat reading: the stored names.
func sameField(name string) string { return name }

// chatAnswer is the chat question's answer format.
func chatAnswer() string {
	return fmt.Sprintf(
		`Return JSON: { "fields": [ { "field", "stated", "value", "source_quote", "page_or_section", "confidence" } ] } — `+
			`one entry for EACH of %s, in that order, and no others. `+
			`Set "stated" to %q for a field the screenshot shows and %q for one it does not, leaving the other values empty when it does not. `+
			`"value" for %s is every visible message, one per line, as printed; `+
			`for %s it is the name in the chat's header; for %s it is the phone number in the header, as printed; `+
			`for %s it is the app's name, only when the screenshot prints it. `+
			`"source_quote" is the exact text the value was read from: the first message for %s, and the header line for the others. `+
			`"page_or_section" is "header" or "messages".`,
		strings.Join(chatFieldOrder(), ", "), documentStated, documentNotStated,
		chatFieldText, chatFieldName, chatFieldPhone, chatFieldNetwork, chatFieldText)
}

// chatCapture reads a chat screenshot for its messages and its counterparty.
var chatCapture = documentQuestion{
	name:      activities.ExtractionQuestionChatCapture,
	intro:     "One chat screenshot (untrusted).",
	system:    chatSystem,
	fields:    chatFieldOrder,
	toModel:   sameField,
	fromModel: sameField,
	answer:    chatAnswer,
	maxValue:  chatMaxValue,
	coerce:    coerceChatValue,
	supported: chatValueSupported,
	resolve:   func(fields []extraction.ExtractedField) []extraction.ExtractedField { return fields },
}

// chatMaxValue bounds one raw value. Only the messages are long.
func chatMaxValue(field string) int {
	if field == chatFieldText {
		return maxChatText
	}
	return maxDocumentValue
}

// coerceChatValue turns one reported value into the stored one. The messages
// keep their line breaks; a header number must carry enough digits to dial.
func coerceChatValue(name, raw string) (string, bool) {
	switch name {
	case chatFieldText:
		text := strings.TrimSpace(raw)
		return text, text != "" && utf8.RuneCountInString(text) <= maxChatText
	case chatFieldName, chatFieldNetwork:
		value := collapseSpace(raw)
		return value, value != ""
	case chatFieldPhone:
		value := collapseSpace(raw)
		return value, countDigits(value) >= minPhoneDigits
	}
	return "", false
}

// chatValueSupported checks a value against the quote cited for it. The quote
// of the messages is one line, so it must appear in them. The quote of a
// header field is the header line, so the value must appear in it.
func chatValueSupported(field documentField) bool {
	switch field.Field {
	case chatFieldText:
		return claims.Quoted(field.Value, field.Quote)
	case chatFieldName, chatFieldPhone, chatFieldNetwork:
		return claims.Quoted(field.Quote, field.Value)
	}
	return false
}

func countDigits(s string) int {
	n := 0
	for _, r := range s {
		if unicode.IsDigit(r) {
			n++
		}
	}
	return n
}
