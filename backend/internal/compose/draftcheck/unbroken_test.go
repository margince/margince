// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftcheck_test

// A draft written as one unbroken block.

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/draftcheck"
	"github.com/margince/margince/backend/internal/shared/kernel/convstate"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func rules(body string) []string {
	out := []string{}
	for _, finding := range draftcheck.Body(body, textlang.German, convstate.BandWeeks, true) {
		out = append(out, finding.Rule)
	}
	return out
}

// flagsUnbrokenBlock reports whether the checks fired the unbroken-block rule.
func flagsUnbrokenBlock(body string) bool {
	for _, rule := range rules(body) {
		if rule == "unbroken-block" {
			return true
		}
	}
	return false
}

// The real defect: greeting and message run together as one long line.
//
// This is the body a model returned for a live contact, shortened only of its
// names. Nothing downstream repairs it — the composer renders the breaks it is
// given — so a rep opens a wall of text.
func TestAMessageRunTogetherOnOneLineIsAFinding(t *testing.T) {
	const body = "Greven. Wir haben die Frage über Michael Grodd offen. Das ist kein " +
		"Smalltalk, sondern Business. Der Mann hat Erfahrung, die wir nutzen müssen. " +
		"Hast du mit ihm gesprochen oder liegt das wieder nur auf deinem Schreibtisch? " +
		"Sag mir bis morgen, ob das mit ihm läuft."

	if !flagsUnbrokenBlock(body) {
		t.Error("a whole message on one line was accepted, so the rep opens a wall of text")
	}
}

// A message WITH a blank line is not this defect.
func TestAMessageWithParagraphsIsNotAFinding(t *testing.T) {
	const body = "Greven,\n\nwir haben die Frage über Michael Grodd offen. Der Mann hat " +
		"Erfahrung, die wir nutzen müssen. Hast du mit ihm gesprochen?\n\n" +
		"Sag mir bis morgen, ob das mit ihm läuft."

	if flagsUnbrokenBlock(body) {
		t.Error("a draft with paragraphs was flagged, which would spend a model call making it worse")
	}
}

// A SHORT one-liner is a complete message, not a wall of text.
//
// The rule exists for a block a reader has to wade through. "Passt Donnerstag?"
// wants no paragraph break, and flagging it would earn a retry that can only
// pad it out.
func TestAShortOneLinerIsNotAFinding(t *testing.T) {
	const body = "Greven, passt Donnerstag um 14 Uhr für den Call mit Michael?"

	if flagsUnbrokenBlock(body) {
		t.Error("a short complete one-line message was flagged as a wall of text")
	}
}

// The threshold is a length, and it is checked from both sides.
//
// A rule with only a positive case passes for a checker that flags everything,
// and only a negative case for one that flags nothing. The pair pins the edge
// rather than the direction.
func TestTheThresholdSeparatesANoteFromAWall(t *testing.T) {
	const sentence = "Wir sollten das Thema Michael Grodd endlich abschliessen. "
	short := strings.TrimSpace(strings.Repeat(sentence, 2))
	long := strings.TrimSpace(strings.Repeat(sentence, 4))

	if flagsUnbrokenBlock(short) {
		t.Errorf("a %d-character one-liner was flagged", len([]rune(short)))
	}
	if !flagsUnbrokenBlock(long) {
		t.Errorf("a %d-character one-liner was accepted", len([]rune(long)))
	}
}
