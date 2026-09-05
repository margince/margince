// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// What a champion seat IS, held as one definition rather than two that agree
// by inspection.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

// The two statements in championcover.go must ask for a live seat by naming the
// shared constant, never by writing the condition out again.
//
// The failure this forecloses is silent in both directions. A second spelling
// that says LESS makes the withheld probe count an edge the visible read does
// not, so a deal reports a committee it no longer has and `no_champion` stops
// firing for it. A second spelling that says MORE hides a seat the reader could
// have seen. Neither shows up as a test failure anywhere else, because each
// statement is self-consistent — they simply stop answering the same question.
func TestOneSpellingOfALiveChampionSeat(t *testing.T) {
	const file = "championcover.go"
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, file, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", file, err)
	}

	// The condition's own text, read from the constant rather than repeated
	// here: a copy in this test would be the third spelling, and the assertion
	// below would then pass while the code drifted away from both.
	var condition string
	var declaration token.Pos
	ast.Inspect(parsed, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok || len(spec.Names) != 1 || spec.Names[0].Name != "livePersonSeat" {
			return true
		}
		lit, isLit := spec.Values[0].(*ast.BasicLit)
		if !isLit {
			return false
		}
		text, unquoteErr := strconv.Unquote(lit.Value)
		if unquoteErr != nil {
			t.Fatalf("reading livePersonSeat: %v", unquoteErr)
		}
		condition, declaration = text, lit.Pos()
		return false
	})
	if condition == "" {
		t.Fatal("livePersonSeat is gone or is no longer a string literal; " +
			"the two statements in championcover.go no longer share a definition of a live seat")
	}

	// Every SQL literal in the file, checked for the condition written out by
	// hand. The constant reaches a statement as a %s verb, so a literal holding
	// the text itself is the second spelling this test exists to refuse.
	ast.Inspect(parsed, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		// The declaration itself is the one spelling, not a second one.
		if lit.Pos() == declaration || !strings.Contains(lit.Value, "archived_at IS NULL") {
			return true
		}
		if strings.Contains(lit.Value, condition) {
			t.Errorf("%s writes the live-seat condition out at %s instead of naming "+
				"livePersonSeat; two spellings drift and the two statements stop "+
				"agreeing about what a seat is", file, fset.Position(lit.Pos()))
		}
		return true
	})
}
