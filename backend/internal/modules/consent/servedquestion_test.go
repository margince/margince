// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"testing"

	"github.com/margince/margince/backend/internal/platform/mailcopy"
)

// TestEveryServedQuestionSplitsIntoItsAskAndAnswers. The page shows the parts
// and sends the whole text back, so no part may carry a line break of its own.
func TestEveryServedQuestionSplitsIntoItsAskAndAnswers(t *testing.T) {
	answers := map[string]int{TemplateMarketingQuestion: 2, TemplateSubscriptionQuestion: 1}
	for key, want := range answers {
		for _, language := range mailcopy.Languages() {
			ask, got := ServedQuestion{Text: RenderQuestion(key, string(language))}.Parts()
			if ask == "" || len(got) != want {
				t.Errorf("%s/%s splits into ask %q and %d answers, want %d", key, language, ask, len(got), want)
			}
		}
	}
}
