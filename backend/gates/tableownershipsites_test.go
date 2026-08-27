// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Where a write is, for the waiver that ratifies it.
//
// Split from tableownership_test.go on size, and along the seam that was
// already there: that file asks WHAT a package writes, and this one answers
// WHERE the write sits. Both halves of this key have had to stop naming a
// container instead of a write — a method's receiver, a grouped block's spec,
// a spec's bound name — so the reasoning is worth reading in one place.
//
// This file declares no Test function, so the gate inventory asks it for no
// `//gate:kind` line: the gate it serves is declared in tableownership_test.go
// and stays one row on that page.

package gates

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

// declSite names the declaration a write sits in, for the waiver key.
//
// A function gives its receiver and name; a package-level var or const gives
// the name it is bound to. The bound name matters: a shared file-wide token
// would let a second package-level literal writing the same table ride the
// first one's waiver, which is the category ratification this key exists to
// end, reopened at file grain.
//
// unnamedDeclSite is the last resort — a declaration with no name to take.
const unnamedDeclSite = "<unnamed-decl>"

func declSite(fset *token.FileSet, decl ast.Decl) string {
	fn, ok := decl.(*ast.FuncDecl)
	switch {
	case !ok || fn.Name == nil:
		return unnamedDeclSite
	case fn.Recv == nil || len(fn.Recv.List) != 1:
		return fn.Name.Name
	}
	// The pointer star carries no information here — a method set is named
	// by its type — and leaving it in would put a `*` in every such key.
	//
	// The receiver itself is not decoration. This tree writes two workers
	// into one file as a matter of course, `Work` on one beside `Work` on
	// another, and a bare method name would collapse both onto one key.
	return strings.TrimPrefix(exprText(fset, fn.Recv.List[0].Type), "*") + "." + fn.Name.Name
}

// specSite names ONE spec of a package-level declaration.
//
// Separate from declSite because a grouped `const (...)` is one declaration
// holding many statements: answering per declaration puts every statement in
// the block under its first name — the same collapse the receiver closes for
// methods, one waiver ratifying writes it never saw.
//
// One spec can still bind SEVERAL names, and this answers for the spec rather
// than for a value in it. `walkDeclSites` pairs values with names where it can
// and only falls back here when it cannot; see the count check there.
func specSite(spec ast.Spec) string {
	if vs, ok := spec.(*ast.ValueSpec); ok && len(vs.Names) > 0 {
		return vs.Names[0].Name
	}
	return unnamedDeclSite
}

// walkDeclSites walks file, calling visit for each node with the site of the
// declaration that node sits in. A file's imports, types and package-level
// values are walked too, because a SQL literal bound to a var is still a write.
//
// The traversal lives here rather than inline in the collector so a test can
// drive the REAL attribution over a fixture. Asserting declSite and specSite
// directly proves the naming helpers work and says nothing about whether the
// walk reaches them, which is where a grouped block is won or lost.
func walkDeclSites(fset *token.FileSet, file *ast.File, visit func(site string, n ast.Node)) {
	// visit returns nothing: every node is offered, because a SQL literal can
	// sit at any depth and this walk has no subtree it can safely skip.
	inspect := func(n ast.Node, site string) {
		ast.Inspect(n, func(n ast.Node) bool { visit(site, n); return true })
	}
	for _, decl := range file.Decls {
		if gen, ok := decl.(*ast.GenDecl); ok {
			for _, spec := range gen.Specs {
				// `const a, b = "…", "…"` is ONE spec binding two statements.
				// Answering per spec puts both under `a`, which is the grouped
				// block's collapse one level further in. Pair each value with
				// the name it is bound to when the counts agree.
				if vs, ok := spec.(*ast.ValueSpec); ok && len(vs.Values) > 0 && len(vs.Names) == len(vs.Values) {
					for i, value := range vs.Values {
						inspect(value, vs.Names[i].Name)
					}
					continue
				}
				// They disagree for `var a, b = f()` — one expression yielding
				// several names, where no value is attributable to one name.
				// The spec's own site is the honest answer there, and it is the
				// first name rather than a token every such spec would share.
				inspect(spec, specSite(spec))
			}
			continue
		}
		inspect(decl, declSite(fset, decl))
	}
}

// literalSitesOf parses src and returns the site attributed to each SQL string
// literal in it, driving the same walk the collector uses.
//
// Through walkDeclSites, not the naming helpers: a case that called specSite
// directly stayed green when the walk's grouped-block arm was deleted, because
// nothing it touched had changed.
func literalSitesOf(t *testing.T, filename, src string) []string {
	t.Helper()
	return sitesOf(t, filename, src, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return false
		}
		text, err := strconv.Unquote(lit.Value)
		return err == nil && len(sqlWriteTargets(text)) > 0
	})
}

// methodSitesOf returns the site attributed to each method BODY in src.
func methodSitesOf(t *testing.T, filename, src string) []string {
	t.Helper()
	return sitesOf(t, filename, src, func(n ast.Node) bool {
		_, ok := n.(*ast.BlockStmt)
		return ok
	})
}

func sitesOf(t *testing.T, filename, src string, keep func(ast.Node) bool) []string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, 0)
	if err != nil {
		t.Fatalf("parse the fixture: %v", err)
	}
	var sites []string
	walkDeclSites(fset, file, func(site string, n ast.Node) {
		if n != nil && keep(n) {
			sites = append(sites, site)
		}
	})
	return sites
}
