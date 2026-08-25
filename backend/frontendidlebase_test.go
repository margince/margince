// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package backendarch

// The deal board and the server must measure silence from the SAME timestamp,
// or a card ages differently from the list that filed the deal stalled.
//
// Both sides fall back from the newest activity to the creation instant, and
// the ORDER of that fallback is the whole rule while being the one part a
// reader cannot see: created_at is never absent, so a fallback written the
// other way round answers "created" for every record and looks identical on
// the page. A board that took the creation date would show a deal touched
// yesterday as months old, beside a list that calls it live.
//
// Neither side can call the other across the wire, so the TypeScript is a
// declared mirror rather than a second answer — and this is what makes
// "mirror" true. It reads the ORDER out of both spellings and compares them:
// reordering the Go SQL fails here, and so does reordering the TypeScript.
//
// The Go side is READ FROM idlebase.SQL rather than written down, so this gate
// cannot agree with a stale copy of the rule it is checking.

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/gradionhq/margince/backend/internal/shared/kernel/idlebase"
)

const frontendIdleBase = "../frontend/src/format/idlebase.ts"

// tsIdleFallback reads the two properties either side of the nullish
// coalescing in the frontend helper.
//
// It requires `??` specifically. `||` would also compile and would also read
// correctly for a timestamp, but it is a different operator with a different
// rule, and a gate that quietly accepted both would be agreeing with a change
// nobody reviewed. A spelling this pattern cannot read fails the test rather
// than passing it, which is the direction a parity check must fail in.
var tsIdleFallback = regexp.MustCompile(`\.\s*([a-z_]+)\s*\?\?\s*[A-Za-z_$]+\s*\.\s*([a-z_]+)`)

// tsComment strips comments before the expression is read, so prose naming a
// column cannot stand in for the code that reads it.
var tsIdleComment = regexp.MustCompile(`(?s)//[^\n]*|/\*.*?\*/`)

func TestTheDealBoardMeasuresIdleTheWayTheServerDoes(t *testing.T) {
	source, err := os.ReadFile(frontendIdleBase)
	if err != nil {
		t.Fatalf("reading the frontend idle-base helper: %v", err)
	}
	body := tsIdleComment.ReplaceAllString(string(source), " ")
	match := tsIdleFallback.FindStringSubmatch(body)
	if match == nil {
		t.Fatalf("%s no longer spells the fallback as `record.<column> ?? record.<column>` — this gate is "+
			"reading a shape that is gone, and a parity check that cannot find its subject agrees with "+
			"anything", frontendIdleBase)
	}
	inBrowser := []string{match[1], match[2]}

	onServer := idleBaseColumnOrder(t)
	if len(inBrowser) != len(onServer) {
		t.Fatalf("the browser falls back through %d column(s) and the server through %d", len(inBrowser), len(onServer))
	}
	for i := range onServer {
		if inBrowser[i] != onServer[i] {
			t.Fatalf("the browser measures idle from %v and the server from %v. The two disagree at position "+
				"%d, so a deal's age on the board and its stalled flag in the list are answering different "+
				"questions about the same row", inBrowser, onServer, i)
		}
	}
}

// idleBaseColumnOrder reads the server's order out of the expression the
// server actually emits, rather than restating it here.
func idleBaseColumnOrder(t *testing.T) []string {
	t.Helper()
	expression := idlebase.SQL("")
	open := strings.Index(expression, "(")
	closed := strings.LastIndex(expression, ")")
	if open < 0 || closed < open {
		t.Fatalf("idlebase.SQL no longer renders a call with arguments (%q), so its order cannot be read", expression)
	}
	var columns []string
	for _, argument := range strings.Split(expression[open+1:closed], ",") {
		columns = append(columns, strings.TrimSpace(argument))
	}
	if len(columns) < 2 {
		t.Fatalf("idlebase.SQL renders %q, which has no fallback to compare", expression)
	}
	return columns
}
