// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package backendarch

// Every surface that writes PROSE a person reads speaks in one voice, or says
// plainly why it does not.
//
// Two prose surfaces existed before promptvoice and they already disagreed:
// the deal card asked its model for "a capable colleague briefing you in the
// corridor", the meeting brief for "a calm, capable colleague" addressing the
// reader as "you". Neither was wrong. Both being hand-written is what was
// wrong — a product with two voices has none, and nothing would have stopped
// the third author writing a third.
//
// WHAT THIS GATE CAN AND CANNOT SEE. It reuses everyModelRequest from
// promptlanguage_test.go — the same AST walk over the same trees, so the two
// gates can never disagree about what a prompt site IS — and asks whether the
// voice rule reaches each literal's System field. Like its sibling that is a
// syntactic reach: it proves the block was ATTACHED, never that the output
// sounds like anything. What it stops is a NEW prose surface being written
// with a fourth hand-rolled voice paragraph, which is the failure that
// actually happened.
//
// Deliberately NOT enforced here: that a governed prompt contains no voice
// wording of its own. A prompt may legitimately add a rule the shared block
// does not carry ("never restate the stage name"), and a gate that forbade
// that would push authors back to rolling their own.

import (
	"os"
	"strings"
	"testing"

	"github.com/gradionhq/margince/backend/internal/compose/promptvoice"
)

// The heading a voice rule opens with, read from the package that DEFINES it
// rather than restated here — the reason languageRuleHeading is read from
// promptlang. A gate holding its own copy of the string it searches for goes
// quietly permissive the day the real one changes: still running, still
// passing, no longer about anything.
var voiceRuleHeading = promptvoice.Heading

// voiceWaiverPrefix marks a request whose output no person reads as prose. A
// reasonless waiver is itself a finding, the bar //craft:ignore holds.
const voiceWaiverPrefix = "//promptvoice:exempt"

// proseSurfaces are the prompts whose output is prose on a person's screen.
//
// This gate is the one place in the repo that carries a LIST rather than
// deriving from the tree, and that is a deliberate narrowing with a cost worth
// naming: a new prose surface in a file not named here is not caught. The
// alternative was worse. Derived-from-the-tree would mean asking "does this
// request return prose?", which no syntactic check can answer — most model
// requests in this product return data, and a gate that demanded a voice rule
// on all of them would be waived into meaninglessness on its first day.
//
// So the list is the SUBJECT, not a shortcut past one: these four files are
// what "Margince writes prose" means today. TestTheProseSurfaceListIsStillTrue
// below is what keeps it honest — it fails when a listed file stops building a
// prompt at all, which is how a stale entry announces itself.
var proseSurfaces = []string{
	"internal/compose/dealstatus/model.go",
	"internal/compose/meetingbrief/model.go",
	"internal/compose/orgbrief/write.go",
	"internal/compose/orgdossier/write.go",
}

func TestEveryProseSurfaceSpeaksInTheOneVoice(t *testing.T) {
	sites := everyModelRequest(t)
	if len(sites) == 0 {
		t.Fatal("found no model.Request literals at all; this gate would pass vacuously")
	}
	for _, surface := range proseSurfaces {
		found := false
		for _, site := range sites {
			if !strings.HasPrefix(site.where, surface) {
				continue
			}
			found = true
			if voiceGoverned(t, surface) || site.waived {
				continue
			}
			t.Errorf("%s writes prose a person reads but composes no shared voice: it will sound like whatever its author "+
				"hand-wrote, which is how this product came to have two voices. Compose promptvoice.Rule into the System "+
				"prompt, or — if what it returns is not prose — mark it %s <reason> and drop it from proseSurfaces",
				site.where, voiceWaiverPrefix)
		}
		if !found {
			t.Errorf("%s is listed as a prose surface but builds no model.Request. Either it moved — point this list at "+
				"where it went — or it stopped writing prose, and it belongs out of the list", surface)
		}
	}
}

// TestTheProseSurfaceListIsStillTrue is the honesty check on the list above.
//
// The list cannot be derived (see proseSurfaces), so the thing that can go
// wrong is it going STALE — a surface renamed, moved, or retired, leaving the
// gate guarding a path that no longer exists while reporting the same word for
// it, PASS. Asserting each entry still parses as a real prompt site is what
// makes that failure loud.
func TestTheProseSurfaceListIsStillTrue(t *testing.T) {
	for _, surface := range proseSurfaces {
		if !strings.HasSuffix(surface, ".go") {
			t.Errorf("%q is not a Go file; proseSurfaces names the file that BUILDS the prompt", surface)
		}
		if _, err := readVoiceSource(surface); err != nil {
			t.Errorf("proseSurfaces names %s, which cannot be read: %v", surface, err)
		}
	}
}

// voiceGoverned reports whether the shared voice reaches this file's prompt.
//
// Textual over the whole file, for the reason systemCarriesALanguageRule is:
// the System expression is a concatenation of constants built elsewhere in the
// file, and following the value would mean resolving constants across packages.
func voiceGoverned(t *testing.T, surface string) bool {
	t.Helper()
	source, err := readVoiceSource(surface)
	if err != nil {
		t.Fatalf("reading %s: %v", surface, err)
	}
	return strings.Contains(source, "promptvoice.") || strings.Contains(source, voiceRuleHeading)
}

func readVoiceSource(surface string) (string, error) {
	raw, err := os.ReadFile(surface)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
