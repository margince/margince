// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

//gate:kind census H2

package gates

import (
	"go/ast"
	"go/token"
	"regexp"
)

func threadDirectQueryArgument(call *ast.CallExpr) (int, bool) {
	method, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return 0, false
	}
	switch method.Sel.Name {
	case "Query", "QueryRow", "Exec":
		return 1, true
	case "Queue":
		return 0, true
	}
	return 0, false
}

func (p threadSQLPackage) queryArgument(call *ast.CallExpr, context *ast.FuncDecl) (int, bool) {
	if index, ok := threadDirectQueryArgument(call); ok {
		return index, true
	}
	target, ok := p.calledFunction(call, context)
	if !ok {
		if method, selector := call.Fun.(*ast.SelectorExpr); selector {
			if qualifier, ident := method.X.(*ast.Ident); ident {
				if owner, imported := p.imports[qualifier.Name]; imported {
					slot, registered := p.transports[owner+":"+method.Sel.Name]
					return slot, registered
				}
			}
		}
		return 0, false
	}
	index, ok := threadWrapperParameter(target)
	return index, ok
}

func threadWrapperParameter(fn *ast.FuncDecl) (int, bool) {
	params := threadParameterNames(fn)
	index := -1
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		slot, ok := threadDirectQueryArgument(call)
		if !ok || len(call.Args) <= slot {
			return true
		}
		name, ok := call.Args[slot].(*ast.Ident)
		if !ok {
			return true
		}
		for i, param := range params {
			if param == name.Name {
				index = i
			}
		}
		return true
	})
	return index, index >= 0
}

func threadParameterNames(fn *ast.FuncDecl) []string {
	var names []string
	for _, field := range fn.Type.Params.List {
		for _, name := range field.Names {
			names = append(names, name.Name)
		}
	}
	return names
}

func (p threadSQLPackage) forwardedSQLParameter(fn *ast.FuncDecl, expr ast.Expr) bool {
	slot, ok := threadWrapperParameter(fn)
	if !ok {
		return false
	}
	name, ok := expr.(*ast.Ident)
	if !ok || name.Name != threadParameterNames(fn)[slot] {
		return false
	}
	if _, registered := p.transports[p.directory+":"+scrubKey(receiverTypeName(fn), fn.Name.Name)]; registered {
		return true
	}
	if threadPureQueryTransport(fn) {
		return true
	}
	if token.IsExported(fn.Name.Name) {
		return false
	}
	called := false
	for _, caller := range p.functions {
		if called {
			break
		}
		ast.Inspect(caller.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			target, ok := p.calledFunction(call, caller)
			if ok && target == fn {
				called = true
			}
			return !called
		})
	}
	return called
}

var threadActivityMutation = regexp.MustCompile(`(?i)\b(?:UPDATE|DELETE\s+FROM)\s+activity\b`)

func (p threadSQLPackage) writeBoundary(fn *ast.FuncDecl, reached map[string]bool) bool {
	key := scrubKey(receiverTypeName(fn), fn.Name.Name)
	if reached[key] {
		return false
	}
	next := cloneThreadSQLReach(reached, key)
	guarded := false
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		name := calleeName(call)
		switch name {
		case "EnsureActivityWritable", "EnsureActivityThreadAvailable", "EnsureThreadAvailable", "lockActivityForWrite":
			guarded = true
			return false
		}
		if target, ok := p.calledFunction(call, fn); ok && p.writeBoundary(target, next) {
			guarded = true
		}
		return !guarded
	})
	return guarded
}
