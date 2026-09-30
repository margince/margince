// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import "github.com/margince/margince/backend/internal/shared/ports/decision"

// decisionQuestionKey is the one question every decision site asks today: which
// kind of thing the state is. A site reads its answer back under the same key.
const decisionQuestionKey = "kind"

// choiceQuestion is a closed-set question: the model picks one of the criteria's
// labels, and each label carries the rule that makes it the right one.
func choiceQuestion(instructions string, criteria map[string]string) decision.Question {
	return decision.Question{Type: decision.Choice, Instructions: instructions, Criteria: criteria}
}
