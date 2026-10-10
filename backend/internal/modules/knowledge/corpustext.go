// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package knowledge

import "github.com/margince/margince/backend/internal/platform/httperr"

// The contract's caps on a corpus's text, in characters.
const (
	maxCorpusName        = 200
	maxCorpusTopic       = 500
	maxCorpusDescription = 2000
	maxAskQuestion       = 1000
)

// RequireQuestion is the rule for a question put to a corpus: visible text
// within the contract's cap. A blank one is refused rather than answered as
// "not covered", which would read as a verdict on the corpus.
func RequireQuestion(raw string) (string, error) {
	return requireCorpusText("question", raw, maxAskQuestion)
}

// requireCorpusText is the one rule for a corpus's name and topic statement,
// on create and edit alike: visible text, within the contract's cap. The topic
// is what the command palette routes a question by, so a blank one leaves a
// corpus nothing can be sent to.
func requireCorpusText(field, raw string, limit int) (string, error) {
	text, err := httperr.RequireNonBlank(field, raw)
	if err != nil {
		return "", err
	}
	return text, httperr.RequireWithin(field, text, limit)
}
