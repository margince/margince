// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// No test in the merge gate decides anything by reading a stopwatch.
//
// `make check` runs on a shared, noisy CI runner and on whatever laptop is
// free. A test that measures elapsed wall time and fails past a threshold
// answers one thing on an idle box and another on a busy one, so its verdict
// is about the machine rather than about the change — and `TestTheCJKStripIsBounded`
// did exactly that on a branch touching neither the strip nor anything it
// reads, because several lanes were in flight. The rulebook's P3 says the same:
// no real-clock flakiness.
//
// The damage is not the red run. It is that a gate which reds at random teaches
// the next reader to re-run rather than to read, and that habit is what lets a
// real failure through.
//
// WHAT TO WRITE INSTEAD. Every one of these tests was protecting a bound on
// WORK — a loop cap, a truncation, a fail-open ceiling — and each had an exact
// statement available that answers the same on any machine: count what the cap
// let through, compare the capped answer against the answer for the capped
// input, assert the corpus came back untouched. Three such rewrites landed with
// this gate, and each is strictly more precise than the clock it replaced: two
// of them caught a loosened bound that the old ceilings had passed.
//
// WHAT THIS DOES NOT PROHIBIT. Reading the clock — a test may take `time.Now()`
// for a fixture instant, and asserting that a production clock IS wall time is
// a claim about identity, not a budget. The subject is narrower: a COMPARISON
// whose operand is an elapsed duration. That is what makes a verdict depend on
// how busy the box is.
//
// The subject is anchored on a FRESHLY READ CLOCK — `time.Since(start)`, or
// `time.Now().Sub(start)`, which is the same quantity spelled the other way. A
// bare `a.Sub(b)` is not: two fixture instants subtracted is a fact about the
// data, and the offline demo's history span is exactly that. A gate that read
// every subtraction would report it, be waived, and teach the next author that
// waiving is the answer.
//
// WHAT IT CANNOT SEE. An elapsed duration carried further than the `if` that
// reads it, a budget crossed inside a helper this gate reads as an ordinary
// call, or a `context.WithTimeout` whose expiry is the assertion. It is a net
// under the shape the tree reaches for rather than a proof — so it matches
// STATEMENTS through the parser, never lines, and it carries no skip-list in
// front of the walk. The integration lane is out of scope because it owns its
// own database and is not the merge gate.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// clockBoundsInTheMergeGate ratifies a comparison that reads elapsed time and
// is nonetheless not a budget.
var clockBoundsInTheMergeGate = gatekit.Waive(map[string]string{
	"internal/modules/approvals/service_test.go:TestNewServiceDefaultsTheClock": "asks whether the " +
		"default clock IS wall time, which is a claim about the clock's identity rather than about " +
		"how long anything took. The tolerance is a minute against a drift that would be years, so " +
		"no load on any machine can change the answer.",
})

// isTimeCall reports whether expr is `time.<name>()`.
func isTimeCall(expr ast.Expr, name string) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != name {
		return false
	}
	pkg, isIdent := sel.X.(*ast.Ident)
	return isIdent && pkg.Name == "time"
}

// isElapsedDuration reports whether expr names time already spent, measured
// against a clock this process just read. `time.Since(start)` and
// `time.Now().Sub(start)` are the same quantity written two ways, and a gate
// that knew only one spelling would let the other through — the direction a
// census must not fail in.
//
// A subtraction of two instants that are NOT a fresh clock reading is a fact
// about the data rather than about the machine, and is deliberately not this.
func isElapsedDuration(expr ast.Expr) bool {
	if isTimeCall(expr, "Since") {
		return true
	}
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Sub" || len(call.Args) != 1 {
		return false
	}
	return isTimeCall(sel.X, "Now") || isTimeCall(call.Args[0], "Now")
}

// elapsedName is the identifier an `if` binds a fresh elapsed duration to in
// its own init, or "" for an `if` that binds none.
//
// `if d := time.Since(start); d > budget` is how this is actually written, and
// a gate reading only the comparison sees two identifiers and clears it. The
// binding is followed exactly this far and no further: an assignment the
// statement itself carries is unambiguous, where a variable traced across a
// function would need the types the parser alone does not have.
func elapsedName(init ast.Stmt) string {
	assign, ok := init.(*ast.AssignStmt)
	if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 || !isElapsedDuration(assign.Rhs[0]) {
		return ""
	}
	name, ok := assign.Lhs[0].(*ast.Ident)
	if !ok {
		return ""
	}
	return name.Name
}

// comparesAnElapsedDuration reports whether this comparison puts elapsed time
// on one side of an ordering operator. Equality is not a budget: `>` and `<`
// are what turn a measurement into a verdict.
func comparesAnElapsedDuration(n ast.Node, bound string) bool {
	cmp, ok := n.(*ast.BinaryExpr)
	if !ok {
		return false
	}
	switch cmp.Op {
	case token.GTR, token.GEQ, token.LSS, token.LEQ:
	default:
		return false
	}
	return isElapsedDuration(cmp.X) || isElapsedDuration(cmp.Y) ||
		namesIdent(cmp.X, bound) || namesIdent(cmp.Y, bound)
}

// namesIdent reports whether expr is the identifier an enclosing `if` bound to
// a fresh elapsed duration. Never true for the empty name, so an `if` that
// bound nothing matches nothing.
func namesIdent(expr ast.Expr, name string) bool {
	if name == "" {
		return false
	}
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == name
}

// enclosingTest is the Test function a node sits in, so a finding names the
// test a reader must open rather than only a line.
func enclosingTest(file *ast.File, pos token.Pos) string {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		if fn.Pos() <= pos && pos <= fn.End() {
			return fn.Name.Name
		}
	}
	return ""
}

func TestNoMergeGateTestDecidesByTheClock(t *testing.T) {
	t.Parallel()
	defer clockBoundsInTheMergeGate.AssertAllMatched(t)

	fset := token.NewFileSet()
	// Counted so the gate cannot pass by reading nothing. A walk that found no
	// test files at all — a moved tree, a changed suffix — reports the same
	// silence as a clean one, and silence is the one answer a census may not
	// give.
	swept := 0
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return walkErr
		}
		path = filepath.ToSlash(path)
		if isIntegrationTagged(path) {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			// The error IS the answer: a file this gate cannot parse is a file
			// it cannot clear, and reporting OK over it is how a census fails
			// short.
			return fmt.Errorf("parsing %s: %w", path, parseErr)
		}
		swept++
		// The name an enclosing `if` bound, carried down its own subtree so
		// the comparison in its condition can be read against it.
		bound := ""
		ast.Inspect(file, func(n ast.Node) bool {
			if ifStmt, isIf := n.(*ast.IfStmt); isIf && ifStmt.Init != nil {
				if named := elapsedName(ifStmt.Init); named != "" {
					bound = named
				}
			}
			if !comparesAnElapsedDuration(n, bound) {
				return true
			}
			where := enclosingTest(file, n.Pos())
			if clockBoundsInTheMergeGate.Waived(t, path+":"+where) {
				return true
			}
			t.Errorf("%s:%d: %s decides by how long something took. `make check` runs on a shared "+
				"runner and on whatever laptop is free, so this verdict is about the machine rather "+
				"than about the change, and a gate that reds at random teaches the next reader to "+
				"re-run instead of to read.\nAssert the WORK the bound permits — what the cap let "+
				"through, the capped answer against the answer for the capped input, the corpus "+
				"handed back untouched — or ratify it in clockBoundsInTheMergeGate with the reason "+
				"no load can change the answer.",
				path, fset.Position(n.Pos()).Line, where)
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("sweeping the merge gate's tests: %v", err)
	}
	if swept == 0 {
		t.Error("the sweep read no test files at all, so its PASS means nothing")
	}
}

// The detector, read against every spelling it claims to see and every one it
// claims not to.
//
// The sweep above passes over a clean tree, and a detector that had quietly
// stopped matching would pass in exactly the same way — silence reading as
// health is the failure a census cannot notice about itself. These cases fail
// instead. Each negative is a shape that really is in the tree: the offline
// demo subtracts two fixture instants to measure a generated history, and a
// gate that reported it would be waived into uselessness.
func TestTheClockBoundDetectorSeesEverySpelling(t *testing.T) {
	t.Parallel()
	for _, k := range []struct {
		name    string
		body    string
		wantHit bool
	}{
		{
			"time.Since in an if-init, the way it is written",
			"if d := time.Since(start); d > time.Second { panic(1) }", true,
		},
		{
			"time.Since compared in place",
			"if time.Since(start) > time.Second { panic(1) }", true,
		},
		{
			"the same quantity spelled as a subtraction from now",
			"if time.Now().Sub(start) > time.Second { panic(1) }", true,
		},
		{
			"the comparison written the other way round",
			"if time.Second < time.Since(start) { panic(1) }", true,
		},
		{
			"two fixture instants, which is a fact about the data",
			"if newest.Sub(oldest) < time.Hour { panic(1) }", false,
		},
		{
			"an elapsed duration compared for equality, which is no budget",
			"if time.Since(start) == 0 { panic(1) }", false,
		},
		{
			"an if-init binding something that is not elapsed time",
			"if d := deadline.Sub(issued); d > time.Second { panic(1) }", false,
		},
	} {
		t.Run(k.name, func(t *testing.T) {
			t.Parallel()
			src := "package p\nimport \"time\"\nfunc f(start, newest, oldest, deadline, issued time.Time) {\n" +
				k.body + "\n}\n"
			file, err := parser.ParseFile(token.NewFileSet(), "p.go", src, 0)
			if err != nil {
				t.Fatalf("parsing the case: %v", err)
			}
			hit := false
			bound := ""
			ast.Inspect(file, func(n ast.Node) bool {
				if ifStmt, isIf := n.(*ast.IfStmt); isIf && ifStmt.Init != nil {
					if named := elapsedName(ifStmt.Init); named != "" {
						bound = named
					}
				}
				if comparesAnElapsedDuration(n, bound) {
					hit = true
				}
				return true
			})
			if hit != k.wantHit {
				t.Errorf("the detector reports %v for %q, want %v", hit, k.body, k.wantHit)
			}
		})
	}
}
