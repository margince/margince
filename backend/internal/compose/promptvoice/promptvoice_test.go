// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package promptvoice_test

// What the voice block must actually say.
//
// The gate in backend/promptvoice_test.go proves the block is ATTACHED to
// every prose surface. It cannot prove the block says anything, and a Rule
// silently emptied would satisfy it perfectly — four surfaces composing a
// constant that instructs nothing, with every test still green.
//
// So this asserts the rules a reader of the product's own voice guidance would
// check for. Not the wording: a rule rephrased is still the rule, and a test
// pinned to today's sentences would fail on every edit and teach the next
// author to delete it. What is pinned is the small set of instructions the
// voice would stop being Margince's without.

import (
	"strings"
	"testing"

	"github.com/gradionhq/margince/backend/internal/compose/promptvoice"
)

func TestTheVoiceAnnouncesItselfToTheModel(t *testing.T) {
	// Rule is BUILT from Heading, so asserting that it starts with Heading
	// would be a tautology — it cannot fail, whatever either value becomes.
	// What can fail is the heading ceasing to be a heading: the block is
	// concatenated between two others in every prompt, and a model reads the
	// boundary from the word and the newline. An empty or unterminated heading
	// runs the voice into the end of the sentence above it.
	if !strings.HasSuffix(promptvoice.Heading, "\n") {
		t.Errorf("promptvoice.Heading is %q, which does not end in a newline: composed between two blocks, "+
			"it would run into the line above it", promptvoice.Heading)
	}
	if len(strings.TrimSpace(promptvoice.Heading)) == 0 {
		t.Error("promptvoice.Heading is blank, so the voice block opens with nothing naming it")
	}
	// The rule must carry more than its own heading.
	if strings.TrimSpace(strings.TrimPrefix(promptvoice.Rule, promptvoice.Heading)) == "" {
		t.Error("promptvoice.Rule is its heading and nothing else: every prose surface composes an instruction that instructs nothing")
	}
}

func TestTheVoiceCarriesEveryInstructionItExistsFor(t *testing.T) {
	// Each entry is a rule from the product's voice guidance that changes what
	// the model writes. A rule dropped here is a rule the product stops
	// following, and nothing else in the tree would notice.
	required := map[string]string{
		"speaks as Margince":         "Margince",
		"says I for its own actions": "I",
		"addresses the reader":       "you",
		"leads with the result":      "Lead with the result",
		"one idea per sentence":      "One idea per sentence",
		"admits what it cannot see":  "could not see",
		"bans the filler openers":    "Absolutely",
		"bans marketing verbs":       "leverage",
		"bans exclamation marks":     "No exclamation marks",
		"bans hedging":               "it appears that",
	}
	rule := promptvoice.Rule
	for what, instruction := range required {
		if !strings.Contains(rule, instruction) {
			t.Errorf("the voice no longer %s: %q is gone from promptvoice.Rule", what, instruction)
		}
	}
}

func TestTheVoiceIsNotEmpty(t *testing.T) {
	// The failure the assertions above cannot see on their own: a Rule reduced
	// to its heading and a handful of keywords would pass every Contains check
	// while instructing nothing.
	body := strings.TrimSpace(strings.TrimPrefix(promptvoice.Rule, promptvoice.Heading))
	if len(body) < 400 {
		t.Errorf("promptvoice.Rule's body is %d characters, which is too short to be the voice: "+
			"a block this small cannot carry the instructions the surfaces depend on", len(body))
	}
}
