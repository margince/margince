// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

// Which scopes may be WRITTEN against, as opposed to merely reported.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// managed_teams resolves from an omitted READ scope — a manager's teams and
// themselves — and names no single subject. A forecast recorded against it
// would be an assertion about a population nobody can name, and the standing
// call for it could never be looked up again, so the write door refuses it.
func TestManagedTeamsIsRefusedAsAWriteScope(t *testing.T) {
	if err := checkScope(Scope{Kind: ScopeManagedTeams}); err == nil {
		t.Fatal("a forecast was accepted against the managed-teams population")
	}
}

// This is the test readScope's doc comment names, and what makes that comment
// safe to believe.
//
// Two endpoints take a scope off the query string, each arriving with its own
// generated scope_kind type for the same three values. The tempting shape is a
// converter per endpoint — and then one of them admits a pair the other
// refuses, silently, because nothing compares them.
//
// This reads the package's own source for handlers that build a Scope out of
// query parameters. Only readScope may; anything else has become the second
// copy this claim denies exists.
func TestTheReadScopeRuleHasOneSpelling(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the package directory: %v", err)
	}
	var builders []string
	seenReadScope := false
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(fset, name, nil, parser.SkipObjectResolution)
		if parseErr != nil {
			t.Fatalf("parsing %s: %v", name, parseErr)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			if fn.Name.Name == "readScope" {
				seenReadScope = true
				continue
			}
			// A function that both takes an openapi UUID pointer (the scope_id
			// query key) and constructs a Scope is deciding the read door's rule
			// for itself.
			if takesScopeID(fn) && buildsAScope(fn.Body) {
				builders = append(builders, fn.Name.Name)
			}
		}
	}
	if !seenReadScope {
		t.Fatal("readScope was not found — it was renamed or removed, and this scan " +
			"would report one spelling over a package that had grown three")
	}
	for _, name := range builders {
		t.Errorf("%s builds a Scope from query parameters itself. The read door's rule is "+
			"readScope; a second copy is how one endpoint comes to admit a scope pair "+
			"another refuses. Call readScope, or say here why this one cannot.", name)
	}
}

// takesScopeID reports whether a function takes the scope_id query key, which
// arrives as *openapi_types.UUID.
func takesScopeID(fn *ast.FuncDecl) bool {
	if fn.Type.Params == nil {
		return false
	}
	for _, param := range fn.Type.Params.List {
		star, ok := param.Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		if sel, ok := star.X.(*ast.SelectorExpr); ok && sel.Sel.Name == "UUID" {
			return true
		}
	}
	return false
}

// buildsAScope reports whether a body constructs a Scope value of its own.
func buildsAScope(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		if name, ok := lit.Type.(*ast.Ident); ok && name.Name == "Scope" {
			found = true
		}
		return true
	})
	return found
}

// managed_teams reaches the call history only as a RESOLVED scope, and it
// stops there.
//
// It is a read result, never a request: a manager who names no scope gets it
// back from the resolver, covering their teams and themselves. A call is an
// assertion about ONE named population, so there is no chain to read for it —
// the same reason GetForecast looks up no standing call under it.
//
// checkScope refuses it, which is what makes the handler's early return
// load-bearing: without it the resolved scope reaches callHistoryTx, whose
// query is not built for a population that names no single subject.
func TestManagedTeamsIsRefusedAsAHistoryScope(t *testing.T) {
	t.Parallel()
	if err := checkScope(Scope{Kind: ScopeManagedTeams}); err == nil {
		t.Fatal("the call history accepted the managed-teams population, which names " +
			"no single subject and so has no chain of its own to read")
	}
}
