// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEveryRelayEntryPointRemainsVisibleToTheCensus(t *testing.T) {
	t.Parallel()
	directory := "internal/platform/mailer"
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	shapes := map[string]int{}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(directory, entry.Name()), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch decl := node.(type) {
			case *ast.InterfaceType:
				for _, field := range decl.Methods.List {
					method, ok := field.Type.(*ast.FuncType)
					if ok && len(field.Names) == 1 && strings.HasPrefix(field.Names[0].Name, "Send") {
						shapes[field.Names[0].Name] = parameterCount(method.Params)
					}
				}
			case *ast.FuncDecl:
				if decl.Recv == nil && strings.HasPrefix(decl.Name.Name, "Send") {
					shapes[decl.Name.Name] = parameterCount(decl.Type.Params)
				}
			}
			return true
		})
	}
	if len(shapes) < 4 {
		t.Fatalf("relay contract census found only %d entrypoints", len(shapes))
	}
	for name, arity := range shapes {
		args := strings.TrimSuffix(strings.Repeat("nil,", arity), ",")
		source := fmt.Sprintf("package example; func unratified(){ relay.%s(%s) }", name, args)
		file, err := parser.ParseFile(token.NewFileSet(), "unratified.go", source, 0)
		if err != nil {
			t.Fatal(err)
		}
		if !callsSendOnAMailerField(file) {
			t.Errorf("unratified %s with %d arguments escapes the census", name, arity)
		}
	}
}

func parameterCount(fields *ast.FieldList) int {
	count := 0
	for _, field := range fields.List {
		count += max(1, len(field.Names))
	}
	return count
}
