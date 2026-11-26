// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

type agentActVocabulary struct{ verbs, recordFrames, operations map[string]string }

func actVocabulary(language textlang.Lang) agentActVocabulary {
	return agentActVocabulary{verbs: systemtext.ActWords(language, "verbs"), recordFrames: systemtext.ActWords(language, "frames"), operations: systemtext.ActWords(language, "operations")}
}

var (
	agentActsEnglish    = actVocabulary(textlang.English)
	agentActsGerman     = actVocabulary(textlang.German)
	agentActsVietnamese = actVocabulary(textlang.Vietnamese)
)
