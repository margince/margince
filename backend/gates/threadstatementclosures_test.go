// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

//gate:kind census H2

package gates

import (
	"go/ast"
	"go/token"
	"strconv"
)

func (p threadSQLPackage) closureValues(call *ast.CallExpr, bindings map[string][]ast.Expr, reached map[string]bool, context *ast.FuncDecl, depth int) ([]threadSQLValue, bool) {
	name, ok := call.Fun.(*ast.Ident)
	if !ok {
		return nil, false
	}
	var out []threadSQLValue
	found := false
	for _, expr := range bindings[name.Name] {
		literal, ok := expr.(*ast.FuncLit)
		if !ok {
			continue
		}
		target := &ast.FuncDecl{Name: ast.NewIdent("$closure:" + strconv.Itoa(int(literal.Pos()))), Type: literal.Type, Body: literal.Body}
		if !threadSQLStringResult(target) {
			continue
		}
		found = true
		key := threadBindingIdentity(context, name.Name) + ":call"
		if reached[key] {
			return []threadSQLValue{{unresolved: true}}, true
		}
		next := cloneThreadSQLReach(reached, key)
		ast.Inspect(literal.Body, func(node ast.Node) bool {
			if _, nested := node.(*ast.FuncLit); nested {
				return false
			}
			result, ok := node.(*ast.ReturnStmt)
			if !ok || len(result.Results) == 0 {
				return true
			}
			scope := p.bindingsBefore(target, result.Pos())
			for variable, values := range bindings {
				if _, local := scope[variable]; !local {
					scope[variable] = values
				}
			}
			params := threadParameterNames(target)
			for index, param := range params {
				if index >= len(call.Args) {
					break
				}
				scope[param] = nil
				for _, value := range p.values(call.Args[index], bindings, reached, context, depth+1) {
					if value.unresolved {
						out = append(out, value)
						continue
					}
					scope[param] = append(scope[param], &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(value.text)})
				}
			}
			out = append(out, p.values(result.Results[0], scope, next, target, depth+1)...)
			return false
		})
	}
	return out, found
}
