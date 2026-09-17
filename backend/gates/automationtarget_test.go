// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// An automation action targets the record its trigger fired on.
//
// The audience gate (modules/automation/audiencegate.go) reads ONE thing: the
// trigger's subject, `ev.Entity`, when it is an activity. It never reads an
// action's own Target. The two can differ by design — gate.go's target-scoped
// arm says so — and today none does: every shipped handler passes the event's
// own subject straight through.
//
// That was a reading of the handlers at the time of writing, and the comment
// said as much: "Nothing holds that, unlike the sibling below." A handler that
// targeted some other activity would ask the audience gate about a row it never
// checked the visibility of — evidence fanned into a task with no check at all
// — and would arrive silently. This is the check the comment was promised.
//
// DERIVED FROM THE SHAPE, not from a list of handlers. What it asserts is that
// the Target is taken from the EVENT — any `*.Entity` selector — rather than
// composed from something else, so a handler added in a module nobody thought
// of is covered the day it is written. Pinning the variable name `ev` would
// have been a second copy of the handlers' own spelling.
//
// WHAT IT CANNOT SEE. A Target assigned in a statement rather than written in
// the literal, and a `.Entity` read off something that is not the event. Both
// are reachable; neither is what a handler looks like today, and the floor
// below is what makes this scan's silence noticeable rather than reassuring.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// actionProducerRoots are the trees a workflow handler can be written in.
var actionProducerRoots = []string{"internal/modules", "internal/compose"}

// actionLiteralFloor is set below the live count of action literals. A scan that
// matched none would report PASS having judged nothing, which is what a tree
// with no diverging handler also looks like.
const actionLiteralFloor = 4

func TestNoHandlerTargetsAnActivityAwayFromItsTrigger(t *testing.T) {
	t.Parallel()
	seen := 0
	for _, root := range actionProducerRoots {
		fset := token.NewFileSet()
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") ||
				strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "_gen.go") {
				return err
			}
			file, perr := parser.ParseFile(fset, path, nil, 0)
			if perr != nil {
				return perr
			}
			ast.Inspect(file, func(n ast.Node) bool {
				literal, ok := n.(*ast.CompositeLit)
				if !ok || !isWorkflowAction(literal) {
					return true
				}
				seen++
				target, named := targetOf(literal)
				if !named {
					// No Target at all is the zero ref, which the gate reads as
					// no activity — not this gate's finding.
					return true
				}
				if sel, ok := target.(*ast.SelectorExpr); ok && sel.Sel.Name == "Entity" {
					return true
				}
				t.Errorf("%s: a workflow action's Target is composed rather than taken from the "+
					"event's own subject. The audience gate reads ev.Entity and never an action's "+
					"Target, so an activity named here is one whose visibility nothing checked — "+
					"extend the gate before shipping a handler that diverges",
					fset.Position(literal.Pos()))
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}
	if seen < actionLiteralFloor {
		t.Fatalf("found %d workflow action literal(s) across %v, want at least %d — the shape has "+
			"changed and this gate is reading a tree it no longer recognises",
			seen, actionProducerRoots, actionLiteralFloor)
	}
}

// isWorkflowAction reports whether a literal builds a workflow.Action. The
// literals in the tree are elided inside `[]workflow.Action{{…}}`, so the type
// is often absent and the KEYS are the evidence: an action is the only shape
// carrying a Kind whose value names a workflow action verb.
func isWorkflowAction(literal *ast.CompositeLit) bool {
	if sel, ok := literal.Type.(*ast.SelectorExpr); ok {
		return sel.Sel.Name == "Action"
	}
	if literal.Type != nil {
		return false
	}
	for _, element := range literal.Elts {
		pair, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if key, ok := pair.Key.(*ast.Ident); !ok || key.Name != "Kind" {
			continue
		}
		verb, ok := pair.Value.(*ast.SelectorExpr)
		return ok && strings.HasPrefix(verb.Sel.Name, "Action")
	}
	return false
}

// targetOf answers the Target field's expression, and whether it was named.
func targetOf(literal *ast.CompositeLit) (ast.Expr, bool) {
	for _, element := range literal.Elts {
		pair, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if key, ok := pair.Key.(*ast.Ident); ok && key.Name == "Target" {
			return pair.Value, true
		}
	}
	return nil, false
}
