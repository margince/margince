// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package knowledge

import (
	"fmt"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/platform/httperr"
)

// The contract's caps on a corpus's text, in characters.
const (
	maxCorpusName        = 200
	maxCorpusTopic       = 500
	maxCorpusDescription = 2000
)

// requireCorpusText is the one rule for a corpus's name and topic statement,
// on create and edit alike: visible text, within the contract's cap. The topic
// is what the command palette routes a question by, so a blank one leaves a
// corpus nothing can be sent to.
func requireCorpusText(field, raw string, limit int) (string, error) {
	text, err := httperr.RequireNonBlank(field, raw)
	if err != nil {
		return "", err
	}
	return text, checkCorpusLength(field, text, limit)
}

// checkCorpusLength refuses text past limit characters.
func checkCorpusLength(field, text string, limit int) error {
	if utf8.RuneCountInString(text) > limit {
		return httperr.Validation(field, "too_long", fmt.Sprintf("%s holds at most %d characters", field, limit))
	}
	return nil
}
