// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// What a selected purge does to a message is written once.
//
// Two paths select mail for destruction — an owner acting on their own
// exclusion rule, and the sweep that destroys personal mail once its window
// closes — and they differ in what they select and why. What HAPPENS to a
// selected message is one answer: anonymise the contacts first, while their mail
// still exists to identify them by; destroy the mail; release the claims on it;
// audit the release; recompute the audience the release changed.
//
// `compose.(*CapturePurger).carryOut` is that answer, and its own doc says it is
// the only place the order is written. A second copy is how one path would
// quietly stop auditing a release or stop recomputing an audience — a message an
// owner was told is gone, still claimed on a colleague's timeline, with nothing
// to say who could read it or when that changed.
//
// DERIVED FROM THE TWO DESTRUCTIVE CALLS carryOut makes, not from a list of
// purge paths. A gate that named the paths would be a second copy of the thing
// it protects and would go stale the day a third arrives, which is the only day
// it matters. Asking instead "who may call these" answers for a path nobody has
// written yet.
//
// WHAT IT CANNOT SEE. A path that destroys mail some other way — a DELETE of its
// own, or a new entry point on the retention service. The table-ownership gate
// is what catches the first; the second would have to be added here, and the
// floor below is what makes its absence visible rather than silent.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// theOneExecutor is the function allowed to compose a purge's effects.
const theOneExecutor = "carryOut"

// destructiveSteps are the calls that make a purge irreversible. Named by
// METHOD, because the receiver is a seam the composition root injects and the
// name is what a second path would reach for.
var destructiveSteps = map[string]bool{
	"PurgeActivities":   true,
	"AnonymiseContacts": true,
}

// purgeCallerRoots are the trees a purge path could be written in. Wider than
// the one package that holds carryOut on purpose: the gate exists for the path
// written somewhere else.
var purgeCallerRoots = []string{"internal/compose", "internal/modules", "cmd"}

func TestBothPurgePathsShareOneExecutor(t *testing.T) {
	t.Parallel()
	calls := 0
	for _, root := range purgeCallerRoots {
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
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				for _, step := range destructiveStepsIn(fn.Body) {
					calls++
					if fn.Name.Name == theOneExecutor {
						continue
					}
					t.Errorf("%s: %s calls %s, which only %s may — what a selected purge does to a "+
						"message is one sequence, and a second copy is how one path stops auditing a "+
						"release or stops recomputing the audience the release changed. Select here "+
						"and hand the subject to the executor",
						fset.Position(fn.Pos()), fn.Name.Name, step, theOneExecutor)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}
	// The census that can fail short: a rename on the retention service would
	// leave this matching nothing and reporting PASS over a tree it never read,
	// which is indistinguishable from a tree with one executor.
	if calls < len(destructiveSteps) {
		t.Fatalf("found %d call(s) to %v across %v, want at least %d — the steps have been renamed and "+
			"this gate is now reading a tree it does not recognise",
			calls, sortedSteps(), purgeCallerRoots, len(destructiveSteps))
	}
}

// destructiveStepsIn names every destructive step a body calls, in source order.
func destructiveStepsIn(body *ast.BlockStmt) []string {
	var found []string
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || !destructiveSteps[sel.Sel.Name] {
			return true
		}
		found = append(found, sel.Sel.Name)
		return true
	})
	return found
}

func sortedSteps() []string {
	out := make([]string, 0, len(destructiveSteps))
	for step := range destructiveSteps {
		out = append(out, step)
	}
	return out
}
