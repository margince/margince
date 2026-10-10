// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind shape H2

package gates

// An update that skips its write because the patch is empty still answers the If-Match.
//
// ApplyGuarded compares the caller's version inside its UPDATE. An update that
// returns before it on an empty patch never reaches that compare. Its empty
// path must compare the version too, through storekit.EnsureIfMatch, a compare
// of the dereferenced If-Match, or a helper handed the If-Match.
//
// The subject is a function that names the If-Match and branches on Empty().
// A no-op test spelled some other way is out of this gate's sight.

import (
	"go/ast"
	"go/token"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

func TestAnEmptyPatchStillAnswersTheIfMatch(t *testing.T) {
	t.Parallel()

	files := gatekit.Scope{
		Roots:   []string{"internal"},
		Subject: fileBranchesOnAnEmptyPatchUnderIfMatch,
	}.Files(t)

	branches := 0
	for _, parsed := range files {
		forEachFuncBody(parsed.File, func(name string, body *ast.BlockStmt) {
			fn := scanNoopBranches(body)
			if !fn.namesIfMatch {
				return
			}
			for _, branch := range fn.emptyBranches {
				branches++
				if !fn.guardedBefore(branch) {
					t.Errorf("%s (%s): this update skips its write on an empty patch without "+
						"comparing the If-Match first, so a stale version answers 200.\n"+
						"\tCall storekit.EnsureIfMatch on the side of the branch that writes nothing, "+
						"or compare the version before the branch.",
						parsed.Path, name)
				}
			}
		})
	}
	if branches == 0 {
		t.Fatal("no update under an If-Match branches on an empty patch, so this gate judged nothing; " +
			"the walk is broken, or Patch.Empty was renamed")
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
	guards        []ast.Node
}

func (s noopScan) guardedBefore(branch emptyBranch) bool {
	for _, guard := range s.guards {
		if guard.Pos() < branch.stmt.Pos() {
			return true
		}
		side := branch.emptySide
		if side != nil && side.Pos() <= guard.Pos() && guard.End() <= side.End() {
			return true
		}
	}
	return false
}

func fileBranchesOnAnEmptyPatchUnderIfMatch(_ string, file *ast.File) bool {
	found := false
	forEachFuncBody(file, func(_ string, body *ast.BlockStmt) {
		fn := scanNoopBranches(body)
		found = found || (fn.namesIfMatch && len(fn.emptyBranches) > 0)
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

// scanNoopBranches reads one function body without entering its closures,
// which forEachFuncBody visits as units of their own.
func scanNoopBranches(body *ast.BlockStmt) noopScan {
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
		case *ast.BinaryExpr:
			if (node.Op == token.NEQ || node.Op == token.EQL) && (isDerefIfMatch(node.X) || isDerefIfMatch(node.Y)) {
				scan.guards = append(scan.guards, node)
			}
		case *ast.CallExpr:
			if isIfMatchGuardCall(node) {
				scan.guards = append(scan.guards, node)
			}
		}
		return true
	})
	return scan
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

// isIfMatchGuardCall is EnsureIfMatch, or any call handed the If-Match itself:
// a helper that takes the pointer is where the compare moved to. The guarded
// write matches too, but it sits after the branch, where guardedBefore does
// not count it.
func isIfMatchGuardCall(call *ast.CallExpr) bool {
	name := ""
	switch fun := call.Fun.(type) {
	case *ast.SelectorExpr:
		name = fun.Sel.Name
	case *ast.Ident:
		name = fun.Name
	}
	if name == "EnsureIfMatch" {
		return true
	}
	return slices.ContainsFunc(call.Args, isIfMatchRef)
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
