// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestExportsAndErasureUseTheReportingLeadReach(t *testing.T) {
	for _, reader := range []struct{ file, function string }{
		{"erasure_leadtwins.go", "anonymizeLeadTwins"},
		{"sar.go", "subjectReach"},
	} {
		t.Run(reader.function, func(t *testing.T) {
			tree, err := parser.ParseFile(token.NewFileSet(), reader.file, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			for _, decl := range tree.Decls {
				function, ok := decl.(*ast.FuncDecl)
				if !ok || function.Name.Name != reader.function {
					continue
				}
				ast.Inspect(function.Body, func(node ast.Node) bool {
					call, ok := node.(*ast.CallExpr)
					if !ok {
						return true
					}
					name, ok := call.Fun.(*ast.Ident)
					if ok && name.Name == "leadTwinOf" {
						calls++
					}
					return true
				})
			}
			if calls != 1 {
				t.Fatalf("%s calls leadTwinOf %d times; exports and erasure must share the reporting predicate", reader.function, calls)
			}
		})
	}
}
