// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The scope narrowing is spelled ONCE, and this is what holds that.
//
// The defect it comes from: the waiting loop judged ownership in its own terms —
// over the WaitingCustomer, before classification — while the page-wide filter
// judged the ranked row. The two then disagreed, and a manager opening a rep's
// queue got a page with every waiting customer removed.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// Only narrowToScope reaches the three keep* filters.
//
// A caller that picked between them itself would be a second copy of the rule,
// and the copy is what drifts: the two spellings agreed the day they were
// written and disagreed the first time either changed.
func TestOnlyNarrowToScopeChoosesBetweenTheScopeFilters(t *testing.T) {
	t.Parallel()

	const chooser = "narrowToScope"
	filters := map[string]bool{
		"keepReadersOwn": true, "keepOwnedBy": true, "keepUnowned": true,
	}
	for file, callers := range callersOfIn(t, filters) {
		for fn, called := range callers {
			if fn == chooser {
				continue
			}
			t.Errorf("%s: %s calls %s directly — every scope narrowing goes through %s, "+
				"so the waiting rows and the whole page cannot be narrowed by different rules",
				file, fn, called, chooser)
		}
	}
}

// callersOfIn reports which function in which file calls one of the named
// functions, as file → caller → callee.
//
// It reads the package's own source rather than a hand-kept list, so a fourth
// filter or a new caller is judged the day it appears. A list would go stale
// silently, which is the failure this whole file is about.
func callersOfIn(t *testing.T, wanted map[string]bool) map[string]map[string]string {
	t.Helper()
	found := map[string]map[string]string{}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the package directory: %v", err)
	}
	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			ast.Inspect(fn, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				ident, ok := call.Fun.(*ast.Ident)
				if !ok || !wanted[ident.Name] {
					return true
				}
				if found[name] == nil {
					found[name] = map[string]string{}
				}
				found[name][fn.Name.Name] = ident.Name
				return true
			})
		}
	}
	// A census that finds nothing has failed rather than passed: the filters are
	// called, so an empty result means the scan stopped seeing its subject.
	if len(found) == 0 {
		t.Fatal("no caller of any scope filter was found — the scan is reading the " +
			"wrong tree, and would report PASS over any defect")
	}
	return found
}
