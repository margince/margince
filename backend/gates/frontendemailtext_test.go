// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The browser and the server split a mail body by the SAME rules, or a row's
// preview and the message it opens disagree about where the sender stopped
// writing.
//
// There are two implementations on purpose — the server composes the preview
// that rides every activity row, and the viewer folds the tail in the drawer —
// and `activities/emailtext.go` and `frontend/src/format/emailtext.ts` are
// deliberate mirrors of one another. What makes "mirror" true is that both are
// held to ONE table of cases, and this gate is what says so.
//
// It was promised before it existed. The Go table's own header said a case
// added on one side and not the other "is the drift this file and
// gates/frontendemailtext_test.go exist to catch" — and that file was not in
// the tree. A comment claiming a protection nobody wrote is worse than
// silence: the next author greps for the gate, finds the sentence, and stops
// looking. This is that file.
//
// Compared in BOTH directions, because either alone passes over the drift it
// is meant to catch: a Go case with no TypeScript twin means the browser was
// never held to a rule the server follows, and a TypeScript case with no Go
// twin means the reverse.

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

const (
	goEmailTextTable = "internal/modules/activities/emailtext_test.go"
	tsEmailTextTable = "../frontend/src/format/emailtext.test.ts"
)

// The body each side feeds its splitter. Both tables write it as a Go/TS
// double-quoted string with escapes intact, so the literal is comparable
// verbatim without either side being unescaped — and unescaping is where a
// comparison like this usually grows its own bug.
//
// The string body is `(?:[^"\\]|\\.)*` rather than `.*`: a greedy dot runs
// past the closing quote and swallows the rest of the line, so
// `splitEmailBody("x").main).toBe("x")` parsed as ONE body nothing on the
// other side could ever match. Every case then reads as drift, and a gate that
// reports drift everywhere is one a reader learns to ignore.
const quotedString = `"(?:[^"\\]|\\.)*"`

// The TypeScript table writes a body two ways — passed straight in, or bound
// to a `const body` first when the same string is asserted against twice. Both
// are read: a case this pattern cannot see is a case the gate silently agrees
// with, which is the failure direction a census must not have.
var (
	goCaseBody = regexp.MustCompile(`(?m)^\s*body:\s*(` + quotedString + `)\s*,\s*$`)
	tsCaseBody = regexp.MustCompile(
		`splitEmailBody\(\s*(` + quotedString + `)` +
			`|(?m)^\s*const body\s*=\s*\n?\s*(` + quotedString + `)\s*;`)
)

func TestBothEmailSplittersAreHeldToTheSameBodies(t *testing.T) {
	t.Parallel()
	goBodies := bodiesIn(t, goEmailTextTable, goCaseBody)
	tsBodies := bodiesIn(t, tsEmailTextTable, tsCaseBody)

	// A census that can fail short has already failed: a table this cannot
	// parse reads as an empty set, and an empty set agrees with everything.
	if len(goBodies) == 0 {
		t.Fatal("no case body parsed out of the Go table — either it moved or this " +
			"gate's pattern no longer reads it, and an unread table is one this gate agrees with")
	}
	if len(tsBodies) == 0 {
		t.Fatal("no case body parsed out of the TypeScript table — same failure, other side")
	}

	for body := range goBodies {
		if !tsBodies[body] {
			t.Errorf("the Go splitter is held to a body the browser is not:\n  %s\n"+
				"add it to %s, or the two splitters are free to disagree about it",
				body, tsEmailTextTable)
		}
	}
	for body := range tsBodies {
		if !goBodies[body] {
			t.Errorf("the browser's splitter is held to a body the server is not:\n  %s\n"+
				"add it to %s, or the two splitters are free to disagree about it",
				body, goEmailTextTable)
		}
	}
}

// bodiesIn reads the case bodies out of one table.
//
// Comments are stripped first: a body quoted inside prose — "the case
// \"Viele Grüße\" covers this" — would otherwise count as a case that exists,
// and this gate would report parity that the tables do not have.
func bodiesIn(t *testing.T, path string, pattern *regexp.Regexp) map[string]bool {
	t.Helper()
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	text := commentPattern.ReplaceAllString(string(source), "")
	out := map[string]bool{}
	for _, match := range pattern.FindAllStringSubmatch(text, -1) {
		// Whichever alternative matched: the TypeScript pattern has two, and
		// reading only the first would drop every body bound to a const.
		body := ""
		for _, group := range match[1:] {
			if group != "" {
				body = strings.TrimSpace(group)
				break
			}
		}
		// The empty body is a real case on both sides and carries no rule of
		// its own about where a message stops. Skipped rather than compared,
		// because the two tables spell it differently ("" against "   \n\n ")
		// and neither spelling is more correct than the other.
		if len(body) <= 2 {
			continue
		}
		out[body] = true
	}
	return out
}

var commentPattern = regexp.MustCompile(`(?s)//[^\n]*|/\*.*?\*/`)
