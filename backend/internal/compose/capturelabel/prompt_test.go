// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capturelabel

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
)

// The stamp is a property of the code, not of the process that computed it:
// one that moved per process would re-offer every declined message after every
// restart.
func TestTheRulesetStampIsStableAcrossProcesses(t *testing.T) {
	first, second := ai.PromptDigest(rulesetPrompt), ai.PromptDigest(rulesetPrompt)
	if first != second {
		t.Fatalf("the stamp moved between two computations in one process: %q then %q", first, second)
	}
	if first != Ruleset {
		t.Errorf("Ruleset = %q but recomputing the same builder gives %q", Ruleset, first)
	}
}

// Half of what the site asks is how a message is laid out, so the stamp must
// move when the user turn does and not only when the system prompt does.
func TestTheRulesetMovesWithTheUserTurn(t *testing.T) {
	outbound := rulesetSample
	outbound.Inbound = false
	moved := ai.PromptDigest(func(fence promptfence.Fence) string {
		return SystemFor(fence) + "\n" + Prompt(fence, []activities.UnlabeledEmail{outbound})
	})
	if moved == Ruleset {
		t.Error("rendering the sample differently left the stamp where it was, so the user turn is not digested")
	}
}
