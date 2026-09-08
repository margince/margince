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
	"strconv"
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
		file: "internal/modules/deals/dealadmittedmove.go", function: "autoExecutedMoveIsOpenToOpen",
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
		compared, ok := endpointsJoinedByAnd(fn, site.openConst)
		if !ok {
			t.Errorf("%s in %s does not read as TWO comparisons against %s joined by one &&.\n"+
				"  The rule is about BOTH endpoints: a side that checks one has stopped mirroring the "+
				"other, and an OR admits a move with one terminal endpoint — the reopen half of what "+
				"this refuses.\n"+
				"  Direct operands of an && found comparing against %s: %v",
				site.function, site.file, site.openConst, site.openConst, compared)
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

// endpointsJoinedByAnd finds an && whose two DIRECT operands are each a
// comparison against the constant, and answers what they compare.
//
// Structural rather than a count of && nodes, which is what this asked first:
// two comparisons and one && somewhere in the function is satisfied by
// `a == open && unrelated` beside a second comparison the rule never reads. The
// operands have to BE the comparisons for the sentence to be the rule.
func endpointsJoinedByAnd(fn *ast.FuncDecl, constName string) ([]string, bool) {
	var found []string
	joined := false
	ast.Inspect(fn, func(n ast.Node) bool {
		and, isBinary := n.(*ast.BinaryExpr)
		if !isBinary || and.Op != token.LAND || joined {
			return true
		}
		left, leftOK := comparedTo(and.X, constName)
		right, rightOK := comparedTo(and.Y, constName)
		if leftOK && rightOK && left != right {
			found, joined = []string{left, right}, true
		}
		return !joined
	})
	return found, joined
}

// comparedTo answers what an expression compares for equality against the
// constant, and whether it is such a comparison at all.
func comparedTo(expr ast.Expr, constName string) (string, bool) {
	bin, isBinary := expr.(*ast.BinaryExpr)
	if !isBinary || bin.Op != token.EQL {
		return "", false
	}
	if names(bin.Y, constName) {
		return render(bin.X), true
	}
	if names(bin.X, constName) {
		return render(bin.Y), true
	}
	return "", false
}

// names reports whether the expression is the PACKAGE-LOCAL constant, bare or
// inside a conversion — StageSemantic(semantic) == SemanticOpen and
// semantic == stageSemanticOpen are the same sentence.
//
// A qualified selector is deliberately NOT one. Matching on the field name alone
// accepted any `somepkg.SemanticOpen`, so a side could come to read an imported
// constant this gate never checks the value of, and the mirror would drift while
// reading green. What each side must compare against is the constant declared in
// its own package, which is the one constValueIn reads.
func names(expr ast.Expr, constName string) bool {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name == constName
	case *ast.CallExpr:
		return len(e.Args) == 1 && names(e.Args[0], constName)
	}
	return false
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

// constValueIn reads one string CONSTANT's value out of a file.
//
// Three things it insists on, each because the loose version accepted something
// that is not the declaration this gate is about:
//
//   - a `const` GenDecl, not any ValueSpec. Go spells `var` with the same node,
//     so the loose reader passed happily after the semantic stopped being a
//     constant — which is a change to the thing being mirrored, made invisible.
//   - one name to one value. A grouped `a, b = "x", "y"` line indexes by
//     position, and reading the wrong half of one would compare the wrong string.
//   - strconv.Unquote rather than trimming quote characters. A raw literal has
//     no quotes to trim and an escaped one means something other than its
//     source text, so trimming answers a value the compiler never sees.
func constValueIn(t *testing.T, tree, relPath, name string) string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), tree+"/"+relPath, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", relPath, err)
	}
	for _, decl := range file.Decls {
		gen, isGen := decl.(*ast.GenDecl)
		if !isGen || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			value, found := constSpecValue(t, spec, relPath, name)
			if found {
				return value
			}
		}
	}
	t.Fatalf("%s declares no string constant %s — the mirror's vocabulary moved, and the OTHER "+
		"side should be checked for having moved with it", relPath, name)
	return ""
}

func constSpecValue(t *testing.T, spec ast.Spec, relPath, name string) (string, bool) {
	t.Helper()
	value, isValue := spec.(*ast.ValueSpec)
	if !isValue {
		return "", false
	}
	for i, ident := range value.Names {
		if ident.Name != name {
			continue
		}
		if len(value.Names) != len(value.Values) {
			t.Fatalf("%s declares %s in a grouped const with %d name(s) and %d value(s); this gate "+
				"reads one name to one value and would otherwise compare the wrong string",
				relPath, name, len(value.Names), len(value.Values))
		}
		lit, isLit := value.Values[i].(*ast.BasicLit)
		if !isLit || lit.Kind != token.STRING {
			t.Fatalf("%s declares %s as something other than a string literal, so what the two sides "+
				"mean by it is no longer readable here", relPath, name)
		}
		unquoted, err := strconv.Unquote(lit.Value)
		if err != nil {
			t.Fatalf("%s declares %s as a string literal this gate cannot read (%s): %v",
				relPath, name, lit.Value, err)
		}
		return unquoted, true
	}
	return "", false
}
