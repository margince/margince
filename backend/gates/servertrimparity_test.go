// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The browser and the server must agree on which values are EMPTY, or a form
// refuses what the save would have taken.
//
// Two agreements exist here, one character apart. `activities.subjectSpace` is
// `unicode.IsSpace` plus the byte-order mark, for an email subject;
// `strings.TrimSpace` is `unicode.IsSpace` alone, everywhere else. Only the
// first was held by anything — replysubjects.txt, read by both sides — and the
// second had already been got wrong: a project-health form spelled the SUBJECT
// set against a handler using plain TrimSpace, so it refused a note the server
// would have accepted, and nothing failed.
//
// What makes the two hard to keep together is that neither language's own
// spelling is either of them. ECMAScript's `\s` cuts U+FEFF and leaves U+0085
// standing; `unicode.IsSpace` does exactly the opposite. So the TypeScript is
// written against `\p{White_Space}` — which IS `unicode.IsSpace`, rune for rune
// — and this is what makes "mirror" more than a claim in a comment.
//
// WHY A CORPUS AND NOT A TABLE. The sibling money gate reads the TypeScript
// table and compares it entry for entry, because a table is data. A trim is an
// algorithm, and the only thing both sides can be asked the same question about
// is what they DO to a string. So the corpus is the subject, and the two sides
// meet on it: this test writes what Go answers, servertrim.test.ts asserts the
// browser answers the same, and neither can be corrected alone.
//
// The corpus being GENERATED is what makes the expectations true rather than
// somebody's reading of the documentation. What it cannot do is notice a case
// nobody thought of, so the coverage test below derives the characters that
// must appear from `unicode.IsSpace` itself — a corpus quietly shrunk to make a
// failure go away fails there instead.

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"
	"unicode"
)

var updateServerTrim = flag.Bool("update-server-trim", false,
	"rewrite gates/testdata/servertrim.json from what strings.TrimSpace answers")

const serverTrimCorpus = "gates/testdata/servertrim.json"

// trimCase is one question both sides are asked.
type trimCase struct {
	In   string `json:"in"`
	Want string `json:"want"`
}

// nearMisses are the characters the two languages DISAGREE about, and the
// reason this gate exists at all. Each is named rather than derived because
// what makes it interesting is precisely that `unicode.IsSpace` does NOT say
// what ECMAScript says about it.
//
// Written as escapes: every one of them is invisible, and a reviewer cannot
// check a character they cannot see.
var nearMisses = []rune{
	'\u0085', // NEXT LINE: White_Space, and ECMAScript's `\s` leaves it standing.
	'\u00A0', // NO-BREAK SPACE: both cut it, which is worth pinning too.
	'\uFEFF', // ZWNBSP: `\s` cuts it, IsSpace does not — the one-character gap.
	'\u180E', // MONGOLIAN VOWEL SEPARATOR: White_Space once, and not since 6.3.
}

// trimCorpus builds the questions both sides are asked, from what Go itself
// calls a space rather than from a list somebody maintains. The coverage test
// below is what holds it to reaching them all.
func trimCorpus() []trimCase {
	var subjects []rune
	for r := rune(0); r < 0x10000; r++ {
		if unicode.IsSpace(r) {
			subjects = append(subjects, r)
		}
	}
	subjects = append(subjects, nearMisses...)

	cases := []trimCase{{In: ""}, {In: "plain"}}
	for _, r := range subjects {
		// Leading, trailing, both, and doubled — a trim that walked one
		// character instead of a run would pass the single cases.
		for _, shape := range []string{
			"%[1]cvalue", "value%[1]c", "%[1]cvalue%[1]c", "%[1]c%[1]cvalue%[1]c%[1]c",
		} {
			cases = append(cases, trimCase{In: fmt.Sprintf(shape, r)})
		}
		// The character INSIDE the value, which must survive: a trim that used
		// a replace rather than an edge walk would take it.
		cases = append(cases, trimCase{In: fmt.Sprintf("a%cb", r)})
		// A value that is nothing but the character, which is the empty answer
		// every "was this filled in?" check turns on.
		cases = append(cases, trimCase{In: string(r)})
	}
	// Astral text, so a walk over UTF-16 units cannot split a surrogate pair
	// and mistake half of one for a space.
	cases = append(cases, trimCase{In: " \U0001F600 "}, trimCase{In: "\U0001F600"})
	for i := range cases {
		cases[i].Want = strings.TrimSpace(cases[i].In)
	}
	return cases
}

func TestTheServerTrimCorpusIsWhatGoAnswers(t *testing.T) {
	t.Parallel()
	cases := trimCorpus()
	// Indented and newline-terminated so a diff of this file reads as a diff of
	// cases rather than of one line.
	rendered, err := json.MarshalIndent(cases, "", "  ")
	if err != nil {
		t.Fatalf("rendering the trim corpus: %v", err)
	}
	rendered = append(rendered, '\n')

	if *updateServerTrim {
		if err := os.WriteFile(serverTrimCorpus, rendered, 0o600); err != nil {
			t.Fatalf("writing %s: %v", serverTrimCorpus, err)
		}
		t.Logf("rewrote %s (%d cases)", serverTrimCorpus, len(cases))
		return
	}

	committed, err := os.ReadFile(serverTrimCorpus)
	if err != nil {
		t.Fatalf("reading %s: %v — regenerate it with -update-server-trim", serverTrimCorpus, err)
	}
	if string(committed) != string(rendered) {
		t.Errorf("%s no longer says what strings.TrimSpace answers.\n\tRegenerate it with:\n"+
			"\t  go test ./gates -run TestTheServerTrimCorpus -update-server-trim\n"+
			"\tand commit it with the change that moved it — frontend/src/format/servertrim.test.ts "+
			"reads this same file, so the browser's mirror is checked against it in the same push.",
			serverTrimCorpus)
	}
}

// The corpus covers every character Go calls a space, and the ones it pointedly
// does not.
//
// Separate from the test above on purpose. That one asks whether the committed
// file matches what Go answers TODAY, and a corpus trimmed down to two cases
// would satisfy it perfectly. This asks whether the corpus still reaches the
// characters the disagreement lives in — the direction a parity gate fails in
// without failing.
func TestTheServerTrimCorpusReachesEveryCharacterGoCallsASpace(t *testing.T) {
	t.Parallel()
	committed, err := os.ReadFile(serverTrimCorpus)
	if err != nil {
		t.Fatalf("reading %s: %v", serverTrimCorpus, err)
	}
	var cases []trimCase
	if err := json.Unmarshal(committed, &cases); err != nil {
		t.Fatalf("parsing %s: %v", serverTrimCorpus, err)
	}
	seen := map[rune]bool{}
	for _, one := range cases {
		for _, r := range one.In {
			seen[r] = true
		}
	}
	var missing []string
	for r := rune(0); r < 0x10000; r++ {
		if unicode.IsSpace(r) && !seen[r] {
			missing = append(missing, fmt.Sprintf("U+%04X", r))
		}
	}
	for _, r := range nearMisses {
		if !seen[r] {
			missing = append(missing, fmt.Sprintf("U+%04X (a near miss, named for the disagreement)", r))
		}
	}
	if len(missing) > 0 {
		t.Errorf("%s asks nothing about %s.\n\tEvery character unicode.IsSpace names has to be in the "+
			"corpus, or the browser's mirror is unchecked exactly where the two languages disagree.",
			serverTrimCorpus, strings.Join(missing, ", "))
	}
}
