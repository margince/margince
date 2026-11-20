// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// A failed approval's line reads in the reader's language when the browser has
// words for its code. A code without them falls back to the English sentence.
// Both directions fail here.

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/approvals"
)

const frontendWorklistReader = "../frontend/src/features/worklist/worklist.reader.ts"

func TestEveryEffectFailureCodeHasWorklistWords(t *testing.T) {
	t.Parallel()
	inGo := map[string]bool{}
	for _, code := range approvals.EffectFailureCodes() {
		inGo[code] = true
	}
	inTS := tsKeySet(t, tsObjectEntries(t, frontendWorklistReader, "export const EFFECT_FAILURE_WORDS"))
	wordsMatchVocabulary(t, "effect failure code", "EFFECT_FAILURE_WORDS", inGo, inTS)
}
