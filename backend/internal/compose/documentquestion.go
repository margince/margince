// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The question a document reading answers. One run record, one lane choice
// and one set of three outcomes serve every question. A question brings its
// prompt, its closed field set, and how a reported value becomes a stored one.

import (
	"fmt"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/ports/extraction"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

type documentQuestion struct {
	name string
	// fields are the stored field names, in the order the prompt asks for them.
	fields    []string
	toModel   func(field string) string
	fromModel func(modelField string) string
	// valueBound is the longest value one model field may carry.
	valueBound func(modelField string) int
	coerce     func(field, raw string) (string, bool)
	// settle runs once over the whole grounded reading.
	settle  func([]extraction.ExtractedField) []extraction.ExtractedField
	request func(documentSource) model.Request
	// noneGrounded says why a finished reading offered nothing.
	noneGrounded langcopy.Phrase
}

func (q documentQuestion) modelFields() []string {
	names := make([]string, len(q.fields))
	for i, name := range q.fields {
		names[i] = q.toModel(name)
	}
	return names
}

func (q documentQuestion) maxValue(modelField string) int { return q.valueBound(modelField) }

func dealFactsQuestion() documentQuestion {
	return documentQuestion{
		name:         activities.ExtractionQuestionDealFacts,
		fields:       documentFieldOrder(),
		toModel:      toModelField,
		fromModel:    fromModelField,
		valueBound:   func(string) int { return maxDocumentValue },
		coerce:       coerceDocumentValue,
		settle:       resolveDocumentAmount,
		request:      documentExtractRequest,
		noneGrounded: documentWords.NoFields,
	}
}

// questionNamed is the question a stored reading names. The column's CHECK
// admits only these, so an unknown name is a schema this binary predates.
func questionNamed(name string) (documentQuestion, error) {
	switch name {
	case activities.ExtractionQuestionDealFacts:
		return dealFactsQuestion(), nil
	case activities.ExtractionQuestionChatCapture:
		return chatCaptureQuestion(), nil
	}
	return documentQuestion{}, fmt.Errorf("compose: a reading asks %q, which this build does not know", name)
}
