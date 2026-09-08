// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind reachability H2

package gates

// Every messaging rule set a shipped unit declares is one the boot registers.
//
// The capability census beside this holds that SOME live unit declares
// Messaging. RegisterExtensions' own tests hold that a declared set reaches the
// registry. Neither can see the case between them: an apply loop that registers
// most units and skips one. A synthetic unit in a unit test still lands, so
// both stay green while the shipped German or Vietnamese pack silently does
// not.
//
// The consequence is not a milder default. consent/authorizecap.go resolves an
// unknown country to NO rules, and no rules means no ceiling — so a Vietnamese
// pack that never registers removes the advertising frequency cap and the
// subject prefix outright, on an installation whose unit list still names the
// unit that declared them.
//
// So this reads the apply loop itself: it must register every rule set it
// iterates, with nothing between the range and the call. A filter there is the
// defect, and it is invisible to any test that supplies its own unit.

import (
	"go/ast"
	"go/token"
	"testing"
)

const (
	// applyFile holds the boot's extension reconciliation.
	applyFile = "internal/compose/extensions.go"
	// applyFunc is the reconciliation itself.
	applyFunc = "RegisterExtensions"
	// messagingField is the capability whose apply loop this reads.
	messagingField = "Messaging"
	// messagingRegister is the call that must be reached for every element.
	messagingRegister = "Register"
)

func TestEveryDeclaredMessagingRuleSetIsRegistered(t *testing.T) {
	t.Parallel()

	fset := token.NewFileSet()
	body := functionBodyIn(t, fset, applyFile, applyFunc)

	loops := messagingApplyLoops(body)
	if len(loops) == 0 {
		t.Fatalf("%s ranges over no %s field — the apply this gate reads has moved, and a gate "+
			"that found nothing to judge reports the boot clean", applyFunc, messagingField)
	}
	for _, loop := range loops {
		if registersUnconditionally(loop) {
			continue
		}
		t.Errorf("%s's %s loop does not register every rule set it iterates. A unit whose pack is "+
			"skipped here still appears in the composed set, and the engine resolves its country "+
			"to no rules at all — which is no ceiling rather than a cautious one",
			applyFunc, messagingField)
	}
}

// messagingApplyLoops finds the range statements over a unit's Messaging field.
func messagingApplyLoops(body *ast.BlockStmt) []*ast.RangeStmt {
	var out []*ast.RangeStmt
	ast.Inspect(body, func(n ast.Node) bool {
		loop, isRange := n.(*ast.RangeStmt)
		if !isRange {
			return true
		}
		if sel, isSel := loop.X.(*ast.SelectorExpr); isSel && sel.Sel.Name == messagingField {
			out = append(out, loop)
		}
		return true
	})
	return out
}

// registersUnconditionally reports whether the loop body is the register call
// and nothing else.
//
// Asked as "one statement, and it is the call" rather than by looking for a
// filter. A deny-list of skip shapes would have to name `continue`, an `if`
// around the call, an early `return`, a `break`, and whatever the next author
// writes; the shape that is CORRECT is a single expression statement, and
// everything else is worth a human reading.
func registersUnconditionally(loop *ast.RangeStmt) bool {
	if len(loop.Body.List) != 1 {
		return false
	}
	stmt, isExpr := loop.Body.List[0].(*ast.ExprStmt)
	if !isExpr {
		return false
	}
	call, isCall := stmt.X.(*ast.CallExpr)
	if !isCall {
		return false
	}
	sel, isSel := call.Fun.(*ast.SelectorExpr)
	return isSel && sel.Sel.Name == messagingRegister
}

// functionBodyIn returns one named function's body from one file.
func functionBodyIn(t *testing.T, fset *token.FileSet, path, name string) *ast.BlockStmt {
	t.Helper()
	for _, file := range parseTreeUnder(t, fset, path) {
		for _, decl := range file.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			if isFunc && fn.Name.Name == name && fn.Body != nil {
				return fn.Body
			}
		}
	}
	t.Fatalf("%s declares no %s: the subject this gate reads has moved", path, name)
	return nil
}
