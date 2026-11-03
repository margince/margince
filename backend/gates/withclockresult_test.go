// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// A WithClock method returns a copy, and no caller discards what it returns.
//
// SetClock is the form that sets the clock on the receiver itself. A bare
// `store.WithClock(clock)` changes nothing, and a test that wrote it keeps the
// wall clock. golangci's unusedresult cannot hold this: it takes package
// functions, not methods. The name is matched in any case, so an unexported
// `withClock` keeps the same promise.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// withClockCallFloor is the fewest WithClock calls the walk may see and still
// be believed. It catches a walk that stopped reading files.
const withClockCallFloor = 40

type withClockFinding struct {
	pos token.Position
	why string
}

func (f withClockFinding) String() string { return f.pos.String() + ": " + f.why }

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

// withClockProbe holds each mistake on its own line, and the allowed forms
// beside them. The test names every line it expects, so one form that stops
// matching cannot hide behind another that still does.
const withClockProbe = `package p
type S struct{ clock func() int }
type A struct{ clock func() int }
type B struct{ clock func() int }
type C struct{ clock func() int }
type D struct{ clock func() int }
type V struct{ clock func() int }
func (s *S) WithClock(c func() int) *S { n := *s; n.clock = c; return &n }
func (s *A) WithClock(c func() int) *A { s.clock = c; t := s; return t }
func (s *B) WithClock(c func() int) *B { (*s).clock = c; return &*s }
func (s *C) WithClock(c func() int) *C { t := s; t.clock = c; return t }
func (s *D) withClock(c func() int) *D { s.clock = c; return s }
func (s V) WithClock(c func() int) V { s.clock = c; return s }
func use(s *S) {
	s.WithClock(nil)
	_ = s.WithClock(nil)
	defer s.WithClock(nil)
	go s.WithClock(nil)
	_, kept := s.WithClock(nil), s.WithClock(nil)
	s.withClock(nil)
	kept = s.WithClock(nil)
	use(kept.WithClock(nil))
}
`

func TestWithClockGateFindsEveryFormOnItsOwnLine(t *testing.T) {
	t.Parallel()

	found, seen := withClockFindings("probe.go", []byte(withClockProbe))
	var lines []int
	for _, f := range found {
		lines = append(lines, f.pos.Line)
	}
	slices.Sort(lines)
	// 9-12: four pointer receivers that change a field of the store they were
	// called on. 15-20: a result thrown away by a bare statement, `_ =`, defer,
	// go, a blank beside a kept value, and a lower-case call. Line 8 copies,
	// line 13 has a value receiver, and lines 21-22 keep what they get.
	want := []int{9, 10, 11, 12, 15, 16, 17, 18, 19, 20}
	if !slices.Equal(lines, want) {
		t.Errorf("found problems on lines %v, want %v:\n%s", lines, want, fmt.Sprint(found))
	}
	if seen != 9 {
		t.Errorf("counted %d WithClock calls in the probe, want 9", seen)
	}
}

// withClockFindings parses one file and returns what breaks the rule, plus how
// many WithClock calls it saw. A file that does not parse is left to the
// compiler.
func withClockFindings(path string, src []byte) (found []withClockFinding, calls int) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, 0
	}
	discarded := func(n ast.Node, how string) {
		found = append(found, withClockFinding{fset.Position(n.Pos()), "the result of WithClock is " + how +
			". WithClock returns a copy, so this call changes nothing; keep the result, or call SetClock"})
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncDecl:
			if field := withClockMutation(n); field != nil {
				found = append(found, withClockFinding{fset.Position(field.Pos()), "WithClock sets a field of " +
					"the store it was called on. Change a copy and return that, or name the method SetClock"})
			}
		case *ast.ExprStmt:
			if isWithClockCall(n.X) {
				discarded(n, "discarded")
			}
		case *ast.DeferStmt:
			if isWithClockCall(n.Call) {
				discarded(n, "dropped by defer")
			}
		case *ast.GoStmt:
			if isWithClockCall(n.Call) {
				discarded(n, "dropped by go")
			}
		case *ast.AssignStmt:
			if blankWithClock(n) {
				discarded(n, "assigned to _")
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

func isWithClockName(name string) bool { return strings.EqualFold(name, "WithClock") }

func isWithClockCall(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && isWithClockName(sel.Sel.Name)
}

// blankWithClock reports an assignment that puts a WithClock result into `_`.
func blankWithClock(assign *ast.AssignStmt) bool {
	if len(assign.Lhs) != len(assign.Rhs) {
		return false
	}
	for i, lhs := range assign.Lhs {
		if id, ok := lhs.(*ast.Ident); ok && id.Name == "_" && isWithClockCall(assign.Rhs[i]) {
			return true
		}
	}
	return false
}

// withClockMutation returns where a WithClock method with a pointer receiver
// first sets a field of that receiver. A local holding the same pointer counts
// as the receiver. A value receiver is already a copy.
func withClockMutation(fn *ast.FuncDecl) ast.Node {
	if fn.Recv == nil || len(fn.Recv.List) != 1 || len(fn.Recv.List[0].Names) != 1 ||
		!isWithClockName(fn.Name.Name) || fn.Body == nil {
		return nil
	}
	if _, pointer := fn.Recv.List[0].Type.(*ast.StarExpr); !pointer {
		return nil
	}
	same := map[string]bool{fn.Recv.List[0].Names[0].Name: true}
	var field ast.Node
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || field != nil {
			return field == nil
		}
		for i, lhs := range assign.Lhs {
			if sel, ok := lhs.(*ast.SelectorExpr); ok && same[rootIdent(sel.X)] {
				field = lhs
				return false
			}
			if id, ok := lhs.(*ast.Ident); ok && len(assign.Rhs) == len(assign.Lhs) {
				same[id.Name] = same[pointerAlias(assign.Rhs[i])]
			}
		}
		return true
	})
	return field
}

// pointerAlias names the variable whose pointer an expression copies as is:
// `s`, `(s)` or `&*s`. A dereference such as `*s` copies the value instead.
func pointerAlias(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.ParenExpr:
		return pointerAlias(e.X)
	case *ast.UnaryExpr:
		if star, ok := e.X.(*ast.StarExpr); ok && e.Op == token.AND {
			return pointerAlias(star.X)
		}
	}
	return ""
}

// rootIdent names the variable an expression reads through, past parentheses,
// a dereference and field selections; "" when it is anything else.
func rootIdent(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.ParenExpr:
		return rootIdent(e.X)
	case *ast.StarExpr:
		return rootIdent(e.X)
	case *ast.SelectorExpr:
		return rootIdent(e.X)
	}
	return ""
}
