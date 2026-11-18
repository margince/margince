// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every task an automation mints decides, where it is written, how it says
// why it exists. A starter that leaves the reason out by default files a
// task its reader cannot explain, so the field is required in every literal.
func TestEveryAutomationTaskSaysWhyItExists(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	literals := 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(fset, name, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			lit, ok := node.(*ast.CompositeLit)
			if !ok {
				return true
			}
			if ident, ok := lit.Type.(*ast.Ident); !ok || ident.Name != "taskWords" {
				return true
			}
			literals++
			for _, elt := range lit.Elts {
				if kv, ok := elt.(*ast.KeyValueExpr); ok {
					if key, ok := kv.Key.(*ast.Ident); ok && key.Name == "why" {
						return true
					}
				}
			}
			t.Errorf("%s: a task's words name no why", fset.Position(lit.Pos()))
			return true
		})
	}
	if literals < 7 {
		t.Fatalf("the census found %d task literals; the starters that mint tasks are missing from it", literals)
	}
}
