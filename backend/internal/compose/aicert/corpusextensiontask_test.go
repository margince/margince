// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert_test

import "github.com/margince/margince/backend/internal/compose/aitasks"

// extensionTaskProofs prove the extension_task check refuses an answer that
// misses the words the scenario names.
func extensionTaskProofs() map[string]judgelessProof {
	return map[string]judgelessProof{
		"reads_a_total_into_the_declared_schema": {
			correct:   literal(`{"total":120.5,"currency":"EUR"}`),
			wrong:     literal(`{"total":99,"currency":"USD"}`),
			wantWrong: aitasks.OutcomeWrongAnswer,
		},
		"summarises_the_text_in_plain_words": {
			correct:   literal("The delivery moves to Thursday."),
			wrong:     literal("The delivery stays as planned."),
			wantWrong: aitasks.OutcomeWrongAnswer,
		},
	}
}
