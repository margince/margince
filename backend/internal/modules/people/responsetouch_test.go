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
// without agreeing; it is the FIELD. A second function reading `source` or
// `capturedBy` off a touch is a second opinion about the same rule, whatever
// words it uses to reach it.
//
// `kind` is deliberately not held: `ladderStepFor` reads it to ask whether the
// touch is a meeting, which is a different question about the same column.

// touchType is the struct whose field reads this holds.
const touchType = "leadResponseTouch"

// ownedTouchFields names, per field, the function that owns the rule the field
// carries. A field here holds a rule rather than a value, so reading it IS
// deciding the rule; anyone else wanting the answer calls that function.
var ownedTouchFields = map[string]string{
	"source":     "humanLoggedNote",
	"capturedBy": "humanCaptured",
}

func TestEachRuleBearingTouchFieldHasOneReader(t *testing.T) {
	readers := map[string]map[string]bool{}
	forEachModuleFile(t, func(_ string, _ *token.FileSet, file *ast.File) {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !takesA(fn, touchType) {
				continue
			}
			for field := range fieldReadsIn(fn.Body) {
				if _, owned := ownedTouchFields[field]; !owned {
					continue
				}
				if readers[field] == nil {
					readers[field] = map[string]bool{}
				}
				readers[field][fn.Name.Name] = true
			}
		}
	})
	for field, owner := range ownedTouchFields {
		found := readers[field]
		if len(found) == 0 {
			t.Errorf("nothing reads %s.%s, so the rule %s carries has no subject and this gate "+
				"judged nothing for it", touchType, field, owner)
			continue
		}
		if !found[owner] {
			t.Errorf("%s does not read %s.%s, so it is no longer where that rule lives — this "+
				"gate is now protecting the wrong function", owner, touchType, field)
		}
		if len(found) == 1 {
			continue
		}
		others := make([]string, 0, len(found))
		for name := range found {
			if name != owner {
				others = append(others, name)
			}
		}
		sort.Strings(others)
		t.Errorf("%s.%s is read by %s as well as %s.\n\nThat is a second opinion about the same "+
			"rule: the captured_by grammar and the composer's stamp are things that change, and "+
			"read in two places they change in one. The ladder then shows a lead as contacted "+
			"while the first-response clock counts it unanswered, on one screen. Call %s.",
			touchType, field, strings.Join(others, ", "), owner, owner)
	}
}

// fieldReadsIn returns the field names selected in body, excluding those taken
// by ADDRESS: `&t.capturedBy` in a row Scan fills the field rather than asking
// it anything, and counting it would report the builder as a second reader of
// every rule it populates.
func fieldReadsIn(body *ast.BlockStmt) map[string]bool {
	addressed := map[ast.Node]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		if unary, ok := n.(*ast.UnaryExpr); ok && unary.Op == token.AND {
			addressed[unary.X] = true
		}
		return true
	})
	read := map[string]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok || addressed[ast.Node(sel)] {
			return true
		}
		if _, isIdent := sel.X.(*ast.Ident); isIdent {
			read[sel.Sel.Name] = true
		}
		return true
	})
	return read
}
