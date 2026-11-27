// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

import (
	"go/ast"
	"go/token"
	"go/types"
	"reflect"
	"strings"
)

type historyImageArg struct {
	expr       ast.Expr
	pkg        *typedPackage
	decl       *ast.FuncDecl
	bound      map[types.Object]historyImageArg
	result     int
	patchLocal bool
	before     token.Pos
}

type historyImageReader struct {
	index           *auditIndex
	evidenceFields  map[string][]string
	reachEntities   []string
	settingKeys     []string
	fieldScanBusy   map[*types.Var]bool
	patchBusy       map[types.Object]bool
	stringFieldBusy map[*types.Var]bool
	storedFieldBusy map[*types.Var]bool
}

func historyStructKeys(typ types.Type) ([]string, bool) {
	if typ == nil {
		return nil, false
	}
	for {
		pointer, ok := types.Unalias(typ).(*types.Pointer)
		if !ok {
			break
		}
		typ = pointer.Elem()
	}
	shape, ok := typ.Underlying().(*types.Struct)
	if !ok {
		return nil, false
	}
	var keys []string
	for i := 0; i < shape.NumFields(); i++ {
		field := shape.Field(i)
		if !field.Exported() {
			continue
		}
		tag, _, _ := strings.Cut(reflect.StructTag(shape.Tag(i)).Get("json"), ",")
		if tag == "-" {
			continue
		}
		if tag != "" {
			keys = append(keys, tag)
			continue
		}
		if field.Anonymous() {
			nested, known := historyStructKeys(field.Type())
			if !known {
				return nil, false
			}
			keys = append(keys, nested...)
			continue
		}
		keys = append(keys, field.Name())
	}
	return keys, true
}

func (r historyImageReader) keys(in historyImageArg, depth int) ([]string, bool) {
	if in.expr == nil || depth > 32 {
		return nil, false
	}
	expr := ast.Unparen(in.expr)
	if isNilExpr(in.pkg.info, expr) {
		return nil, true
	}
	if keys, ok := historyStructKeys(in.pkg.info.TypeOf(expr)); ok {
		return keys, true
	}
	switch value := expr.(type) {
	case *ast.UnaryExpr:
		in.expr = value.X
		return r.keys(in, depth+1)
	case *ast.CompositeLit:
		return r.literalKeys(in, value, depth)
	case *ast.Ident:
		return r.namedKeys(in, value, depth)
	case *ast.CallExpr:
		if name, ok := value.Fun.(*ast.Ident); ok && name.Name == "make" {
			if _, ok := in.pkg.info.TypeOf(value).Underlying().(*types.Map); ok {
				return nil, true
			}
		}
		if len(value.Args) == 1 {
			if name, ok := value.Fun.(*ast.Ident); ok {
				if _, isType := in.pkg.info.ObjectOf(name).(*types.TypeName); isType {
					in.expr = value.Args[0]
					return r.keys(in, depth+1)
				}
			}
		}
		if selector, ok := value.Fun.(*ast.SelectorExpr); ok && (selector.Sel.Name == "Before" || selector.Sel.Name == "After" || selector.Sel.Name == "Moved") {
			if callee := auditCallee(in.pkg.info, value); callee != nil && strings.Contains(callee.FullName(), "/storekit.Patch).") {
				return r.patchKeys(in, value, depth)
			}
		}
		return r.returnedKeys(in, value, depth)
	case *ast.SelectorExpr:
		return r.selectedKeys(in, value, depth)
	}
	return nil, false
}

func (r historyImageReader) literalKeys(in historyImageArg, literal *ast.CompositeLit, depth int) ([]string, bool) {
	var keys []string
	for _, element := range literal.Elts {
		pair, ok := element.(*ast.KeyValueExpr)
		if !ok {
			return nil, false
		}
		found, ok := r.strings(in, pair.Key, depth+1)
		if !ok {
			return keys, false
		}
		keys = append(keys, found...)
	}
	return keys, true
}

func (r historyImageReader) namedKeys(in historyImageArg, name *ast.Ident, depth int) ([]string, bool) {
	object := in.pkg.info.ObjectOf(name)
	if bound, ok := in.bound[object]; ok {
		return r.keys(bound, depth+1)
	}
	if in.decl == nil {
		return nil, false
	}
	if at, parameter := historyParameter(in.pkg.info, in.decl, object); parameter {
		key := auditFuncKey(in.pkg.info, in.decl)
		var keys []string
		uses := r.index.uses[key]
		known := len(uses) > 0
		for _, use := range uses {
			if use.call == nil || at+use.shift >= len(use.call.Args) {
				known = false
				continue
			}
			found, ok := r.keys(historyImageArg{expr: use.call.Args[at+use.shift], pkg: use.pkg, decl: use.decl}, depth+1)
			keys = append(keys, found...)
			known = known && ok
		}
		return keys, known
	}
	var keys []string
	found, known := false, true
	ast.Inspect(in.decl.Body, func(node ast.Node) bool {
		if call, ok := node.(*ast.CallExpr); ok && len(call.Args) == 2 {
			callee := auditCallee(in.pkg.info, call)
			receiver, isName := call.Args[0].(*ast.Ident)
			if callee != nil && callee.FullName() == "maps.Copy" && isName && in.pkg.info.ObjectOf(receiver) == object {
				child := in
				child.expr = call.Args[1]
				read, ok := r.keys(child, depth+1)
				keys = append(keys, read...)
				known = known && ok
				found = true
			}
		}
		if value, ok := node.(*ast.ValueSpec); ok {
			for i, name := range value.Names {
				if in.pkg.info.Defs[name] != object {
					continue
				}
				found = true
				if i < len(value.Values) {
					child := in
					child.expr = value.Values[i]
					read, ok := r.keys(child, depth+1)
					keys = append(keys, read...)
					known = known && ok
				}
			}
		}
		assignment, ok := node.(*ast.AssignStmt)
		if !ok || (in.before > 0 && assignment.Pos() >= in.before) {
			return true
		}
		for i, target := range assignment.Lhs {
			if index, ok := target.(*ast.IndexExpr); ok {
				receiver, named := index.X.(*ast.Ident)
				if !named || in.pkg.info.ObjectOf(receiver) != object || historyRangeRewritesOwnKeys(in, index.Index, object) {
					continue
				}
				found, ok := r.strings(in, index.Index, depth+1)
				known = known && ok
				if ok {
					keys = append(keys, found...)
				}
				continue
			}
			ident, ok := target.(*ast.Ident)
			if !ok || in.pkg.info.ObjectOf(ident) != object {
				continue
			}
			child := in
			child.before = assignment.Pos()
			child.result = 0
			switch {
			case len(assignment.Rhs) == 1:
				child.expr = assignment.Rhs[0]
				child.result = i
			case i < len(assignment.Rhs):
				child.expr = assignment.Rhs[i]
			default:
				known = false
				continue
			}
			read, ok := r.keys(child, depth+1)
			keys = append(keys, read...)
			found = true
			known = known && ok
		}
		return true
	})
	return keys, found && known
}

func (r historyImageReader) returnedKeys(in historyImageArg, call *ast.CallExpr, depth int) ([]string, bool) {
	callee := auditCallee(in.pkg.info, call)
	if callee != nil && ((callee.Name() == "ChangedColumns" && callee.Pkg() != nil && strings.HasSuffix(callee.Pkg().Path(), "/storekit")) || callee.FullName() == "maps.Clone") {
		var keys []string
		known := len(call.Args) > 0
		for _, arg := range call.Args {
			child := in
			child.expr = arg
			child.result = 0
			found, ok := r.keys(child, depth+1)
			keys = append(keys, found...)
			known = known && ok
		}
		return keys, known
	}
	return r.returnedValues(in, call, depth, r.keys)
}

func (r historyImageReader) returnedValues(in historyImageArg, call *ast.CallExpr, depth int, read func(historyImageArg, int) ([]string, bool)) ([]string, bool) {
	callee := auditCallee(in.pkg.info, call)
	if callee == nil {
		return r.closureValues(in, call, depth, read)
	}
	decl := r.index.decls[callee.Origin().FullName()]
	if decl == nil || decl.Body == nil {
		return nil, false
	}
	pkg := r.index.declPkg[decl]
	bound := map[types.Object]historyImageArg{}
	if decl.Recv != nil && len(decl.Recv.List) > 0 && len(decl.Recv.List[0].Names) > 0 {
		if selector, ok := call.Fun.(*ast.SelectorExpr); ok {
			bound[pkg.info.Defs[decl.Recv.List[0].Names[0]]] = historyImageArg{expr: selector.X, pkg: in.pkg, decl: in.decl, bound: in.bound, before: in.before}
		}
	}
	at := 0
	for _, field := range decl.Type.Params.List {
		for _, name := range field.Names {
			if at >= len(call.Args) {
				return nil, false
			}
			bound[pkg.info.Defs[name]] = historyImageArg{expr: call.Args[at], pkg: in.pkg, decl: in.decl, bound: in.bound, before: in.before}
			at++
		}
	}
	child := historyImageArg{pkg: pkg, decl: decl, bound: bound}
	var keys []string
	found, known := false, true
	ast.Inspect(decl.Body, func(node ast.Node) bool {
		if _, closure := node.(*ast.FuncLit); closure {
			return false
		}
		returned, ok := node.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		if len(returned.Results) == 0 && decl.Type.Results != nil {
			names := []*ast.Ident{}
			for _, field := range decl.Type.Results.List {
				names = append(names, field.Names...)
			}
			if in.result < len(names) {
				child.expr = names[in.result]
				child.result = 0
				got, ok := read(child, depth+1)
				keys = append(keys, got...)
				found = true
				known = known && ok
				return false
			}
		}
		child.result = 0
		switch {
		case in.result < len(returned.Results):
			child.expr = returned.Results[in.result]
		case len(returned.Results) == 1:
			if _, call := returned.Results[0].(*ast.CallExpr); !call {
				known = false
				return false
			}
			child.expr = returned.Results[0]
			child.result = in.result
		default:
			known = false
			return false
		}
		got, ok := read(child, depth+1)
		keys = append(keys, got...)
		found = true
		known = known && ok
		return false
	})
	return keys, found && known
}
