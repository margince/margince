// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H1

package gates

// Every prompt either speaks in Margince's one voice or says why it does not.
//
// Two prose surfaces existed before promptvoice and they already disagreed:
// the deal card asked its model for "a capable colleague briefing you in the
// corridor", the meeting brief for "a calm, capable colleague" addressing the
// reader as "you". Neither was wrong. Both being hand-written is what was
// wrong — a product with two voices has none.
//
// WHY THIS GATE IS INVERTED. Its first version carried a LIST of the four
// files that write prose, and justified the list by saying those four were
// what "Margince writes prose" meant. The list was already wrong on the day it
// shipped: orgdossier/growthfitwrite.go writes the recommended angle a reader
// sees on the company page, carried its own hand-rolled "write one claim per
// sentence, plainly", and sat in the same package as one of the four. A list
// can only fail SHORT — it reads a smaller tree and reports the same word for
// it, PASS — which is the failure CLAUDE.md rule 8 names, and it failed that
// way immediately.
//
// So the question is asked of every model.Request in the tree, exactly as the
// language gate asks its own. A prompt whose reply is data answers with a
// waiver naming what it returns, which is a sentence somebody can check;
// silence is not an available answer.
//
// WHAT IT CAN AND CANNOT SEE. Like its sibling this is a syntactic reach: it
// proves the block was ATTACHED, never that the output sounds like anything.
// It also does not forbid a governed prompt from adding voice wording of its
// own — a surface may legitimately carry a rule the shared block does not
// ("never restate the stage name"), and forbidding that would push authors
// back to rolling their own.

import (
	"go/ast"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/promptvoice"
)

// The heading a voice rule opens with, read from the package that DEFINES it
// rather than restated here — the reason languageRuleHeading is read from
// promptlang. A gate holding its own copy of the string it searches for goes
// quietly permissive the day the real one changes: still running, still
// passing, no longer about anything.
var voiceRuleHeading = promptvoice.Heading

// voiceWaiverPrefix marks a request whose output no person reads as prose.
//
// Deliberately NOT the language gate's prefix. The first version of this gate
// reused everyManyRequest's waiver walk, which matched //promptlang:exempt
// only — so it told authors to write //promptvoice:exempt, ignored that
// comment entirely, and let a pre-existing language waiver on an unrelated
// function silently exempt a file from the voice rule. The two answers are
// different answers: a prompt returning an enum needs neither, but a prompt
// returning a person's own name in their own language needs a language waiver
// and still has no prose to sound like anything.
const voiceWaiverPrefix = "//promptvoice:exempt"

func TestEveryPromptEitherSpeaksInTheOneVoiceOrSaysWhyNot(t *testing.T) {
	t.Parallel()
	sites := everyModelRequest(t)
	if len(sites) == 0 {
		t.Fatal("found no model.Request literals at all; this gate would pass vacuously")
	}
	for _, site := range sites {
		if site.voiceGoverned || site.voiceWaived {
			continue
		}
		t.Errorf("%s builds a model.Request that neither composes the shared voice nor waives it. "+
			"If a person reads its output as prose, compose promptvoice.Rule into the System prompt; "+
			"if the reply is data — an enum, a number, a field copied out of a document — mark it %s <reason>",
			site.where, voiceWaiverPrefix)
	}
}

func TestEveryVoiceWaiverGivesAReason(t *testing.T) {
	t.Parallel()
	for _, site := range everyModelRequest(t) {
		if site.voiceWaived && site.voiceWaiverReason == "" {
			t.Errorf("%s waives the voice without saying why. A waiver nobody can check is one nobody will "+
				"revisit: say what this request returns that no person reads as prose", site.where)
		}
	}
}

// TestTheVoiceWaiverIsItsOwnAnswer is the regression this gate's own history
// demands.
//
// Sharing the language gate's walk while it hard-coded one prefix made the
// voice waiver a dead constant: the error message named //promptvoice:exempt
// and the code honoured //promptlang:exempt. Every voice waiver in the tree
// must therefore be a VOICE waiver, and the two sets must be able to differ —
// a file waiving one and not the other is the case that proves they are read
// separately.
func TestTheVoiceWaiverIsItsOwnAnswer(t *testing.T) {
	t.Parallel()
	languageOnly := 0
	for _, site := range everyModelRequest(t) {
		if site.waived && !site.voiceWaived {
			languageOnly++
		}
	}
	if languageOnly == 0 {
		t.Error("no request waives the language rule without also waiving the voice. That is not impossible, " +
			"but it is what a gate reading the WRONG prefix looks like: check that voiceWaiverPrefix is the " +
			"prefix waiverAround is actually given for the voice answer")
	}
}

// systemCarriesTheVoice reports whether the shared voice reaches this
// literal's System field.
//
// Textual over the whole file, for the reason systemCarriesALanguageRule is:
// the System expression is a concatenation of constants built elsewhere in the
// file, and following the value would mean resolving constants across
// packages.
func systemCarriesTheVoice(lit *ast.CompositeLit, source string) bool {
	if !hasSystemField(lit) {
		// A request with no System field carries no prompt for a rule to live
		// in — a continuation turn, or a tool-result round trip. The prompt
		// that started it is governed where it was built.
		return true
	}
	return strings.Contains(source, "promptvoice.") || strings.Contains(source, voiceRuleHeading)
}
