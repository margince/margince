// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

import (
	"go/ast"
	"go/token"
	"testing"
)

func outcomeDeclarations(t *testing.T, dir string) map[string][]*ast.FuncDecl {
	t.Helper()
	declarations := map[string][]*ast.FuncDecl{}
	for _, file := range parsePackageFiles(t, dir) {
		for _, declaration := range file.Decls {
			if fn, ok := declaration.(*ast.FuncDecl); ok && fn.Body != nil {
				declarations[fn.Name.Name] = append(declarations[fn.Name.Name], fn)
			}
		}
	}
	return declarations
}

func outcomeLocalCall(fn *ast.FuncDecl, call *ast.CallExpr, declarations map[string][]*ast.FuncDecl) string {
	switch called := call.Fun.(type) {
	case *ast.Ident:
		if group := declarations[called.Name]; len(group) == 1 && group[0].Recv == nil {
			return called.Name
		}
	case *ast.SelectorExpr:
		group := declarations[called.Sel.Name]
		if len(group) != 1 || group[0].Recv == nil {
			return ""
		}
		owner, ok := called.X.(*ast.Ident)
		if !ok {
			return ""
		}
		types := outcomeLocalTypes(fn, call.Pos(), declarations)
		if types[owner.Name] != "" && types[owner.Name] == receiverTypeName(group[0]) {
			return called.Sel.Name
		}
	}
	return ""
}

func outcomeLocalTypes(fn *ast.FuncDecl, before token.Pos, declarations map[string][]*ast.FuncDecl) map[string]string {
	types := map[string]string{}
	if receiver := receiverVarName(fn); receiver != "" {
		types[receiver] = receiverTypeName(fn)
	}
	for _, field := range fn.Type.Params.List {
		for _, name := range field.Names {
			types[name.Name] = outcomeTypeName(field.Type)
		}
	}
	for _, statement := range fn.Body.List {
		if statement.Pos() >= before {
			break
		}
		assignment, ok := statement.(*ast.AssignStmt)
		if !ok {
			continue
		}
		for i, left := range assignment.Lhs {
			name, ok := left.(*ast.Ident)
			if !ok {
				continue
			}
			delete(types, name.Name)
			if i >= len(assignment.Rhs) {
				continue
			}
			constructor, ok := assignment.Rhs[i].(*ast.CallExpr)
			if !ok {
				continue
			}
			called, ok := constructor.Fun.(*ast.Ident)
			if !ok {
				continue
			}
			group := declarations[called.Name]
			if len(group) != 1 || group[0].Recv != nil || group[0].Type.Results == nil {
				continue
			}
			if len(group[0].Type.Results.List) == 1 {
				types[name.Name] = outcomeTypeName(group[0].Type.Results.List[0].Type)
			}
		}
	}
	return types
}

func outcomeTypeName(expression ast.Expr) string {
	switch typed := expression.(type) {
	case *ast.Ident:
		return typed.Name
	case *ast.StarExpr:
		return outcomeTypeName(typed.X)
	}
	return ""
}
