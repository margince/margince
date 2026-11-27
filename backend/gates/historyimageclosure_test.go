// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

import (
	"go/ast"
	"go/types"
)

func (r historyImageReader) closureValues(in historyImageArg, call *ast.CallExpr, depth int, read func(historyImageArg, int) ([]string, bool)) ([]string, bool) {
	if depth > 32 || in.decl == nil {
		return nil, false
	}
	name, ok := call.Fun.(*ast.Ident)
	if !ok {
		return nil, false
	}
	object := in.pkg.info.ObjectOf(name)
	var literal *ast.FuncLit
	ast.Inspect(in.decl.Body, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for at, target := range assignment.Lhs {
			ident, ok := target.(*ast.Ident)
			if ok && in.pkg.info.ObjectOf(ident) == object && at < len(assignment.Rhs) {
				literal, _ = assignment.Rhs[at].(*ast.FuncLit)
			}
		}
		return true
	})
	if literal == nil {
		return nil, false
	}
	bound := map[types.Object]historyImageArg{}
	for object, value := range in.bound {
		bound[object] = value
	}
	at := 0
	for _, field := range literal.Type.Params.List {
		for _, name := range field.Names {
			if at >= len(call.Args) {
				return nil, false
			}
			child := in
			child.expr = call.Args[at]
			bound[in.pkg.info.Defs[name]] = child
			at++
		}
	}
	var keys []string
	known, found := true, false
	ast.Inspect(literal.Body, func(node ast.Node) bool {
		if _, ok := node.(*ast.FuncLit); ok {
			return false
		}
		returned, ok := node.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		if in.result >= len(returned.Results) {
			known = false
			return false
		}
		child := in
		child.expr = returned.Results[in.result]
		child.result = 0
		child.bound = bound
		got, ok := read(child, depth+1)
		keys = append(keys, got...)
		known = known && ok
		found = true
		return false
	})
	return keys, known && found
}
