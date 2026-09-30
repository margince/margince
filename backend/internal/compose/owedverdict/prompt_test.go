// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package owedverdict

// The ruleset stamp's stability, and the reason it is built the way it is.
//
// Both failures here are silent. A stamp that moves on its own re-judges every
// workspace after every restart and nothing logs it; a stamp blind to the
// rendering holds still while the question changes and nothing logs that
// either. Neither is visible from the value, which looks like a digest whatever
// it is a digest of.

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
)

// The stamp is a property of the code, not of the process that computed it.
func TestTheRulesetStampIsStableAcrossProcesses(t *testing.T) {
	build := func(fence promptfence.Fence) string {
		return SystemFor(fence) + "\n" + Prompt(fence, []activities.OwedCandidate{rulesetSample})
	}
	first, second := ai.PromptDigest(build), ai.PromptDigest(build)
	if first != second {
		t.Fatalf("the stamp moved between two computations in one process: %q then %q — "+
			"every restart would re-judge every workspace", first, second)
	}
	if first != Ruleset {
		t.Errorf("Ruleset = %q but recomputing the same builder gives %q", Ruleset, first)
	}
}

// Why the system prompt and the user turn are folded into ONE builder string.
//
// PromptDigest canonicalises each builder's output against the marker THAT
// string declares. The system prompt declares the fence marker; a user turn
// does not, so a user turn digested on its own keeps a live nonce and hashes
// differently every time.
//
// Asserted as the POSITIVE fact rather than left as a comment, because the
// folding otherwise reads as a style choice and the obvious tidy-up — passing
// the two builders separately, which PromptDigest's variadic signature invites
// — reintroduces exactly this.
func TestAUserTurnDigestedAloneIsNotStable(t *testing.T) {
	userTurn := func(fence promptfence.Fence) string {
		return Prompt(fence, []activities.OwedCandidate{rulesetSample})
	}
	if first, second := ai.PromptDigest(userTurn), ai.PromptDigest(userTurn); first == second {
		t.Fatalf("a user turn digested alone is stable at %q, so the folding in Ruleset "+
			"no longer earns its keep — check whether PromptDigest or the fence changed", first)
	}
	// And the two-builder form, which is the shape the tidy-up takes.
	if first, second := ai.PromptDigest(SystemFor, userTurn), ai.PromptDigest(SystemFor, userTurn); first == second {
		t.Fatal("PromptDigest(system, userTurn) is stable, so the one-string form is no longer required — " +
			"the comment on Ruleset should be corrected rather than left overstating the constraint")
	}
}

// Every optional line of the template is in the sample, so a label edit anywhere
// moves the stamp.
//
// A sample missing one of them would leave that part of the prompt free to
// change under verdicts still claiming to have been judged by it.
func TestTheRulesetSampleRendersEveryLineOfTheTemplate(t *testing.T) {
	rendered := Prompt(promptfence.New(), []activities.OwedCandidate{rulesetSample})
	for _, line := range []string{"Subject:", "To:", "Cc:", "calendar invitation", "context_for", "Sent:"} {
		if !strings.Contains(rendered, line) {
			t.Errorf("the ruleset sample does not render %q, so an edit to that line would not move the stamp", line)
		}
	}
}
