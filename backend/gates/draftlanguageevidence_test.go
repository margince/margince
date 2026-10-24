// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every draft route reads its language from the same evidence.
//
// draftfloor.Written carries the correspondence a draft's language is read
// from. A route that builds its own Body or Thread brings back a second
// language rule that disagrees with the others where a user sees it. Body is
// the answered message and belongs to the reply route alone. Every Thread is a
// draftcore.LanguageEvidence call, directly or through a package-local
// languageEvidence function that makes one.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

const writtenCallSiteFloor = 4

func TestEveryWrittenLiteralCarriesTheSharedLanguageEvidence(t *testing.T) {
	t.Parallel()

	root := filepath.Join(repoRoot, "backend", "internal", "compose")
	fset := token.NewFileSet()
	sites := 0
	wrapped := map[string]bool{}
	wrapperOK := map[string]bool{}
	wrapperUses := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") ||
			strings.Contains(path, string(filepath.Separator)+"draftcore"+string(filepath.Separator)) {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		imports := localImportPaths(file)
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "languageEvidence" && callsSharedEvidence(fn, imports) {
				wrapperOK[filepath.Dir(path)] = true
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			lit, ok := node.(*ast.CompositeLit)
			if !ok || !isDraftfloorWritten(lit.Type, imports) {
				return true
			}
			sites++
			keys := map[string]bool{}
			var thread ast.Expr
			for _, elt := range lit.Elts {
				if kv, ok := elt.(*ast.KeyValueExpr); ok {
					if key, ok := kv.Key.(*ast.Ident); ok {
						keys[key.Name] = true
						if key.Name == "Thread" {
							thread = kv.Value
						}
					}
				}
			}
			where := fset.Position(lit.Pos()).String()
			if !keys["Thread"] {
				t.Errorf("%s: draftfloor.Written sets no Thread; fill it from draftcore.LanguageEvidence", where)
			} else if isLocalLanguageEvidence(thread) {
				wrapped[filepath.Dir(path)] = true
				wrapperUses[filepath.Dir(path)] = where
			} else if !isSharedLanguageEvidence(thread, imports) {
				t.Errorf("%s: draftfloor.Written Thread is not a draftcore.LanguageEvidence or LanguageEvidenceOf call", where)
			}
			if keys["Body"] && filepath.Base(path) != "replydraft.go" {
				t.Errorf("%s: draftfloor.Written sets Body outside the reply route; only the answered message is a Body", where)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for dir := range wrapped {
		if !wrapperOK[dir] {
			t.Errorf("%s: Thread calls languageEvidence but no such function here calls draftcore.LanguageEvidenceOf", wrapperUses[dir])
		}
	}
	if sites < writtenCallSiteFloor {
		t.Errorf("found %d draftfloor.Written literals, want at least %d: the scan reads a smaller tree than exists", sites, writtenCallSiteFloor)
	}
}

const (
	draftfloorImport = "github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
	draftcoreImport  = "github.com/margince/margince/backend/internal/compose/draftcore"
)

// localImportPaths maps the local name of each import to its path, so an aliased
// import resolves to the same package as an unaliased one.
func localImportPaths(file *ast.File) map[string]string {
	names := map[string]string{}
	for _, spec := range file.Imports {
		path := strings.Trim(spec.Path.Value, `"`)
		name := filepath.Base(path)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		names[name] = path
	}
	return names
}

func isDraftfloorWritten(expr ast.Expr, imports map[string]string) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Written" {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && imports[pkg.Name] == draftfloorImport
}

func isSharedLanguageEvidence(expr ast.Expr, imports map[string]string) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || (sel.Sel.Name != "LanguageEvidence" && sel.Sel.Name != "LanguageEvidenceOf") {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && imports[pkg.Name] == draftcoreImport
}

func isLocalLanguageEvidence(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	fn, ok := call.Fun.(*ast.Ident)
	return ok && fn.Name == "languageEvidence"
}

func callsSharedEvidence(fn *ast.FuncDecl, imports map[string]string) bool {
	found := false
	ast.Inspect(fn, func(node ast.Node) bool {
		if call, ok := node.(*ast.CallExpr); ok && isSharedLanguageEvidence(call, imports) {
			found = true
		}
		return !found
	})
	return found
}
