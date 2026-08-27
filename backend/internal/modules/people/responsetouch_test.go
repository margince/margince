// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"go/ast"
	"go/token"
	"sort"
	"strings"
	"testing"
)

// §18.1 asks two questions of a touch that look like field reads and are not:
// "did a rep type this into the composer" and "was a person involved at all".
// Each is a rule about the captured_by grammar and the Log composer's stamp,
// and each is asked from two places — the ladder's contact arm and the
// first-response clock. Spelled twice, the two change in one place and the
// stepper then shows a lead as Contacted while the clock goes on counting it as
// unanswered. Both are on the same screen.
//
// So the subject is not the predicate's text, which two readers can respell
// without agreeing; it is the FIELD. A second function reading a rule-bearing
// field off a touch is a second opinion about the same rule, whatever words it
// uses to reach it.
//
// EVERY field is placed, rather than two being named and judged. A gate that
// lists the fields it holds is a second copy of the struct, and it is short
// from the moment a field is added — silently, because it goes on passing over
// the fields it does know. Listing the EXCEPTIONS instead inverts that: a new
// field is held the day it is written, and an exception that stops being true
// fails rather than quietly widening what is allowed.
//
// The owner is derived too — it is whichever function reads the field, and
// there is one. Naming it would fail a rename that moved nothing.

// touchType is the struct whose field reads this holds.
const touchType = "leadResponseTouch"

// touchFieldsReadByMany are the fields more than one function may read, and
// why. Each is asserted to still need its exception below: an exception nobody
// needs is a hole, because it goes on permitting a second reader long after the
// reason for the first one has gone.
//
// gatekit:fixture the exceptions this gate allows, not a set of ratified debts.
// Both entries hold for one reason: the field carries a VALUE, and each reader
// compares it to a different literal to ask a different question. Two questions
// about one fact are not two answers to one rule — which is what `source` and
// `capturedBy` are, where deciding what counts as human-typed is a grammar that
// changes and must change in one place.
var touchFieldsReadByMany = map[string]string{
	"direction": "the way the touch went is a fact on the row: the ladder asks whether it was " +
		"inbound, the clock asks whether it was outbound, and neither is deciding a grammar",
	"kind": "ladderStepFor asks whether the touch is a meeting, which is a different question " +
		"about the same column from the one humanLoggedNote asks",
}

func TestEachRuleBearingTouchFieldHasOneReader(t *testing.T) {
	readers := map[string]map[string]bool{}
	forEachModuleFunc(t, func(_ moduleFile, fn *ast.FuncDecl) {
		if !takesA(fn, touchType) {
			return
		}
		for field := range fieldReadsIn(fn.Body) {
			if readers[field] == nil {
				readers[field] = map[string]bool{}
			}
			readers[field][fn.Name.Name] = true
		}
	})

	read := 0
	for _, field := range fieldsOf(t, touchType) {
		found := readers[field]
		reason, excepted := touchFieldsReadByMany[field]
		if len(found) > 0 {
			read++
		}
		switch {
		case len(found) > 1 && !excepted:
			names := make([]string, 0, len(found))
			for name := range found {
				names = append(names, name)
			}
			sort.Strings(names)
			t.Errorf("%s.%s is read by %s.\n\nThat is a second opinion about the same rule: the "+
				"captured_by grammar and the composer's stamp are things that change, and read "+
				"in two places they change in one. The ladder then shows a lead as contacted "+
				"while the first-response clock counts it unanswered, on one screen. Give the "+
				"field one reader and call it — or add it to touchFieldsReadByMany with the "+
				"reason it carries a value rather than a rule.",
				touchType, field, strings.Join(names, ", "))
		case len(found) <= 1 && excepted:
			// An exception is a statement about the code, and this one has
			// stopped being true. Left in place it goes on permitting a second
			// reader that nobody has asked for yet.
			t.Errorf("%s.%s is listed in touchFieldsReadByMany (%q) but has %d reader(s).\n\n"+
				"The exception is no longer needed, and an exception nobody needs is a hole. "+
				"Remove it.", touchType, field, reason, len(found))
		}
	}
	// A struct whose fields nothing reads means this gate walked a corpus that
	// no longer holds the rule — which reads exactly like a clean one.
	if read == 0 {
		t.Fatalf("nothing in this module reads any field of a %s, so this gate judged nothing "+
			"and the rules it holds have moved", touchType)
	}
}

// fieldReadsIn returns the field names selected in body, excluding those taken
// by ADDRESS: `&t.capturedBy` in a row Scan fills the field rather than asking
// it anything, and counting it would report the builder as a second reader of
// every rule it populates.
func fieldReadsIn(body *ast.BlockStmt) map[string]bool {
	addressed := map[ast.Node]bool{}
	ast.Inspect(body, func(node ast.Node) bool {
		if unary, isUnary := node.(*ast.UnaryExpr); isUnary && unary.Op == token.AND {
			addressed[unary.X] = true
		}
		return true
	})
	read := map[string]bool{}
	ast.Inspect(body, func(node ast.Node) bool {
		sel, isSel := node.(*ast.SelectorExpr)
		if !isSel || addressed[ast.Node(sel)] {
			return true
		}
		if _, isIdent := sel.X.(*ast.Ident); isIdent {
			read[sel.Sel.Name] = true
		}
		return true
	})
	return read
}
