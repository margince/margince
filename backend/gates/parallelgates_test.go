// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates

// Every gate here runs in parallel with the others, and this is what keeps that
// true as gates are added.
//
// These tests are readers: each walks the tree, parses what it finds and judges
// it. Serially that was 102 s and the longest single test was 5.4 s — a flat
// tail of ~100 independent walks, not a hot spot — which is exactly the shape
// parallelism answers. With t.Parallel() it is 18 s.
//
// The reason this needs a gate rather than a convention: a gate added without
// the call still PASSES. It just runs alone while the others share the machine,
// and the suite gets a little slower every time. Nothing fails, so nothing
// tells anybody — the cost arrives as a number in a CI log that nobody is
// comparing against last month's.
//
// WHAT MAKES IT SAFE, since a package of parallel tests sharing state is a fair
// thing to worry about: the state they share is each gatekit.Waivers set, whose
// `matched` map is mutex-guarded, and whose AssertAllMatched must be called
// from exactly one place per declaration — held by
// TestEveryWaiversDeclarationIsSweptForStalenessExactlyOnce. Getting that wrong
// under parallelism reports staleness that is not there, which is a LOUD
// failure rather than a gate quietly passing.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// parallelExempt names the gates that cannot take t.Parallel(), with the reason.
//
// Keyed by test name rather than by file: the obligation is a property of the
// function, and a file-keyed exemption would silently cover every gate that
// later joins that file.
//
// A gatekit.Waivers rather than a bare map, so the reasons are held to the same
// standard as every other waiver in this tree and a stale entry is reported by
// the mechanism that already does that, instead of by a second copy of it here.
var parallelExempt = gatekit.Waive(map[string]string{
	// t.Setenv panics outright in a parallel test — the environment is
	// process-wide, so Go refuses the combination rather than letting one
	// test's variable leak into another's.
	"TestTheSchemaAndTheParserAgreeOnEveryInputDeclaration": "calls t.Setenv, which Go forbids in a parallel test",
	// Under -update-citations another gate REWRITES the file this one reads.
	// Go resumes parallel tests only once every sequential one has finished, so
	// staying sequential is what orders the read before the rewrite.
	"TestTheCitationRegisterIsSortedAndUnique": "reads danglingcitations.txt, which -update-citations rewrites from another gate",
})

func TestEveryGateRunsInParallelWithTheOthers(t *testing.T) {
	t.Parallel()
	// The sole sweep of this declaration, as gatekit requires: matched
	// accumulates across the package, so a second caller would report staleness
	// that is not there.
	defer parallelExempt.AssertAllMatched(t)

	// Below the real count, so a walk that stopped finding gates fails here
	// rather than reporting that all nought of them are parallel.
	const gateFloor = 250

	paths, err := filepath.Glob(filepath.Join(gateDir, "*_test.go"))
	if err != nil {
		t.Fatalf("listing the gate sources: %v", err)
	}

	fset := token.NewFileSet()
	serial, seen := []string{}, 0
	for _, path := range paths {
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			t.Fatalf("parsing %s: %v", path, parseErr)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || !strings.HasPrefix(fn.Name.Name, "Test") || fn.Name.Name == "TestMain" {
				continue
			}
			seen++
			if parallelExempt.Waived(t, fn.Name.Name) {
				continue
			}
			if !callsParallelFirst(fn) {
				serial = append(serial, filepath.Base(path)+":"+fn.Name.Name)
			}
		}
	}

	if seen < gateFloor {
		t.Fatalf("found %d gate(s) and expected at least %d — this census stopped seeing the "+
			"package rather than the package having shrunk", seen, gateFloor)
	}
	for _, name := range serial {
		t.Errorf("%s does not open with t.Parallel(). These gates are readers and run "+
			"concurrently; one that does not costs the suite its own runtime on every run, and "+
			"nothing fails to say so. Add the call, or name it in parallelExempt with the reason "+
			"it cannot take one", name)
	}
}

// callsParallelFirst reports whether the function's FIRST statement is
// t.Parallel().
//
// First, not merely present: Go runs a parallel test's body up to the call
// synchronously, so work above it is serial work that reads as parallel. A gate
// that walked the tree and then called t.Parallel() would look annotated and
// buy nothing.
func callsParallelFirst(fn *ast.FuncDecl) bool {
	if fn.Body == nil || len(fn.Body.List) == 0 {
		return false
	}
	expr, ok := fn.Body.List[0].(*ast.ExprStmt)
	if !ok {
		return false
	}
	call, ok := expr.X.(*ast.CallExpr)
	if !ok {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Parallel" {
		return false
	}
	receiver, ok := selector.X.(*ast.Ident)
	return ok && receiver.Name == "t"
}
