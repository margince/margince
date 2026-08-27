// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind reachability H2

package gates

// The weekly's outbound send must not sit inside the loop that measures reps.
//
// This is a SOURCE check because the behaviour it protects cannot be reached
// from a test: measureWorkspace resolves every rep through EffectiveAuthority,
// and the shared integration harness seeds no role grants, so every seat
// declines before either arrangement of the code would send. A runtime test
// written against it passes identically whether the send is inside the loop or
// outside it — which is worse than no test, because it reads as protection.
//
// What is at stake is not the mail. The workspace job has ten minutes, one
// send may take 45 seconds, and the loop is serial: a stalled relay inside it
// spends the budget on the first reps, and everyone after them loses THE
// REVIEW — the counts and the deal lines. A rep skipped that way is skipped
// for good, because the candidate query only ever asks about the week that
// just closed.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

const weeklyJobsFile = "internal/compose/weeklyjobs.go"

// TestTheWeeklySendIsNotInsideTheMeasuringLoop fails if mailWeekly is called
// from measureFor, or from within any loop body in measureWorkspace that also
// measures.
func TestTheWeeklySendIsNotInsideTheMeasuringLoop(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, weeklyJobsFile, nil, 0)
	if err != nil {
		t.Fatalf("reading %s: %v", weeklyJobsFile, err)
	}

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		switch fn.Name.Name {
		case "measureFor":
			if callsMailWeekly(fn.Body) {
				t.Errorf("%s: measureFor sends the mail itself. It must hand the review back "+
					"so every rep is measured before any relay is dialled — otherwise one "+
					"stalled relay costs the reps after this one their review, not just their mail",
					weeklyJobsFile)
			}
		case "measureWorkspace":
			assertSendsAfterMeasuring(t, fn.Body)
		}
	}
}

// assertSendsAfterMeasuring fails if one loop body both measures and sends.
func assertSendsAfterMeasuring(t *testing.T, body *ast.BlockStmt) {
	t.Helper()
	ast.Inspect(body, func(n ast.Node) bool {
		loop, ok := n.(*ast.RangeStmt)
		if !ok || loop.Body == nil {
			return true
		}
		if callsMailWeekly(loop.Body) && callsNamed(loop.Body, "measureFor") {
			t.Errorf("%s: one loop in measureWorkspace both measures a rep and mails them. "+
				"Measuring must finish for every rep before the first relay is dialled",
				weeklyJobsFile)
		}
		return true
	})
}

func callsMailWeekly(body *ast.BlockStmt) bool { return callsNamed(body, "mailWeekly") }

// callsNamed reports whether the block calls a method of that name on anything.
func callsNamed(body *ast.BlockStmt, name string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == name {
			found = true
			return false
		}
		return true
	})
	return found
}

// The gate is worthless if it reads the wrong file, and a rename would leave it
// green over nothing. This is the under-recognition failure the rulebook names:
// a census that can fail short has already failed.
func TestTheOrderGateReadsTheRealFile(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, weeklyJobsFile, nil, 0)
	if err != nil {
		t.Fatalf("the weekly job file moved; this gate now reads nothing: %v", err)
	}
	var names []string
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			names = append(names, fn.Name.Name)
		}
	}
	for _, required := range []string{"measureWorkspace", "measureFor"} {
		if !declaresFunc(names, required) {
			t.Fatalf("%s no longer declares %s, so the order check above matched nothing. "+
				"Found: %s", weeklyJobsFile, required, strings.Join(names, ", "))
		}
	}
}

func declaresFunc(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
