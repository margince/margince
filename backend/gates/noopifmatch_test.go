// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind shape H2

package gates

// An update that skips its write because nothing changes still answers the If-Match.
//
// ApplyGuarded compares the caller's version inside its UPDATE. An update that
// answers before it never reaches that compare. That path must compare the
// version too: storekit.EnsureIfMatch, a compare of the dereferenced If-Match,
// or a helper handed the If-Match, placed before the early answer.
//
// The subject is a function that names the If-Match and hands it to a write.
// Two shapes skip that write: a branch on a patch's Empty(), and a success
// return before the write.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

func TestAnEmptyPatchStillAnswersTheIfMatch(t *testing.T) {
	t.Parallel()

	files := gatekit.Scope{
		Roots:   []string{"internal"},
		Subject: fileSkipsAWriteUnderIfMatch,
	}.Files(t)

	judged := 0
	for _, parsed := range files {
		for _, miss := range unguardedNoopExits(parsed.File) {
			t.Errorf("%s (%s): %s without comparing the If-Match first, so a stale version answers 200.\n"+
				"\tCall storekit.EnsureIfMatch before that answer.", parsed.Path, miss.fn, miss.what)
		}
		judged++
	}
	if judged == 0 {
		t.Fatal("no update under an If-Match skips its write anywhere, so this gate judged nothing; " +
			"the walk is broken, or the If-Match field was renamed")
	}
}

// The gate must see both shapes it claims, or a green run says nothing about
// them. Each planted body is a shape that stood unguarded in this tree.
func TestTheNoopIfMatchGateSeesItsShapes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body string
		want int
	}{
		{"an early return before the write", `
			if current.Stage == in.Stage {
				out, err = readDeal(ctx, tx, id)
				return err
			}
			return applyGuarded(ctx, tx, id, p, in.IfVersion)`, 1},
		{"the same return after EnsureIfMatch", `
			if current.Stage == in.Stage {
				if err := storekit.EnsureIfMatch(ctx, tx, "deal", id, in.IfVersion); err != nil {
					return err
				}
				out, err = readDeal(ctx, tx, id)
				return err
			}
			return applyGuarded(ctx, tx, id, p, in.IfVersion)`, 0},
		{"an empty patch answered as it stands", `
			if p.Empty() {
				return current, nil
			}
			return p.ApplyGuarded(ctx, tx, "contact", id, in.IfVersion)`, 1},
		{"a write skipped with no else", `
			if !p.Empty() {
				if err := p.ApplyGuarded(ctx, tx, "doc", id, in.IfVersion); err != nil {
					return err
				}
			}
			out, err = readDocument(ctx, tx, id)
			return err`, 1},
		{"an error return before the write", `
			if err := refuse(current); err != nil {
				return err
			}
			return p.ApplyGuarded(ctx, tx, "contact", id, in.IfVersion)`, 0},
	}
	for _, tc := range cases {
		file, err := parser.ParseFile(token.NewFileSet(), "planted.go",
			"package planted\nfunc update() error {"+tc.body+"\n}\n", 0)
		if err != nil {
			t.Fatalf("%s: parse: %v", tc.name, err)
		}
		if got := len(unguardedNoopExits(file)); got != tc.want {
			t.Errorf("%s: %d findings, want %d", tc.name, got, tc.want)
		}
	}
}

type noopMiss struct{ fn, what string }

func unguardedNoopExits(file *ast.File) []noopMiss {
	var misses []noopMiss
	forEachFuncBody(file, func(name string, body *ast.BlockStmt) {
		fn := scanNoopExits(body)
		if !fn.subject() {
			return
		}
		for _, branch := range fn.emptyBranches {
			if !fn.guardedBranch(branch) {
				misses = append(misses, noopMiss{name, "it skips its write on an empty patch"})
			}
		}
		for _, exit := range fn.earlyExits {
			if exit.Pos() < fn.write && !fn.inEmptyBranch(exit) && !fn.guardedBefore(exit.Pos()) {
				misses = append(misses, noopMiss{name, "it answers before its write"})
			}
		}
	})
	return misses
}

func fileSkipsAWriteUnderIfMatch(_ string, file *ast.File) bool {
	found := false
	forEachFuncBody(file, func(_ string, body *ast.BlockStmt) {
		fn := scanNoopExits(body)
		found = found || (fn.subject() && (len(fn.emptyBranches) > 0 || len(fn.earlyExits) > 0))
	})
	return found
}

// forEachFuncBody visits every declared function and every closure as its own
// unit. The update body usually sits in a transaction closure, and the
// If-Match it reads is named there.
func forEachFuncBody(file *ast.File, visit func(name string, body *ast.BlockStmt)) {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		visit(fn.Name.Name, fn.Body)
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if lit, ok := n.(*ast.FuncLit); ok {
				visit(fn.Name.Name+" (closure)", lit.Body)
			}
			return true
		})
	}
}

// emptyBranch is one `if p.Empty()` or `if !p.Empty()`, with the block that
// runs when the patch writes nothing (nil when that side is absent).
type emptyBranch struct {
	stmt      *ast.IfStmt
	emptySide ast.Node
}

type noopScan struct {
	namesIfMatch  bool
	emptyBranches []emptyBranch
	// earlyExits are the returns that answer success: a nil error, or the
	// error of the read just before them.
	earlyExits []*ast.ReturnStmt
	guards     []ast.Node
	// write is the position of the last call handed the If-Match, which is
	// the guarded write; zero when the function hands it to nothing.
	write token.Pos
}

func (s noopScan) subject() bool { return s.namesIfMatch && s.write.IsValid() }

func (s noopScan) guardedBefore(pos token.Pos) bool {
	return slices.ContainsFunc(s.guards, func(guard ast.Node) bool { return guard.Pos() < pos })
}

func (s noopScan) guardedBranch(branch emptyBranch) bool {
	if s.guardedBefore(branch.stmt.Pos()) {
		return true
	}
	side := branch.emptySide
	return side != nil && slices.ContainsFunc(s.guards, func(guard ast.Node) bool {
		return side.Pos() <= guard.Pos() && guard.End() <= side.End()
	})
}

func (s noopScan) inEmptyBranch(exit ast.Node) bool {
	return slices.ContainsFunc(s.emptyBranches, func(branch emptyBranch) bool {
		side := branch.emptySide
		return side != nil && side.Pos() <= exit.Pos() && exit.End() <= side.End()
	})
}

// scanNoopExits reads one function body without entering its closures,
// which forEachFuncBody visits as units of their own.
func scanNoopExits(body *ast.BlockStmt) noopScan {
	var scan noopScan
	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.Ident:
			scan.namesIfMatch = scan.namesIfMatch || isIfMatchName(node.Name)
		case *ast.SelectorExpr:
			scan.namesIfMatch = scan.namesIfMatch || isIfMatchName(node.Sel.Name)
		case *ast.IfStmt:
			if branch, ok := asEmptyBranch(node); ok {
				scan.emptyBranches = append(scan.emptyBranches, branch)
			}
		case *ast.BlockStmt:
			scan.earlyExits = append(scan.earlyExits, successReturns(node.List)...)
		case *ast.CaseClause:
			scan.earlyExits = append(scan.earlyExits, successReturns(node.Body)...)
		case *ast.BinaryExpr:
			if (node.Op == token.NEQ || node.Op == token.EQL) && (isDerefIfMatch(node.X) || isDerefIfMatch(node.Y)) {
				scan.guards = append(scan.guards, node)
			}
		case *ast.CallExpr:
			scan.noteCall(node)
		}
		return true
	})
	return scan
}

// noteCall files EnsureIfMatch as a guard, and any other call handed the
// If-Match as both a guard and the latest write. A helper handed the pointer
// is where the compare moved to. The last such call is the write the early
// answers skip.
func (s *noopScan) noteCall(call *ast.CallExpr) {
	name := ""
	switch fun := call.Fun.(type) {
	case *ast.SelectorExpr:
		name = fun.Sel.Name
	case *ast.Ident:
		name = fun.Name
	}
	if name == "EnsureIfMatch" {
		s.guards = append(s.guards, call)
		return
	}
	if slices.ContainsFunc(call.Args, isIfMatchRef) {
		s.guards = append(s.guards, call)
		s.write = max(s.write, call.Pos())
	}
}

// successReturns finds the returns in one statement list that answer without
// an error of their own. That is a nil last result, or `return err` straight
// after a call assigned err, which is how a read-and-answer ends.
func successReturns(list []ast.Stmt) []*ast.ReturnStmt {
	var out []*ast.ReturnStmt
	for i, stmt := range list {
		ret, ok := stmt.(*ast.ReturnStmt)
		if !ok || len(ret.Results) == 0 {
			continue
		}
		last := ret.Results[len(ret.Results)-1]
		if isIdentNamed(last, "nil") || (isIdentNamed(last, "err") && i > 0 && assignsErrFromCall(list[i-1])) {
			out = append(out, ret)
		}
	}
	return out
}

func assignsErrFromCall(stmt ast.Stmt) bool {
	assign, ok := stmt.(*ast.AssignStmt)
	if !ok || len(assign.Rhs) != 1 {
		return false
	}
	if _, ok := assign.Rhs[0].(*ast.CallExpr); !ok {
		return false
	}
	return slices.ContainsFunc(assign.Lhs, func(lhs ast.Expr) bool { return isIdentNamed(lhs, "err") })
}

func isIdentNamed(expr ast.Expr, name string) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == name
}

func isIfMatchName(name string) bool { return name == "IfVersion" || name == "ifVersion" }

func isIfMatchRef(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.Ident:
		return isIfMatchName(e.Name)
	case *ast.SelectorExpr:
		return isIfMatchName(e.Sel.Name)
	}
	return false
}

func isDerefIfMatch(expr ast.Expr) bool {
	star, ok := expr.(*ast.StarExpr)
	return ok && isIfMatchRef(star.X)
}

func asEmptyBranch(stmt *ast.IfStmt) (emptyBranch, bool) {
	cond := stmt.Cond
	negated := false
	if unary, ok := cond.(*ast.UnaryExpr); ok && unary.Op == token.NOT {
		cond, negated = unary.X, true
	}
	call, ok := cond.(*ast.CallExpr)
	if !ok || len(call.Args) != 0 {
		return emptyBranch{}, false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Empty" {
		return emptyBranch{}, false
	}
	if !negated {
		return emptyBranch{stmt: stmt, emptySide: stmt.Body}, true
	}
	if stmt.Else == nil {
		return emptyBranch{stmt: stmt}, true
	}
	return emptyBranch{stmt: stmt, emptySide: stmt.Else}, true
}
