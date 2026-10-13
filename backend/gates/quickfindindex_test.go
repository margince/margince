// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

//go:build !integration

package gates

// Every list quick-find is proved to read its trigram index.
//
// storekit.QuickFindClause matches a name expression its caller passes in, and
// the index that should serve it is a separate statement in a migration. When
// the two drift apart the results stay right and the plan becomes a
// scan of the whole table, so no result test can notice. The proof is a plan
// test (storekit/quickfindtest.AssertIndexed) beside each caller; this gate
// makes sure every caller has one, naming the expression it actually passes.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const (
	storekitImport      = "github.com/margince/margince/backend/internal/platform/database/storekit"
	quickFindTestImport = storekitImport + "/quickfindtest"
)

// quickFindCallText counts the calls in plain text, under any package name, so
// a call the syntax walk fails to resolve shows as a mismatch.
var quickFindCallText = regexp.MustCompile(`\bQuickFindClause(With)?\(`)

// quickFindSite is one call outside storekit, with the names a plan test has
// to mention for it: the expression's identifier, or the functions that build
// the struct the expression is read from.
type quickFindSite struct {
	pos   string
	names []string
}

func TestEveryQuickFindHasAPlanTestNamingItsExpression(t *testing.T) {
	t.Parallel()
	root := filepath.Join(moduleRoot(t), "internal")
	sites, textual := quickFindSites(t, root)
	count := 0
	for _, packageSites := range sites {
		count += len(packageSites)
	}
	if count != textual {
		t.Fatalf("read %d quick-find calls from the syntax tree and %d from the text; "+
			"a call shape this gate does not recognise would go unchecked", count, textual)
	}
	if count == 0 {
		t.Fatal("found no quick-find call at all; the walk is reading the wrong tree")
	}
	for dir, packageSites := range sites {
		named := planTestNames(t, dir)
		if len(named) == 0 {
			t.Errorf("%s calls storekit.QuickFindClause but no integration test there calls "+
				"quickfindtest.AssertIndexed, so nothing proves the search reads an index", dir)
			continue
		}
		for _, site := range packageSites {
			for _, name := range site.names {
				if !named[name] {
					t.Errorf("%s reaches the quick-find through %s, and no plan test in %s names it",
						site.pos, name, dir)
				}
			}
		}
	}
}

// quickFindSites collects the calls by package directory, and how many calls
// the plain text holds.
func quickFindSites(t *testing.T, root string) (map[string][]quickFindSite, int) {
	t.Helper()
	sites := map[string][]quickFindSite{}
	textual := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// storekit defines the clause and quickfindtest renders it to explain
		// it; neither is a caller.
		if d.IsDir() && (path == filepath.Join(root, "platform/database/storekit") || d.Name() == "quickfindtest") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		textual += len(quickFindCallText.FindAll(src, -1))
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, src, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || !(callsPackageFunc(file, call.Fun, storekitImport, "QuickFindClause") ||
				callsPackageFunc(file, call.Fun, storekitImport, "QuickFindClauseWith")) {
				return true
			}
			dir := filepath.Dir(path)
			site := quickFindSite{pos: fset.Position(call.Pos()).String()}
			switch arg := call.Args[1].(type) {
			case *ast.Ident:
				site.names = []string{arg.Name}
			case *ast.SelectorExpr:
				site.names = literalBuilders(t, dir, arg.Sel.Name)
			default:
				t.Errorf("%s: the quick-find expression is neither a name nor a field, so this gate cannot "+
					"tell which index it needs", site.pos)
			}
			sites[dir] = append(sites[dir], site)
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	return sites, textual
}

// literalBuilders names the functions in dir whose struct literals set field,
// so a plan test that runs those builders explains what production passes.
func literalBuilders(t *testing.T, dir, field string) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("listing %s: %v", dir, err)
	}
	var names []string
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if ok && fn.Body != nil && setsField(fn.Body, field) {
				names = append(names, fn.Name.Name)
			}
		}
	}
	if len(names) == 0 {
		t.Errorf("%s: no function sets %s in a struct literal, so this gate cannot tell which expression "+
			"reaches the quick-find", dir, field)
	}
	return names
}

func setsField(body ast.Node, field string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if kv, ok := n.(*ast.KeyValueExpr); ok {
			if key, isIdent := kv.Key.(*ast.Ident); isIdent && key.Name == field {
				found = true
			}
		}
		return !found
	})
	return found
}

// planTestNames lists every identifier used inside the integration-test
// functions in dir that call quickfindtest.AssertIndexed. Comments and strings
// are not identifiers, so a mention there counts for nothing.
func planTestNames(t *testing.T, dir string) map[string]bool {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*_integration_test.go"))
	if err != nil {
		t.Fatalf("listing integration tests in %s: %v", dir, err)
	}
	names := map[string]bool{}
	for _, path := range files {
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !callsAssertIndexed(file, fn.Body) {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if ident, ok := n.(*ast.Ident); ok {
					names[ident.Name] = true
				}
				return true
			})
		}
	}
	return names
}

func callsAssertIndexed(file *ast.File, body ast.Node) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok && callsPackageFunc(file, call.Fun, quickFindTestImport, "AssertIndexed") {
			found = true
		}
		return !found
	})
	return found
}
