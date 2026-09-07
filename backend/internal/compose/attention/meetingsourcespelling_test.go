// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The two meeting source words are spelled once, and this is what holds it.
//
// The failure they would otherwise have is silent in the direction that matters.
// scope.go compares a row's Source against these words to decide that the
// meeting lanes already answered the ownership question in their own query. A
// second spelling — a literal "meeting" typed into that comparison, or a renamed
// constant read by only one side — makes the comparison simply never match: no
// error, no panic, just a scope filter re-judging rows it must not, and an
// invited colleague's meetings vanishing from their own page with nothing to say
// why.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// The wire words themselves, repeated here on purpose.
//
// A gate that read the constants it protects would agree with them by
// construction and could never fail — it would pass just as happily after a
// rename that left the other side behind. These are the strings the CONTRACT
// declares (crm.yaml's AttentionItem.source enum), so a change to either
// constant has to come here and meet the contract's own spelling.
const (
	wireMeeting        = "meeting"
	wireMeetingOutcome = "meeting_outcome"
)

// The constants carry the words the contract declares.
func TestTheMeetingSourceConstantsMatchTheWire(t *testing.T) {
	t.Parallel()

	if sourceMeeting != wireMeeting {
		t.Errorf("sourceMeeting = %q, want %q — the contract's AttentionItem.source enum spells it that way, "+
			"and a row whose source does not match reaches no scope filter that knows about it",
			sourceMeeting, wireMeeting)
	}
	if sourceMeetingOutcome != wireMeetingOutcome {
		t.Errorf("sourceMeetingOutcome = %q, want %q — same reason", sourceMeetingOutcome, wireMeetingOutcome)
	}
}

// Nothing in this package types either word as a bare literal.
//
// The one place each may appear is its own const declaration, checked above.
// Everywhere else must go through the constant, or the two spellings drift and
// the comparison in scope.go stops matching rows it is supposed to keep.
func TestTheMeetingSourceWordsAreNotRetyped(t *testing.T) {
	t.Parallel()

	watched := map[string]string{
		wireMeeting:        "sourceMeeting",
		wireMeetingOutcome: "sourceMeetingOutcome",
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the package directory: %v", err)
	}
	fset := token.NewFileSet()
	inspected := 0
	for _, entry := range entries {
		name := entry.Name()
		// meeting.go is where the constants are declared.
		//
		// Tests are skipped, and the skip is not a hole: a fixture BUILDS a row
		// whose source is the wire word, which is what the contract says rather
		// than a second spelling of the rule. What this gate protects is the
		// production comparison, and a test typing the word as data cannot make
		// scope.go stop matching.
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || name == "meeting.go" {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		inspected++
		ast.Inspect(file, func(n ast.Node) bool {
			expr, isExpr := n.(ast.Expr)
			if !isExpr {
				return true
			}
			// The DECODED string, never lit.Value: the raw source text carries
			// its own quotes and escapes, so a census reading that compares the
			// wrong thing and reports a clean tree over the shape it looks for.
			text, isLiteral := gatekit.LiteralText(expr)
			if !isLiteral {
				return true
			}
			if constant, watchedWord := watched[text]; watchedWord {
				t.Errorf("%s:%d types %q as a literal — use %s, or the scope filter and the row "+
					"drift apart and an invited colleague's meetings disappear from their own page",
					name, fset.Position(expr.Pos()).Line, text, constant)
			}
			return true
		})
	}
	// A walk that reached nothing reports PASS exactly like a clean tree.
	if inspected == 0 {
		t.Fatal("this gate inspected no production file — it would report a clean package over any number of retyped literals")
	}
}
