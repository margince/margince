// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// A Brief count and the door beneath it must exclude the SAME rows.
//
// Brief draws its decisions as cards in a deck, and the feed below counts only
// what the deck has not answered. The link under that count opens the worklist
// narrowed to `except_decisions`, which the server evaluates. So one exclusion
// is spelled twice — `brief.sentence.ts` decides the count, `page.go` decides
// the door — and the two disagreeing is the exact defect the filter was added to
// remove, reappearing where nobody looks for it.
//
// This is not hypothetical. The obvious server-side reading was category
// `decisions`, which is a WIDER set than the client's source test: an
// introduction request classifies as a decision and is not an approval, so the
// door dropped a row the count had included. That version passed every unit test
// on both sides, because each side is self-consistent.
//
// Both directions, like the minor-units mirror: a source added to one side and
// not the other fails.

import (
	"os"
	"regexp"
	"testing"
)

const (
	briefDeckClient = "../frontend/src/screens/brief.sentence.ts"
	briefDeckServer = "internal/compose/attention/page.go"
)

// The client's constant: `const DECK_ANSWERS = "approval";`
var clientDeckAnswers = regexp.MustCompile(
	`DECK_ANSWERS\s*=\s*"([a-z_]+)"`)

// The server's: `const deckAnswers = crmcontracts.WorklistItemSource("approval")`
var serverDeckAnswers = regexp.MustCompile(
	`deckAnswers\s*=\s*crmcontracts\.WorklistItemSource\("([a-z_]+)"\)`)

// TestTheDeckExclusionIsSpelledTheSameOnBothSides holds the pair.
func TestTheDeckExclusionIsSpelledTheSameOnBothSides(t *testing.T) {
	t.Parallel()

	client := onlyMatch(t, briefDeckClient, clientDeckAnswers,
		"DECK_ANSWERS in brief.sentence.ts")
	server := onlyMatch(t, briefDeckServer, serverDeckAnswers,
		"deckAnswers in page.go")

	if client != server {
		t.Fatalf("the count excludes source %q and the door excludes %q — a rep "+
			"told \"N more\" would land on a list holding a different number",
			client, server)
	}
}

// onlyMatch reads exactly one capture out of a file.
//
// Exactly one, because both halves of a failure are silent otherwise: no match
// means the constant was renamed and this gate has stopped reading its subject,
// and two matches mean one side grew a second spelling that this comparison
// would then pick between arbitrarily.
func onlyMatch(t *testing.T, path string, pattern *regexp.Regexp, what string) string {
	t.Helper()

	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	found := pattern.FindAllStringSubmatch(string(source), -1)
	if len(found) != 1 {
		t.Fatalf("found %d spellings of %s in %s, wanted exactly one: this gate "+
			"reads that constant, so none means it has stopped watching its "+
			"subject and two means the subject has forked",
			len(found), what, path)
	}
	return found[0][1]
}
