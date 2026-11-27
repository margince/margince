// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

import (
	"go/ast"
	"go/types"
	"strings"
)

func historyParameter(info *types.Info, fn *ast.FuncDecl, object types.Object) (int, bool) {
	at := 0
	for _, field := range fn.Type.Params.List {
		for _, name := range field.Names {
			if info.Defs[name] == object {
				return at, true
			}
			at++
		}
	}
	return 0, false
}

func (r historyImageReader) selectedKeys(in historyImageArg, selector *ast.SelectorExpr, depth int) ([]string, bool) {
	if selector.Sel.Name == "CustomFields" {
		typ := in.pkg.info.TypeOf(selector.X)
		if pointer, ok := typ.(*types.Pointer); ok {
			typ = pointer.Elem()
		}
		if named, ok := typ.(*types.Named); ok && named.Obj().Pkg() != nil && strings.HasSuffix(named.Obj().Pkg().Path(), "/modules/activities") && (named.Obj().Name() == "LogActivityInput" || named.Obj().Name() == "UpdateActivityInput") {
			// These values are checked against fieldcatalog; History gets cf_ labels
			// from that same live catalog rather than the static core-field maps.
			return nil, true
		}
	}

	child := in
	child.expr = selector.X
	keys, known := r.imageField(child, selector.Sel.Name, depth+1)
	if known {
		return keys, true
	}
	return r.structFieldImages(in, selector, depth+1)
}

// Patch's public images hold the columns its actual setters were passed.
// Reading setter calls avoids taking the private map fields as an unknown image.
func (r historyImageReader) patchKeys(in historyImageArg, call *ast.CallExpr, depth int) ([]string, bool) {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || (selector.Sel.Name != "Before" && selector.Sel.Name != "After" && selector.Sel.Name != "Moved") {
		return nil, false
	}
	callee := auditCallee(in.pkg.info, call)
	if callee == nil || !strings.Contains(callee.FullName(), "/storekit.Patch).") {
		return nil, false
	}
	receiver := in
	receiver.expr = selector.X
	keys, known := r.patchWrites(receiver, depth+1)
	return keys, known
}

func (r historyImageReader) patchWrites(in historyImageArg, depth int) ([]string, bool) {
	if depth > 32 || in.decl == nil {
		return nil, false
	}
	if call, ok := ast.Unparen(in.expr).(*ast.CallExpr); ok {
		if callee := auditCallee(in.pkg.info, call); callee != nil && strings.Contains(callee.FullName(), "/storekit.NewPatch") {
			return nil, true
		}
		return r.returnedValues(in, call, depth+1, r.patchWrites)
	}
	if selector, ok := ast.Unparen(in.expr).(*ast.SelectorExpr); ok {
		child := in
		child.expr = selector.X
		return r.imageFieldRead(child, selector.Sel.Name, depth+1, r.patchWrites)
	}
	name, ok := ast.Unparen(in.expr).(*ast.Ident)
	if !ok {
		return nil, false
	}
	object := in.pkg.info.ObjectOf(name)
	if r.patchBusy == nil {
		r.patchBusy = map[types.Object]bool{}
	}
	if r.patchBusy[object] {
		return nil, true
	}
	r.patchBusy[object] = true
	defer delete(r.patchBusy, object)
	if value, known := in.bound[object]; known {
		return r.patchWrites(value, depth+1)
	}
	if at, parameter := historyParameter(in.pkg.info, in.decl, object); parameter && !in.patchLocal {
		var keys []string
		known := true
		uses := r.index.uses[auditFuncKey(in.pkg.info, in.decl)]
		for _, use := range uses {
			if use.call == nil || at+use.shift >= len(use.call.Args) {
				known = false
				continue
			}
			found, ok := r.patchWrites(historyImageArg{expr: use.call.Args[at+use.shift], pkg: use.pkg, decl: use.decl}, depth+1)
			keys = append(keys, found...)
			known = known && ok
		}
		return keys, known && len(uses) > 0
	}
	var keys []string
	known := true
	ast.Inspect(in.decl.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		for position, arg := range call.Args {
			name, ok := arg.(*ast.Ident)
			if !ok || in.pkg.info.ObjectOf(name) != object {
				continue
			}
			callee := auditCallee(in.pkg.info, call)
			if callee == nil {
				continue
			}
			if strings.Contains(callee.FullName(), "/storekit.SetCustomFieldPatch") {
				// Custom columns take their History labels from the live field catalog.
				continue
			}
			if strings.Contains(callee.FullName(), "/storekit.ApplyClears") && len(call.Args) > 2 {
				child := in
				child.expr = call.Args[2]
				found, ok := r.keys(child, depth+1)
				keys = append(keys, found...)
				known = known && ok
				continue
			}
			fn := r.index.decls[callee.Origin().FullName()]
			if fn == nil {
				continue
			}
			owner := r.index.declPkg[fn]
			parameters := []*ast.Ident{}
			for _, field := range fn.Type.Params.List {
				parameters = append(parameters, field.Names...)
			}
			if position >= len(parameters) {
				known = false
				continue
			}
			bound := map[types.Object]historyImageArg{}
			for i, parameter := range parameters {
				if i != position && i < len(call.Args) {
					bound[owner.info.Defs[parameter]] = historyImageArg{expr: call.Args[i], pkg: in.pkg, decl: in.decl, bound: in.bound, before: in.before}
				}
			}
			child := historyImageArg{expr: parameters[position], pkg: owner, decl: fn, bound: bound, patchLocal: true}
			found, ok := r.patchWrites(child, depth+1)
			keys = append(keys, found...)
			known = known && ok
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		receiver, ok := selector.X.(*ast.Ident)
		if !ok || in.pkg.info.ObjectOf(receiver) != object {
			return true
		}
		switch selector.Sel.Name {
		case "Set", "SetDate", "SetNullable", "SetID", "SetJSON", "SetBytes", "SetTime":
			if len(call.Args) == 0 {
				known = false
				return true
			}
			found, ok := r.strings(in, call.Args[0], depth+1)
			if !ok {
				known = false
			} else {
				keys = append(keys, found...)
			}
		}
		return true
	})
	return keys, known
}
