// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

import (
	"go/ast"
	"go/constant"
	"go/types"
	"strings"
)

// Contract enums are decoded against their published closed vocabulary.
func (r historyImageReader) enumStrings(in historyImageArg, expr ast.Expr) ([]string, bool) {
	named, ok := in.pkg.info.TypeOf(expr).(*types.Named)
	if !ok || named.Obj().Pkg() == nil || !strings.HasSuffix(named.Obj().Pkg().Path(), "/internal/contracts") {
		return nil, false
	}
	scope := named.Obj().Pkg().Scope()
	var values []string
	for _, name := range scope.Names() {
		if value, ok := scope.Lookup(name).(*types.Const); ok && types.Identical(value.Type(), named) && value.Val().Kind() == constant.String {
			values = append(values, constant.StringVal(value.Val()))
		}
	}
	return values, len(values) > 0
}

// A local function's parameter belongs to its literal, not the enclosing declaration.
func (r historyImageReader) closureStrings(in historyImageArg, name *ast.Ident, depth int) ([]string, bool) {
	if in.decl == nil || depth > 32 {
		return nil, false
	}
	object := in.pkg.info.ObjectOf(name)
	var values []string
	known, found := true, false
	ast.Inspect(in.decl.Body, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for at, expr := range assignment.Rhs {
			literal, ok := expr.(*ast.FuncLit)
			if !ok || at >= len(assignment.Lhs) {
				continue
			}
			local, ok := assignment.Lhs[at].(*ast.Ident)
			if !ok {
				continue
			}
			position := -1
			index := 0
			for _, field := range literal.Type.Params.List {
				for _, parameter := range field.Names {
					if in.pkg.info.Defs[parameter] == object {
						position = index
					}
					index++
				}
			}
			if position < 0 {
				continue
			}
			ast.Inspect(in.decl.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				callee, ok := call.Fun.(*ast.Ident)
				if !ok || in.pkg.info.ObjectOf(callee) != in.pkg.info.ObjectOf(local) || position >= len(call.Args) {
					return true
				}
				got, ok := r.strings(in, call.Args[position], depth+1)
				values = append(values, got...)
				known = known && ok
				found = true
				return true
			})
		}
		return true
	})
	return values, found && known
}
