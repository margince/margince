// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"
)

func (r historyImageReader) strings(in historyImageArg, expr ast.Expr, depth int) ([]string, bool) {
	if depth > 32 || expr == nil {
		return nil, false
	}
	if values, known := r.metadataStrings(in, expr); known {
		return values, true
	}
	if name, ok := expr.(*ast.Ident); ok {
		if name.Name == "key" && len(r.settingKeys) > 0 && strings.HasSuffix(in.pkg.path, "/platform/settings") {
			return r.settingKeys, true
		}
		if bound, ok := in.bound[in.pkg.info.ObjectOf(name)]; ok {
			return r.strings(bound, bound.expr, depth+1)
		}
		if values, known := r.rangeStrings(in, name, depth+1); known {
			return values, true
		}
	}
	if selector, ok := ast.Unparen(expr).(*ast.SelectorExpr); ok {
		if values, known := r.storedFieldStrings(in, selector, depth+1); known {
			return values, true
		}
		if values, known := r.privateFieldStrings(in, selector, depth+1); known {
			return values, true
		}
		if values, known := r.imageFieldRead(historyImageArg{expr: selector.X, pkg: in.pkg, decl: in.decl, bound: in.bound, before: in.before}, selector.Sel.Name, depth+1, func(child historyImageArg, d int) ([]string, bool) { return r.strings(child, child.expr, d) }); known {
			return values, true
		}
	}
	if call, ok := ast.Unparen(expr).(*ast.CallExpr); ok && len(call.Args) == 1 && in.pkg.info.Types[call.Fun].IsType() {
		return r.strings(in, call.Args[0], depth+1)
	}
	if call, ok := ast.Unparen(expr).(*ast.CallExpr); ok {
		if values, known := r.returnedValues(in, call, depth+1, func(child historyImageArg, d int) ([]string, bool) { return r.strings(child, child.expr, d) }); known {
			return values, true
		}
	}
	if values, known := r.enumStrings(in, expr); known {
		return values, true
	}
	if name, ok := ast.Unparen(expr).(*ast.Ident); ok {
		if values, known := r.closureStrings(in, name, depth+1); known {
			return values, true
		}
	}
	read, known := r.index.resolver(auditFuncUse{pkg: in.pkg, decl: in.decl}).resolve(expr)
	if !known {
		return nil, false
	}
	out := read.values
	for _, param := range read.params {
		uses := r.index.uses[param.key]
		if len(uses) == 0 {
			return out, false
		}
		for _, use := range uses {
			if use.call == nil || param.param+use.shift >= len(use.call.Args) {
				return out, false
			}
			found, ok := r.strings(historyImageArg{pkg: use.pkg, decl: use.decl}, use.call.Args[param.param+use.shift], depth+1)
			if !ok {
				return out, false
			}
			out = append(out, found...)
		}
	}
	return out, len(out) > 0
}

func (r historyImageReader) rangeStrings(in historyImageArg, name *ast.Ident, depth int) ([]string, bool) {
	if in.decl == nil {
		return nil, false
	}
	object := in.pkg.info.ObjectOf(name)
	var out []string
	known := true
	found := false
	ast.Inspect(in.decl.Body, func(node ast.Node) bool {
		loop, ok := node.(*ast.RangeStmt)
		if !ok {
			return true
		}
		key, isKey := loop.Key.(*ast.Ident)
		value, isValue := loop.Value.(*ast.Ident)
		if isKey && in.pkg.info.ObjectOf(key) == object {
			if _, isMap := in.pkg.info.TypeOf(loop.X).Underlying().(*types.Map); isMap {
				child := in
				child.expr = loop.X
				keys, ok := r.keys(child, depth+1)
				out = append(out, keys...)
				known = known && ok
				found = true
			}
		}
		if isValue && in.pkg.info.ObjectOf(value) == object {
			child := in
			child.expr = loop.X
			keys, ok := r.sliceStrings(child, depth+1)
			out = append(out, keys...)
			known = known && ok
			found = true
		}
		return true
	})
	return out, found && known
}

func (r historyImageReader) sliceStrings(in historyImageArg, depth int) ([]string, bool) {
	if isNilExpr(in.pkg.info, in.expr) {
		return nil, true
	}

	if depth > 32 {
		return nil, false
	}
	switch value := ast.Unparen(in.expr).(type) {
	case *ast.CompositeLit:
		var out []string
		for _, element := range value.Elts {
			expr := element
			if pair, ok := element.(*ast.KeyValueExpr); ok {
				expr = pair.Value
			}
			keys, known := r.strings(in, expr, depth+1)
			if !known {
				return out, false
			}
			out = append(out, keys...)
		}
		return out, true
	case *ast.Ident:
		object := in.pkg.info.ObjectOf(value)
		if bound, ok := in.bound[object]; ok {
			return r.sliceStrings(bound, depth+1)
		}
		if in.decl != nil {
			if at, parameter := historyParameter(in.pkg.info, in.decl, object); parameter {
				var out []string
				known := true
				uses := r.index.uses[auditFuncKey(in.pkg.info, in.decl)]
				for _, use := range uses {
					if use.call == nil || at+use.shift >= len(use.call.Args) {
						known = false
						continue
					}
					keys, ok := r.sliceStrings(historyImageArg{expr: use.call.Args[at+use.shift], pkg: use.pkg, decl: use.decl}, depth+1)
					out = append(out, keys...)
					known = known && ok
				}
				return out, known && len(uses) > 0
			}
			var out []string
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
							keys, ok := r.sliceStrings(child, depth+1)
							out = append(out, keys...)
							known = known && ok
						}
					}
				}
				assignment, ok := node.(*ast.AssignStmt)
				if !ok || (in.before > 0 && assignment.Pos() >= in.before) {
					return true
				}
				for at, target := range assignment.Lhs {
					name, ok := target.(*ast.Ident)
					if !ok || in.pkg.info.ObjectOf(name) != object {
						continue
					}
					child := in
					child.before = assignment.Pos()
					child.result = 0
					if len(assignment.Rhs) == 1 {
						child.expr = assignment.Rhs[0]
						child.result = at
					} else if at < len(assignment.Rhs) {
						child.expr = assignment.Rhs[at]
					} else {
						known = false
						continue
					}
					keys, ok := r.sliceStrings(child, depth+1)
					out = append(out, keys...)
					known = known && ok
					found = true
				}
				return true
			})
			if found {
				return out, known
			}
		}
		for _, file := range in.pkg.files {
			var keys []string
			known := false
			ast.Inspect(file, func(node ast.Node) bool {
				spec, ok := node.(*ast.ValueSpec)
				if !ok {
					return true
				}
				for i, name := range spec.Names {
					if in.pkg.info.Defs[name] == object && i < len(spec.Values) {
						child := in
						child.expr = spec.Values[i]
						keys, known = r.sliceStrings(child, depth+1)
					}
				}
				return true
			})
			if known {
				return keys, true
			}
		}
	case *ast.SelectorExpr:
		if values, known := r.storedFieldValues(in, value, depth+1, true, r.sliceStrings); known {
			return values, true
		}
		child := in
		child.expr = value.X
		return r.imageFieldRead(child, value.Sel.Name, depth+1, r.sliceStrings)
	case *ast.CallExpr:
		if name, ok := value.Fun.(*ast.Ident); ok && name.Name == "make" {
			return nil, true
		}
		if name, ok := value.Fun.(*ast.Ident); ok && name.Name == "append" {
			var out []string
			for at, arg := range value.Args {
				child := in
				child.expr = arg
				child.result = 0
				var keys []string
				var known bool
				if at == 0 || (value.Ellipsis != token.NoPos && at == len(value.Args)-1) {
					keys, known = r.sliceStrings(child, depth+1)
				} else {
					keys, known = r.strings(in, arg, depth+1)
				}
				out = append(out, keys...)
				if !known {
					return out, false
				}
			}
			return out, true
		}
		return r.returnedValues(in, value, depth+1, r.sliceStrings)
	}
	return nil, false
}

func historyRangeRewritesOwnKeys(in historyImageArg, key ast.Expr, object types.Object) bool {
	name, ok := key.(*ast.Ident)
	if !ok || in.decl == nil {
		return false
	}
	keyObject := in.pkg.info.ObjectOf(name)
	same := false
	ast.Inspect(in.decl.Body, func(node ast.Node) bool {
		loop, ok := node.(*ast.RangeStmt)
		if !ok {
			return true
		}
		ident, ok := loop.Key.(*ast.Ident)
		if !ok || in.pkg.info.ObjectOf(ident) != keyObject {
			return true
		}
		source, ok := loop.X.(*ast.Ident)
		same = same || (ok && in.pkg.info.ObjectOf(source) == object)
		return true
	})
	return same
}
