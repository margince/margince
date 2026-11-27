// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

import (
	"go/ast"
	"go/types"
)

func (r historyImageReader) imageField(in historyImageArg, field string, depth int) ([]string, bool) {
	return r.imageFieldRead(in, field, depth, r.keys)
}

func (r historyImageReader) imageFieldRead(in historyImageArg, field string, depth int, leaf func(historyImageArg, int) ([]string, bool)) ([]string, bool) {
	if depth > 32 || in.expr == nil {
		return nil, false
	}
	switch value := ast.Unparen(in.expr).(type) {
	case *ast.UnaryExpr:
		in.expr = value.X
		return r.imageFieldRead(in, field, depth+1, leaf)
	case *ast.CompositeLit:
		for _, element := range value.Elts {
			if pair, ok := element.(*ast.KeyValueExpr); ok {
				if key, ok := pair.Key.(*ast.Ident); ok && key.Name == field {
					in.expr = pair.Value
					return leaf(in, depth+1)
				}
			}
		}
		return nil, true
	case *ast.Ident:
		object := in.pkg.info.ObjectOf(value)
		if bound, known := in.bound[object]; known {
			return r.imageFieldRead(bound, field, depth+1, leaf)
		}
		if in.decl == nil {
			return nil, false
		}
		if at, parameter := historyParameter(in.pkg.info, in.decl, object); parameter {
			var keys []string
			known := true
			uses := r.index.uses[auditFuncKey(in.pkg.info, in.decl)]
			for _, use := range uses {
				if use.call == nil || at+use.shift >= len(use.call.Args) {
					known = false
					continue
				}
				found, ok := r.imageFieldRead(historyImageArg{expr: use.call.Args[at+use.shift], pkg: use.pkg, decl: use.decl}, field, depth+1, leaf)
				keys = append(keys, found...)
				known = known && ok
			}
			return keys, known && len(uses) > 0
		}
		var keys []string
		known := true
		found := false
		ast.Inspect(in.decl.Body, func(node ast.Node) bool {
			if spec, ok := node.(*ast.ValueSpec); ok {
				for at, name := range spec.Names {
					if in.pkg.info.Defs[name] != object {
						continue
					}
					found = true
					if at < len(spec.Values) {
						child := in
						child.expr = spec.Values[at]
						child.before = spec.Pos()
						got, ok := r.imageFieldRead(child, field, depth+1, leaf)
						keys = append(keys, got...)
						known = known && ok
					}
				}
			}
			assignment, ok := node.(*ast.AssignStmt)
			if !ok || (in.before > 0 && assignment.Pos() >= in.before) {
				return true
			}
			for at, target := range assignment.Lhs {
				if selector, ok := target.(*ast.SelectorExpr); ok && selector.Sel.Name == field {
					if base, ok := selector.X.(*ast.Ident); ok && in.pkg.info.ObjectOf(base) == object && at < len(assignment.Rhs) {
						child := in
						child.before = assignment.Pos()
						child.expr = assignment.Rhs[at]
						got, ok := leaf(child, depth+1)
						keys = append(keys, got...)
						known = known && ok
						found = true
					}
				}
				name, ok := target.(*ast.Ident)
				if !ok || in.pkg.info.ObjectOf(name) != object || at >= len(assignment.Rhs) {
					continue
				}
				child := in
				child.before = assignment.Pos()
				child.expr = assignment.Rhs[at]
				got, ok := r.imageFieldRead(child, field, depth+1, leaf)
				keys = append(keys, got...)
				known = known && ok
				found = true
			}
			return true
		})
		return keys, known && found
	case *ast.CallExpr:
		callee := auditCallee(in.pkg.info, value)
		if callee == nil {
			return nil, false
		}
		fn := r.index.decls[callee.Origin().FullName()]
		if fn == nil || fn.Body == nil {
			return nil, false
		}
		pkg := r.index.declPkg[fn]
		bound := map[types.Object]historyImageArg{}
		at := 0
		for _, parameter := range fn.Type.Params.List {
			for _, name := range parameter.Names {
				if at < len(value.Args) {
					bound[pkg.info.Defs[name]] = historyImageArg{expr: value.Args[at], pkg: in.pkg, decl: in.decl, bound: in.bound, before: in.before}
				}
				at++
			}
		}
		var keys []string
		known := true
		found := false
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			if _, nested := node.(*ast.FuncLit); nested {
				return false
			}
			returned, ok := node.(*ast.ReturnStmt)
			if !ok {
				return true
			}
			if len(returned.Results) == 0 {
				known = false
				return false
			}
			got, ok := r.imageFieldRead(historyImageArg{expr: returned.Results[0], pkg: pkg, decl: fn, bound: bound}, field, depth+1, leaf)
			keys = append(keys, got...)
			known = known && ok
			found = true
			return false
		})
		return keys, known && found
	}
	return nil, false
}
