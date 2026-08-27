// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package runner

// The run's final summary is filed on a record the whole team reads, so it is
// written in the installation's shared language rather than whichever language
// the run's observations happened to be in.
//
// The rule reaches this package as text: promptlang.Rule renders it in compose,
// because a module may not import compose and rendering a second copy here is
// the duplication that package exists to prevent. What THIS package owes is
// narrow and worth holding — that a rule it was handed reaches the prompt, and
// that being handed none produces no rule rather than a blank line where one
// belongs. The whole-tree gate in backend/gates/promptlanguage_test.go reads one file
// at a time and cannot follow a string across the package boundary, which is
// what the waiver on asRequest says and what these two tests stand in for.

import (
	"strings"
	"testing"
)

func TestTheRunnerPromptCarriesTheLanguageItWasGiven(t *testing.T) {
	// Deliberately not a real promptlang.Rule: this package cannot import that
	// one, and a fixture that quoted its text would go stale silently the day
	// the wording changed. What is being proven is that the block PASSES
	// THROUGH, so a string nothing else could have produced proves it best.
	const rule = "LANGUAGE\nWrite every human-readable sentence of your output in Vietnamese."

	win := newWindow(Job{Goal: "prep the meeting", LanguageRule: rule}, nil, nil)
	system := win.asRequest(1000).System

	if !strings.Contains(system, rule) {
		t.Fatalf("the language rule never reached the system prompt.\nwanted to find:\n%s\n\ngot:\n%s", rule, system)
	}
	// Before the tool listing, because the listing is the last thing the prompt
	// carries and a rule after it reads as commentary on the tools.
	if strings.Index(system, rule) > strings.Index(system, "Available tools:") {
		t.Error("the language rule sits after the tool listing, where it reads as a note about the tools rather than an instruction about the answer")
	}
}

func TestARunGivenNoLanguageRuleCarriesNone(t *testing.T) {
	// The certification lane builds a Job with no rule on purpose: a cert grades
	// a fixed corpus, and a score that moved with an installation's settings
	// would not be comparable between two installations.
	win := newWindow(Job{Goal: "prep the meeting"}, nil, nil)
	system := win.asRequest(1000).System

	if strings.Contains(system, "LANGUAGE") {
		t.Errorf("a run given no language rule still carries one:\n%s", system)
	}
	// The empty case must not leave the gap the rule would have filled. A prompt
	// that ends a section with two blank lines is not broken, but it is the tell
	// that the writer wrote the separator unconditionally — and the next person
	// to add a block there inherits the same bug with real text in it.
	if strings.Contains(system, "\n\n\n") {
		t.Errorf("the absent rule left its separator behind:\n%q", system)
	}
}
