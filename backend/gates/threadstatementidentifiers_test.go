// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

//gate:kind census H2

package gates

import (
	"go/ast"
	"strings"
)

func (p threadSQLPackage) identifierValues(call *ast.CallExpr, bindings map[string][]ast.Expr, reached map[string]bool, context *ast.FuncDecl, depth int) ([]threadSQLValue, bool) {
	method, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || method.Sel.Name != "Sanitize" {
		return nil, false
	}
	literal, ok := method.X.(*ast.CompositeLit)
	if !ok {
		return nil, false
	}
	shape, ok := literal.Type.(*ast.SelectorExpr)
	if !ok || shape.Sel.Name != "Identifier" {
		return nil, false
	}
	qualifier, ok := shape.X.(*ast.Ident)
	if !ok || p.imports[qualifier.Name] != "github.com/jackc/pgx/v5" {
		return nil, false
	}
	alternatives := [][]string{{}}
	for _, part := range literal.Elts {
		var next [][]string
		for _, value := range p.values(part, bindings, reached, context, depth+1) {
			names := []string{value.text}
			if strings.Contains(value.text, "?") {
				names = []string{"activity", "__catalog_identifier"}
			}
			for _, previous := range alternatives {
				for _, name := range names {
					next = append(next, append(append([]string(nil), previous...), name))
					if len(next) > threadSQLMaxAlternatives {
						return []threadSQLValue{{unresolved: true}}, true
					}
				}
			}
		}
		alternatives = next
	}
	var out []threadSQLValue
	for _, parts := range alternatives {
		for index, part := range parts {
			if !isBareIdentifier(part) {
				parts[index] = "__catalog_identifier"
			}
		}
		out = append(out, threadSQLValue{text: strings.Join(parts, ".")})
	}
	return out, true
}
