// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/ports/extraction"
)

// documentQuestion is what one reading asks of a document. It holds the
// prompt, the closed field set and how each answer is bounded and checked.
// The run, the lanes and the three outcomes are the same for every question.
type documentQuestion struct {
	// name is the question as the reading's row stores it.
	name string
	// intro is the prompt's first line, saying what the untrusted input is.
	intro string
	// system is the system prompt, before the fence rule is added.
	system string
	// fields is the closed field set in stored names, in asking order.
	fields func() []string
	// toModel and fromModel cross between stored and model field names.
	toModel, fromModel func(string) string
	// answer is the answer-format paragraph of the prompt.
	answer func() string
	// maxValue bounds one field's raw value.
	maxValue func(field string) int
	// coerce turns one raw value into the stored value, or refuses it.
	coerce func(name, raw string) (string, bool)
	// supported checks that a value is in the quote cited for it.
	supported func(documentField) bool
	// resolve runs after every field is grounded, for rules across fields.
	resolve func([]extraction.ExtractedField) []extraction.ExtractedField
}

// dealFacts reads a document for the four deal fields it states.
var dealFacts = documentQuestion{
	name:      activities.ExtractionQuestionDealFacts,
	intro:     "One business document (untrusted).",
	system:    documentSystem,
	fields:    dealFactsFieldOrder,
	toModel:   toModelField,
	fromModel: fromModelField,
	answer:    dealFactsAnswer,
	maxValue:  func(string) int { return maxDocumentValue },
	coerce:    coerceDocumentValue,
	supported: valueSupportedByQuote,
	resolve:   resolveDocumentAmount,
}

// documentQuestionNamed is the question a reading's row names. An unknown name
// is refused, which a reading closes as could-not-read.
func documentQuestionNamed(name string) (documentQuestion, bool) {
	switch name {
	case activities.ExtractionQuestionDealFacts:
		return dealFacts, true
	case activities.ExtractionQuestionChatCapture:
		return chatCapture, true
	}
	return documentQuestion{}, false
}
