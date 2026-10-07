// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

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

var quickFindCallText = regexp.MustCompile(`storekit\.QuickFindClause(With)?\(`)

// quickFindSite is one call outside storekit, with the identifiers whose value
// reaches the name argument.
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
		proof := planTestText(t, dir)
		if proof == "" {
			t.Errorf("%s calls storekit.QuickFindClause but no integration test there calls "+
				"quickfindtest.AssertIndexed, so nothing proves the search reads an index", dir)
			continue
		}
		for _, site := range packageSites {
			for _, name := range site.names {
				if !regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\b`).MatchString(proof) {
					t.Errorf("%s passes %s as the quick-find expression, and no plan test in %s names it",
						site.pos, name, dir)
				}
			}
		}
	}
}

// quickFindSites collects the calls by package directory, and how many calls the
// plain text holds, so a call the syntax walk misses shows as a mismatch.
func quickFindSites(t *testing.T, root string) (map[string][]quickFindSite, int) {
	t.Helper()
	sites := map[string][]quickFindSite{}
	textual := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// The plan check renders the clause to explain it; it is the proof, not a caller.
		if d.IsDir() && d.Name() == "quickfindtest" {
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
			if !ok || !isStorekitQuickFind(call) || len(call.Args) < 2 {
				return true
			}
			dir := filepath.Dir(path)
			site := quickFindSite{pos: fset.Position(call.Pos()).String()}
			switch arg := call.Args[1].(type) {
			case *ast.Ident:
				site.names = []string{arg.Name}
			case *ast.SelectorExpr:
				site.names = fieldValues(t, dir, arg.Sel.Name)
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

func isStorekitQuickFind(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "storekit" && (sel.Sel.Name == "QuickFindClause" || sel.Sel.Name == "QuickFindClauseWith")
}

// fieldValues lists the identifiers the package's struct literals put in field,
// except in a literal that also sets Query to nil: that list never searches
// through the shared clause, so its name column never reaches it.
func fieldValues(t *testing.T, dir, field string) []string {
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
		names = append(names, literalValues(file, field)...)
	}
	if len(names) == 0 {
		t.Errorf("%s: no struct literal sets %s, so this gate cannot tell which expression reaches the quick-find", dir, field)
	}
	return names
}

// planTestText is the source of every integration test in dir that calls the
// plan check, or "" when none does.
func planTestText(t *testing.T, dir string) string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*_integration_test.go"))
	if err != nil {
		t.Fatalf("listing integration tests in %s: %v", dir, err)
	}
	var proof strings.Builder
	for _, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		if strings.Contains(string(src), "quickfindtest.AssertIndexed(") {
			proof.Write(src)
		}
	}
	return proof.String()
}

// literalValues is what file's searching struct literals put in field.
func literalValues(file *ast.File, field string) []string {
	var names []string
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		value, searches := "", true
		for _, elt := range lit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, _ := kv.Key.(*ast.Ident)
			ident, _ := kv.Value.(*ast.Ident)
			switch {
			case key == nil || ident == nil:
			case key.Name == field:
				value = ident.Name
			case key.Name == "Query" && ident.Name == "nil":
				searches = false
			}
		}
		if value != "" && searches {
			names = append(names, value)
		}
		return true
	})
	return names
}
