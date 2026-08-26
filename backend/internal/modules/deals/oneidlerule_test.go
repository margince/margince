// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// "Has this deal gone quiet?" is one rule asked at two patiences, and the
// product answers it on two surfaces — in Go for a decision taken at an
// instant, in SQL for a list the database filters.
//
// The threshold-named entry points are meant to carry NO rule of their own:
// each is its windowed sibling with the number filled in. That is what makes
// the stalled bar and the morning queue's shorter window the same question
// rather than two, and it is the half nothing else checks — a hand-inlined
// copy compiles, passes every behaviour test written the day it is made, and
// only disagrees later, when somebody changes one side.
//
// The rule the gate applies is DELEGATION, not text: the body must be a single
// return of a call to the windowed function. A body that grew a second
// condition, or that stopped calling its sibling at all, fails here.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// delegations pairs each threshold-named entry point with the windowed
// function it must be nothing more than a call to.
//
// gatekit:fixture the windowed function each entry point must delegate to — the
// values are the subject of the assertion, not the cost of waiving one.
var delegations = map[string]string{
	"IsStalled":  "IsQuietFor",
	"StalledSQL": "QuietSQL",
}

func TestAThresholdEntryPointCarriesNoRuleOfItsOwn(t *testing.T) {
	const source = "formulas.go"
	file, err := parser.ParseFile(token.NewFileSet(), source, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing %s: %v", source, err)
	}
	found := map[string]bool{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil {
			continue
		}
		want, governed := delegations[fn.Name.Name]
		if !governed {
			continue
		}
		found[fn.Name.Name] = true
		if callee := soleReturnedCall(fn); callee != want {
			t.Errorf("%s is the %s-day spelling of %s and must be a single call to it; its body "+
				"returns %q.\n\n"+
				"A rule spelled here as well as in %s is two answers to \"has this deal gone "+
				"quiet\", and they agree until one of them is edited. Pass the window to %s "+
				"instead.",
				fn.Name.Name, "threshold", want, callee, want, want)
		}
	}
	// The names are written here and the declarations live elsewhere, so a
	// rename empties this test rather than failing it — which reads exactly
	// like a clean tree.
	for name := range delegations {
		if !found[name] {
			t.Errorf("%s is not declared in %s — this gate judged nothing about it, which is "+
				"indistinguishable from it delegating correctly", name, source)
		}
	}
}

// soleReturnedCall names the function a body consists of returning a call to,
// and "" when the body is anything else — several statements, a bare value, or
// a call built from more than a plain identifier.
func soleReturnedCall(fn *ast.FuncDecl) string {
	if fn.Body == nil || len(fn.Body.List) != 1 {
		return ""
	}
	ret, ok := fn.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return ""
	}
	call, ok := ret.Results[0].(*ast.CallExpr)
	if !ok {
		return ""
	}
	callee, ok := call.Fun.(*ast.Ident)
	if !ok {
		return ""
	}
	return callee.Name
}
