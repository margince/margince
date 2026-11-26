// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestTheLanguageKeyedSummariesFollowTheStylePages(t *testing.T) {
	holdSentences(t, "signalSummary", signalSummaryByLang[textlang.English], signalSummaryByLang[textlang.Vietnamese])
	holdSentences(t, "technicalSummary", technicalSummaryByLang[textlang.English], technicalSummaryByLang[textlang.Vietnamese])
	holdSentences(t, "projectHealth", projectHealthByLang[textlang.English], projectHealthByLang[textlang.Vietnamese])
	for i, heading := range importProblemsHeaders[textlang.English] {
		langcopytest.EnglishText(t, fmt.Sprintf("importProblemsHeaders[%d]", i), heading)
	}
	for i, heading := range importProblemsHeaders[textlang.Vietnamese] {
		langcopytest.VietnameseText(t, fmt.Sprintf("importProblemsHeaders[%d]", i), heading)
	}
}

func holdSentences[T any](t *testing.T, table string, english, vietnamese T) {
	t.Helper()
	for name, text := range langcopytest.Sentences(t, english) {
		langcopytest.EnglishText(t, table+"."+name, text)
	}
	for name, text := range langcopytest.Sentences(t, vietnamese) {
		langcopytest.VietnameseText(t, table+"."+name, text)
	}
}
