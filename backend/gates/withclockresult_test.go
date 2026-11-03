// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// A WithClock method returns a copy, and no caller discards what it returns.
//
// SetClock is the form that sets the clock on the receiver itself. A bare
// `store.WithClock(clock)` changes nothing, and a test that wrote it keeps the
// wall clock. golangci's unusedresult cannot hold this: it takes package
// functions, not methods.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withClockCallFloor is the fewest WithClock calls the walk may see and still
// be believed. It catches a walk that stopped reading files.
const withClockCallFloor = 40

func TestWithClockReturnsACopyWhoseResultIsUsed(t *testing.T) {
	t.Parallel()

	calls := 0
	for _, root := range []string{"internal", "cmd", "pkg", "gates"} {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
				return err
			}
			src, err := os.ReadFile(path) // #nosec G304 -- a *.go file from walking the trusted backend tree
			if err != nil {
				return err
			}
			found, seen := withClockFindings(path, src)
			calls += seen
			for _, f := range found {
				t.Error(f)
			}
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			t.Fatalf("walking %s: %v", root, err)
		}
	}
	if calls < withClockCallFloor {
		t.Fatalf("saw only %d WithClock call(s) and expects at least %d: the walk is broken, not the tree",
			calls, withClockCallFloor)
	}
}

// TestWithClockGateCatchesBothShapes runs the check on the two mistakes it
// exists for, so a gate that stopped matching cannot pass a broken tree.
func TestWithClockGateCatchesBothShapes(t *testing.T) {
	t.Parallel()

	src := []byte(`package p
type Store struct{ clock func() int }
func (s *Store) WithClock(c func() int) *Store { s.clock = c; return s }
func use(s *Store) {
	s.WithClock(nil)
	_ = s.WithClock(nil)
	t := s.WithClock(nil)
	t.WithClock(nil).WithClock(nil)
}
`)
	found, seen := withClockFindings("probe.go", src)
	if seen != 5 {
		t.Errorf("counted %d WithClock calls in the probe, want 5", seen)
	}
	if len(found) != 3 {
		t.Fatalf("found %d problem(s) in the probe, want 3 (the mutating method and two discarded calls): %q",
			len(found), found)
	}
}

// withClockFindings parses one file and returns what breaks the rule, plus how
// many WithClock calls it saw. A file that does not parse is left to the
// compiler.
func withClockFindings(path string, src []byte) (found []string, calls int) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, 0
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncDecl:
			if receiver := withClockMutatingReceiver(n); receiver != "" {
				found = append(found, fset.Position(n.Pos()).String()+": WithClock returns its receiver "+
					receiver+", so it sets the clock in place. Return a copy, or name the method SetClock")
			}
		case *ast.ExprStmt:
			if isWithClockCall(n.X) {
				found = append(found, fset.Position(n.Pos()).String()+": the result of WithClock is discarded. "+
					"WithClock returns a copy, so this call changes nothing; assign it, or call SetClock")
			}
		case *ast.CallExpr:
			if isWithClockCall(n) {
				calls++
			}
		}
		return true
	})
	return found, calls
}

func isWithClockCall(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "WithClock"
}

// withClockMutatingReceiver names the receiver a WithClock method hands back
// as is, or returns "" when the method is something else.
func withClockMutatingReceiver(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) != 1 || len(fn.Recv.List[0].Names) != 1 ||
		fn.Name.Name != "WithClock" || fn.Body == nil {
		return ""
	}
	receiver := fn.Recv.List[0].Names[0].Name
	mutates := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if ret, ok := n.(*ast.ReturnStmt); ok && len(ret.Results) == 1 {
			if id, ok := ret.Results[0].(*ast.Ident); ok && id.Name == receiver {
				mutates = true
			}
		}
		return !mutates
	})
	if mutates {
		return receiver
	}
	return ""
}
