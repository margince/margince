// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// One writer decides whether the reindex surface exists, and it is the option
// that wires the engine, handing /me the routes' own serving predicate.
//
// A second writer is silent in both directions: /me offering a settings entry
// whose routes answer 501, or hiding a bound lane nobody can then reach. Both
// sides are booleans, so only the source can tell one writer from two.
//
// Calls are matched in the AST, so a writer split across lines, reached through
// another receiver, or placed in a sub-package of compose is still counted.
func TestOneWriterDecidesWhetherTheReindexSurfaceExists(t *testing.T) {
	const owner, ownerFunc = "embedreindextransport.go", "WithEmbedReindex"

	type writer struct{ file, fn, arg string }
	var writers []writer
	fset := token.NewFileSet()
	walkErr := filepath.WalkDir(".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		parsed, parseErr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if parseErr != nil {
			return parseErr
		}
		for _, decl := range parsed.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			ast.Inspect(fn, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "WithEmbedReindexAvailable" {
					return true
				}
				arg := ""
				if len(call.Args) == 1 {
					if predicate, isSelector := call.Args[0].(*ast.SelectorExpr); isSelector {
						arg = predicate.Sel.Name
					}
				}
				writers = append(writers, writer{file: path, fn: fn.Name.Name, arg: arg})
				return true
			})
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("scanning the compose tree: %v", walkErr)
	}

	if len(writers) != 1 {
		t.Fatalf("%d calls set /me's reindex availability (%+v), want exactly one. Two writers "+
			"of one question drift, and the drift is a surface a client offers that the server refuses.",
			len(writers), writers)
	}
	if got := writers[0]; got.file != owner || got.fn != ownerFunc || got.arg != "serving" {
		t.Errorf("/me's reindex availability is set by %+v, want %s's %s passing the handlers' serving "+
			"predicate — any other expression is a second predicate for one question",
			got, owner, ownerFunc)
	}
}
