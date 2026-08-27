// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// A prompt built from a crawled page is bounded by this product, not by the
// site.
//
// Two lanes render a company's own website into a model call. The profile lane
// bounded its corpus — a per-page share inside a whole-call budget — and the
// page-facts lane did not: it indexed the page's full stripped text, up to
// webread's 1 MiB fetch cap. Nothing said the two were the same obligation, so
// one was fixed and the other was not noticed for months.
//
// The cost is not hypothetical. Prompt length there is chosen by whoever
// published the page: on a metered provider it is their decision about our
// tokens, and on a local one it sizes the context window the adapter has to
// allocate, which is how the gap was found. Clamping it inside the adapter is
// the last line of defence; the prompt is where the limit belongs.
//
// So the argument to newSnippetIndex — the one place every such prompt's
// evidence space is built — must be a CALL to a function that applied a
// budget. A page's own text cannot reach it without one, and a third lane
// cannot be added without deciding how much of a page it reads.
//
// Tests are exempt by rule rather than by waiver: a unit test of the index
// itself constructs the passages it is about, and routing those through an
// excerpt would make every fixture describe a budget it is not testing.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// promptLanesDir holds the lanes that build model calls.
const promptLanesDir = "internal/compose"

// indexBuilder is the one constructor of a call's numbered evidence space.
const indexBuilder = "newSnippetIndex"

// excerptFuncs are the functions ratified to decide how much of a page a
// prompt reads. Each applies a rune budget and reports what it left behind, so
// a truncated read is not mistaken downstream for a complete one.
//
// Adding a name here is the decision this gate exists to make deliberate: it
// says a new lane has a budget, and the reviewer's job is to check that it
// does.
var excerptFuncs = []string{"pageFactsExcerpt", "profileExcerptPages"}

func TestEverySnippetIndexIsBuiltFromAnExcerpt(t *testing.T) {
	t.Parallel()
	var raw []string
	built := 0
	fset := token.NewFileSet()
	err := filepath.WalkDir(promptLanesDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		path = filepath.ToSlash(path)
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		excerpted := excerptedNames(file)
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			name, ok := call.Fun.(*ast.Ident)
			if !ok || name.Name != indexBuilder || len(call.Args) != 1 {
				return true
			}
			built++
			if !fromExcerpt(call.Args[0], excerpted) {
				raw = append(raw, fset.Position(call.Pos()).String())
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", promptLanesDir, err)
	}

	for _, at := range raw {
		t.Errorf("%s builds a snippet index from something no excerpt function produced.\n"+
			"The prompt would then be as long as whoever published the page chose to make it — a "+
			"stranger's decision about a metered provider's tokens, and about the context window a "+
			"local one must allocate. Pass the pages through a function that applies a rune budget "+
			"and reports what it left behind (%s), and name it in excerptFuncs.",
			at, strings.Join(excerptFuncs, ", "))
	}

	// A gate that finds no call sites passes while checking nothing, which is
	// exactly what it would do if the constructor were renamed.
	if built == 0 {
		t.Errorf("no call to %s found under %s — either the evidence space is built some other way now, "+
			"or this gate has gone blind and reports PASS over a tree it no longer describes",
			indexBuilder, promptLanesDir)
	}
}

// fromExcerpt reports whether this argument is the result of an excerpt
// function: the call written inline, or a name assigned from one in the same
// file.
//
// The name form has to be admitted, because the excerpt functions return TWO
// values — the pages and what they left behind — and a two-value call cannot be
// written inline. Refusing it would have forced the remainder to be dropped or
// re-measured, which is the honesty half of this obligation traded away to make
// the other half easier to check.
//
// One file is where the following stops, and that is the whole extent of it: no
// scopes, no shadowing, no assignment through a struct field. A gate that
// followed further would be a data-flow analysis, and one that gave up quietly
// halfway is worse than one whose limit is stated — this one refuses what it
// cannot see, so the failure is a rewrite rather than a false pass.
func fromExcerpt(arg ast.Expr, fromExcerptNames map[string]bool) bool {
	switch node := arg.(type) {
	case *ast.CallExpr:
		name, ok := node.Fun.(*ast.Ident)
		return ok && slices.Contains(excerptFuncs, name.Name)
	case *ast.Ident:
		return fromExcerptNames[node.Name]
	}
	return false
}

// excerptedNames collects the names this file assigns from an excerpt call.
func excerptedNames(file *ast.File) map[string]bool {
	names := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Rhs) != 1 {
			return true
		}
		call, ok := assign.Rhs[0].(*ast.CallExpr)
		if !ok {
			return true
		}
		fn, ok := call.Fun.(*ast.Ident)
		if !ok || !slices.Contains(excerptFuncs, fn.Name) {
			return true
		}
		// The FIRST result is the pages; the second is what was left behind.
		// Naming only the first keeps the remainder from being mistaken for an
		// excerpt if a caller ever passes it by hand.
		if lhs, ok := assign.Lhs[0].(*ast.Ident); ok {
			names[lhs.Name] = true
		}
		return true
	})
	return names
}
