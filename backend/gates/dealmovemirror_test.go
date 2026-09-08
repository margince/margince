// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// One rule, spelled on both sides of a module boundary, held equal in both
// directions.
//
// The tier gate auto-executes a deal move exactly when BOTH endpoints are open
// (agents.advanceDealTier). The write re-derives that premise inside its own
// transaction (deals.autoExecutedMoveIsOpenToOpen), because the version pin the
// gate carries forward binds the DEAL and a stage row is mutable independently
// of any deal — so an admin changing a stage's semantic in the window leaves
// the pin satisfied while the move the gate admitted is no longer the move
// being made.
//
// The two cannot share a function: a module never imports a sibling, and these
// are the agents and deals modules. So the rule is spelled twice, and this gate
// is what stops the copies drifting — the shape AGENTS.md names, where fixing
// one side alone can be a regression rather than half a fix. Widen the
// resolver's condition without widening the re-check and the write refuses
// moves the gate admits; narrow the re-check without narrowing the resolver and
// the window reopens silently.
//
// It reads SOURCE rather than calling either function, because calling them
// would need this gate to import both modules and would prove only that two
// packages compile. What has to stay equal is the sentence each one is written
// as.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// dealMoveOpenSemantic is the value both sides must call "open". It is written
// here rather than taken from either side: read from one, the comparison would
// be that side against itself, and whatever it said would pass.
const dealMoveOpenSemantic = "open"

// dealMoveMirrorSites are the two declarations that spell the rule, and what
// each one's own package calls the open semantic.
var dealMoveMirrorSites = []struct {
	file, function, openConst, constFile string
}{
	{
		file: "internal/modules/agents/dealmove.go", function: "advanceDealTier",
		openConst: "stageSemanticOpen", constFile: "internal/modules/agents/dealmove.go",
	},
	{
		file: "internal/modules/deals/deal_advance.go", function: "autoExecutedMoveIsOpenToOpen",
		// The deals module keeps its semantic vocabulary in one place, so the
		// constant is not in the file that spells the rule.
		openConst: "SemanticOpen", constFile: "internal/modules/deals/status.go",
	},
}

// TestTheDealMoveTierRuleReadsTheSameOnBothSides holds the two spellings to one
// shape: each compares a SOURCE and a TARGET semantic against its package's own
// open constant, joined by AND.
func TestTheDealMoveTierRuleReadsTheSameOnBothSides(t *testing.T) {
	t.Parallel()
	tree := moduleRoot(t)

	for _, site := range dealMoveMirrorSites {
		fn := functionNamed(t, tree, site.file, site.function)
		compared := identsComparedTo(fn, site.openConst)
		if len(compared) != 2 {
			t.Errorf("%s in %s compares %d value(s) against %s, want exactly 2 — the rule is about "+
				"BOTH endpoints of the move, and a side that checks one has stopped mirroring the "+
				"other. Comparisons found: %v",
				site.function, site.file, len(compared), site.openConst, compared)
		}
		if ands := countLogicalAnds(fn); ands != 1 {
			t.Errorf("%s in %s joins its comparisons with %d && , want exactly 1 — an OR here admits "+
				"a move with one terminal endpoint, which is the reopen half of what this rule refuses",
				site.function, site.file, ands)
		}
	}

	// And that both packages mean the same string by "open". A gate holding the
	// two SHAPES equal while the constants differ would pass a tree where one
	// side auto-executes moves the other refuses.
	for _, site := range dealMoveMirrorSites {
		if got := constValueIn(t, tree, site.constFile, site.openConst); got != dealMoveOpenSemantic {
			t.Errorf("%s in %s is %q, want %q — the two sides would then disagree about which "+
				"moves run unattended", site.openConst, site.constFile, got, dealMoveOpenSemantic)
		}
	}
}

// functionNamed returns one named function declaration, failing when it is
// gone: a mirror site that vanished is the drift this gate exists to report,
// not a case to skip.
func functionNamed(t *testing.T, tree, relPath, name string) *ast.FuncDecl {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), tree+"/"+relPath, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", relPath, err)
	}
	for _, decl := range file.Decls {
		fn, isFunc := decl.(*ast.FuncDecl)
		if isFunc && fn.Name.Name == name && fn.Body != nil {
			return fn
		}
	}
	t.Fatalf("%s declares no function %s — the mirror moved; repoint dealMoveMirrorSites at where "+
		"the rule is spelled now, and check the OTHER side moved with it", relPath, name)
	return nil
}

// identsComparedTo names every expression compared for equality against the
// constant, sorted so the report is stable.
func identsComparedTo(fn *ast.FuncDecl, constName string) []string {
	var compared []string
	ast.Inspect(fn, func(n ast.Node) bool {
		bin, isBinary := n.(*ast.BinaryExpr)
		if !isBinary || bin.Op != token.EQL {
			return true
		}
		if names(bin.Y, constName) {
			compared = append(compared, render(bin.X))
		}
		if names(bin.X, constName) {
			compared = append(compared, render(bin.Y))
		}
		return true
	})
	return compared
}

// names reports whether the expression is the constant, bare or as a
// conversion around it — StageSemantic(semantic) == SemanticOpen and
// semantic == stageSemanticOpen are the same sentence.
func names(expr ast.Expr, constName string) bool {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name == constName
	case *ast.SelectorExpr:
		return e.Sel.Name == constName
	case *ast.CallExpr:
		return len(e.Args) == 1 && names(e.Args[0], constName)
	}
	return false
}

func countLogicalAnds(fn *ast.FuncDecl) int {
	ands := 0
	ast.Inspect(fn, func(n ast.Node) bool {
		if bin, isBinary := n.(*ast.BinaryExpr); isBinary && bin.Op == token.LAND {
			ands++
		}
		return true
	})
	return ands
}

// render is an expression as its source words, enough to name it in a failure.
func render(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return render(e.X) + "." + e.Sel.Name
	case *ast.CallExpr:
		var args []string
		for _, a := range e.Args {
			args = append(args, render(a))
		}
		return render(e.Fun) + "(" + strings.Join(args, ", ") + ")"
	}
	return "?"
}

// constValueIn reads one string constant's literal value out of a file.
func constValueIn(t *testing.T, tree, relPath, name string) string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), tree+"/"+relPath, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", relPath, err)
	}
	found := ""
	ast.Inspect(file, func(n ast.Node) bool {
		spec, isValue := n.(*ast.ValueSpec)
		if !isValue {
			return true
		}
		for i, ident := range spec.Names {
			if ident.Name != name || i >= len(spec.Values) {
				continue
			}
			if lit, isLit := spec.Values[i].(*ast.BasicLit); isLit && lit.Kind == token.STRING {
				found = strings.Trim(lit.Value, `"`)
			}
		}
		return true
	})
	if found == "" {
		t.Fatalf("%s declares no string constant %s — the mirror's vocabulary moved", relPath, name)
	}
	return found
}
