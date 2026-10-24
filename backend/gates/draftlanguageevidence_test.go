// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every draft route reads its language from the same evidence.
//
// draftfloor.Written carries the correspondence a draft's language is read
// from. A route that builds its own text for Body, or sets no Thread, brings
// back a second language rule that disagrees with the others where a user sees
// it. Body is the answered message and belongs to the reply route alone; the
// other routes pass draftcore.LanguageEvidence output as Thread.

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
		ast.Inspect(file, func(node ast.Node) bool {
			lit, ok := node.(*ast.CompositeLit)
			if !ok || !isDraftfloorWritten(lit.Type) {
				return true
			}
			sites++
			keys := map[string]bool{}
			for _, elt := range lit.Elts {
				if kv, ok := elt.(*ast.KeyValueExpr); ok {
					if key, ok := kv.Key.(*ast.Ident); ok {
						keys[key.Name] = true
					}
				}
			}
			where := fset.Position(lit.Pos()).String()
			if !keys["Thread"] {
				t.Errorf("%s: draftfloor.Written sets no Thread; fill it from draftcore.LanguageEvidence", where)
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
	if sites < writtenCallSiteFloor {
		t.Errorf("found %d draftfloor.Written literals, want at least %d: the scan reads a smaller tree than exists", sites, writtenCallSiteFloor)
	}
}

func isDraftfloorWritten(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Written" {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "draftfloor"
}
