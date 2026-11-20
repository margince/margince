// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

// How the audit write census follows an entity argument to its values.

import (
	"go/ast"
	"go/token"
	"slices"
	"strconv"
)

const auditResolveDepth = 8

// resolve follows an entity argument to the string values it can hold.
func (c *auditCensus) resolve(src *auditSource, fn *ast.FuncDecl, closures []*ast.FuncType, expr ast.Expr, depth int) ([]string, bool) {
	if depth > auditResolveDepth {
		return nil, false
	}
	switch e := unconverted(expr).(type) {
	case *ast.BasicLit:
		value, err := strconv.Unquote(e.Value)
		return []string{value}, e.Kind == token.STRING && err == nil
	case *ast.BinaryExpr:
		left, okL := c.resolve(src, fn, closures, e.X, depth+1)
		right, okR := c.resolve(src, fn, closures, e.Y, depth+1)
		if e.Op != token.ADD || !okL || !okR || len(left) != 1 || len(right) != 1 {
			return nil, false
		}
		return []string{left[0] + right[0]}, true
	case *ast.Ident:
		return c.resolveName(src, fn, closures, e.Name, depth)
	case *ast.SelectorExpr:
		return c.resolveSelector(src, fn, closures, e, depth)
	case *ast.CallExpr:
		return c.resolveReturns(src, e, 0, depth)
	}
	return nil, false
}

// resolveReturns reads a call by what its body returns in the given result: a
// function, or a method of the caller's own package.
func (c *auditCensus) resolveReturns(src *auditSource, call *ast.CallExpr, result, depth int) ([]string, bool) {
	key := auditFuncKey{dir: src.dir}
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		key.name = fun.Name
	case *ast.SelectorExpr:
		key.name, key.method = fun.Sel.Name, true
		if q, isIdent := fun.X.(*ast.Ident); isIdent {
			if dir, imported := src.imports[q.Name]; imported {
				key.dir, key.method = dir, false
			}
		}
	default:
		return nil, false
	}
	decl, ok := c.funcs[key]
	if !ok || decl.Body == nil {
		return nil, false
	}
	var out []string
	resolved := true
	ast.Inspect(decl.Body, func(n ast.Node) bool {
		if _, nested := n.(*ast.FuncLit); nested || !resolved {
			return false
		}
		ret, isReturn := n.(*ast.ReturnStmt)
		if !isReturn {
			return true
		}
		if result >= len(ret.Results) {
			resolved = false
			return false
		}
		values, ok := c.resolve(c.srcOf[decl], decl, nil, ret.Results[result], depth+1)
		resolved = resolved && ok
		out = append(out, values...)
		return false
	})
	return out, resolved
}

// paramBehind is the parameter an argument hands on, through local names that
// each hold one value, or -1.
func paramBehind(fn *ast.FuncDecl, closures []*ast.FuncType, arg ast.Expr, depth int) int {
	ident, ok := unconverted(arg).(*ast.Ident)
	if fn == nil || !ok || depth > auditResolveDepth || shadowedByClosure(closures, ident.Name) {
		return -1
	}
	bindings, bound := localValues(fn, ident.Name)
	switch {
	case !bound:
		return auditParamIndex(fn, ident.Name)
	case len(bindings) == 1 && bindings[0].value != nil:
		return paramBehind(fn, closures, bindings[0].value, depth+1)
	}
	return -1
}

// shadowedByClosure reports whether a closure around the call rebinds name.
func shadowedByClosure(closures []*ast.FuncType, name string) bool {
	for _, closure := range closures {
		if fieldListNames(closure.Params)[name] {
			return true
		}
	}
	return false
}

func (c *auditCensus) resolveName(src *auditSource, fn *ast.FuncDecl, closures []*ast.FuncType, name string, depth int) ([]string, bool) {
	if shadowedByClosure(closures, name) {
		return nil, false
	}
	if fn != nil {
		if bindings, found := localValues(fn, name); found {
			var out []string
			for _, binding := range bindings {
				types, ok := c.resolveBinding(src, fn, closures, binding, depth+1)
				if !ok {
					return nil, false
				}
				out = append(out, types...)
			}
			return out, true
		}
		if i := auditParamIndex(fn, name); i >= 0 {
			return c.resolveParam(fn, i, depth)
		}
	}
	if constant, ok := c.consts[src.dir][name]; ok {
		return c.resolve(constant.src, nil, nil, constant.expr, depth+1)
	}
	return nil, false
}

// resolveParam reads a parameter by what every caller passes for it.
func (c *auditCensus) resolveParam(fn *ast.FuncDecl, index, depth int) ([]string, bool) {
	src := c.srcOf[fn]
	key := auditFuncKey{dir: src.dir, name: fn.Name.Name, method: fn.Recv != nil}
	var out []string
	callers := 0
	for _, at := range c.calls[fn.Name.Name] {
		if !c.reaches(at, key) || index >= len(at.call.Args) {
			continue
		}
		callers++
		values, ok := c.resolve(at.src, at.fn, at.closures, at.call.Args[index], depth+1)
		if !ok {
			return nil, false
		}
		out = append(out, values...)
	}
	return out, callers > 0
}

// resolveBinding reads one local binding: a value, or each element a range
// over a literal collection can bind.
func (c *auditCensus) resolveBinding(src *auditSource, fn *ast.FuncDecl, closures []*ast.FuncType, binding auditBinding, depth int) ([]string, bool) {
	if binding.call != nil {
		return c.resolveReturns(src, binding.call, binding.result, depth)
	}
	if binding.rangeOver == nil {
		return c.resolve(src, fn, closures, binding.value, depth)
	}
	ownerSrc, collection, ok := c.literalCollection(src, fn, closures, binding.rangeOver, depth)
	if !ok {
		return nil, false
	}
	var out []string
	for _, elt := range collection.Elts {
		value := elt
		if kv, isPair := elt.(*ast.KeyValueExpr); isPair {
			value = kv.Value
			if binding.rangeKey {
				value = kv.Key
			}
		} else if binding.rangeKey {
			return nil, false
		}
		types, ok := c.resolve(ownerSrc, nil, nil, value, depth+1)
		if !ok {
			return nil, false
		}
		out = append(out, types...)
	}
	return out, true
}

// literalCollection finds the composite literal a ranged name holds.
func (c *auditCensus) literalCollection(src *auditSource, fn *ast.FuncDecl, closures []*ast.FuncType, expr ast.Expr, depth int) (*auditSource, *ast.CompositeLit, bool) {
	if depth > auditResolveDepth {
		return nil, nil, false
	}
	var constant auditConst
	switch e := expr.(type) {
	case *ast.CompositeLit:
		return src, e, true
	case *ast.Ident:
		if bindings, found := localValues(fn, e.Name); fn != nil && found {
			if len(bindings) != 1 || bindings[0].value == nil || shadowedByClosure(closures, e.Name) {
				return nil, nil, false
			}
			return c.literalCollection(src, fn, closures, bindings[0].value, depth+1)
		}
		known := false
		constant, known = c.consts[src.dir][e.Name]
		if !known {
			return nil, nil, false
		}
	case *ast.SelectorExpr:
		q, isIdent := e.X.(*ast.Ident)
		dir, imported := "", false
		if isIdent {
			dir, imported = src.imports[q.Name]
		}
		known := false
		if constant, known = c.consts[dir][e.Sel.Name]; !imported || !known {
			return nil, nil, false
		}
	default:
		return nil, nil, false
	}
	return c.literalCollection(constant.src, nil, nil, constant.expr, depth+1)
}

// resolveSelector reads pkg.Name, or x.Field by every value written to that
// field of x's type. An x whose type this cannot name resolves to nothing.
func (c *auditCensus) resolveSelector(src *auditSource, fn *ast.FuncDecl, closures []*ast.FuncType, sel *ast.SelectorExpr, depth int) ([]string, bool) {
	x, ok := sel.X.(*ast.Ident)
	if !ok {
		return nil, false
	}
	if dir, imported := src.imports[x.Name]; imported {
		if constant, ok := c.consts[dir][sel.Sel.Name]; ok {
			return c.resolve(constant.src, nil, nil, constant.expr, depth+1)
		}
		return nil, false
	}
	if shadowedByClosure(closures, x.Name) {
		return nil, false
	}
	typeDir, typeName, typed := c.typeOfLocal(src, fn, x.Name)
	if !typed || c.wholeWrites[typeDir+"."+typeName] {
		return nil, false
	}
	written := slices.Clone(c.fields[typeDir+"."+typeName+"."+sel.Sel.Name])
	// Another package can write T's field without naming T only if the field is
	// exported and that package imports T's.
	for dir := range c.importers[typeDir] {
		if ast.IsExported(sel.Sel.Name) {
			written = append(written, c.fields[dir+".*."+sel.Sel.Name]...)
		}
	}
	written = append(written, c.fields[typeDir+".*."+sel.Sel.Name]...)
	if len(written) == 0 {
		return nil, false
	}
	var out []string
	for _, field := range written {
		types, ok := c.resolve(field.src, field.fn, nil, field.expr, depth+1)
		if !ok {
			return nil, false
		}
		out = append(out, types...)
	}
	return out, true
}

// localType is the declared type of a name in fn, and the source its spelling
// is read in. It comes from a receiver, a parameter, a var declaration, or the
// struct literal or call result the name is bound to.
func (c *auditCensus) localType(src *auditSource, fn *ast.FuncDecl, name string) (ast.Expr, *auditSource) {
	if fn == nil {
		return nil, nil
	}
	if fn.Recv != nil {
		for _, field := range fn.Recv.List {
			for _, n := range field.Names {
				if n.Name == name {
					return field.Type, src
				}
			}
		}
	}
	if typ := paramType(fn, name); typ != nil {
		return typ, src
	}
	var found ast.Expr
	foundIn := src
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.ValueSpec:
			for _, ident := range node.Names {
				if ident.Name == name && node.Type != nil {
					found = node.Type
				}
			}
		case *ast.AssignStmt:
			for i, lhs := range node.Lhs {
				if !isIdentNamed(lhs, name) {
					continue
				}
				if lit, ok := auditLiteralOf(auditAssignedValue(node.Rhs, len(node.Lhs), i)); ok {
					found = lit.Type
				} else if call, isCall := node.Rhs[0].(*ast.CallExpr); isCall && len(node.Rhs) == 1 {
					found, foundIn = c.resultType(src, call, i)
				}
			}
		}
		return found == nil
	})
	return found, foundIn
}

// typeOfLocal names the type of a local name, as typeOf names a type.
func (c *auditCensus) typeOfLocal(src *auditSource, fn *ast.FuncDecl, name string) (string, string, bool) {
	typ, in := c.localType(src, fn, name)
	if typ == nil {
		return "", "", false
	}
	return c.typeOf(in, typ)
}

// resultType is the declared type of a call's result at index, for a call to a
// function this census holds.
func (c *auditCensus) resultType(src *auditSource, call *ast.CallExpr, index int) (ast.Expr, *auditSource) {
	key := auditFuncKey{dir: src.dir}
	switch fun := unindexed(call.Fun).(type) {
	case *ast.Ident:
		key.name = fun.Name
	case *ast.SelectorExpr:
		q, isIdent := fun.X.(*ast.Ident)
		dir, imported := "", false
		if isIdent {
			dir, imported = src.imports[q.Name]
		}
		if !imported {
			return nil, src
		}
		key.dir, key.name = dir, fun.Sel.Name
	default:
		return nil, src
	}
	decl, ok := c.funcs[key]
	if !ok || decl.Type.Results == nil {
		return nil, src
	}
	i := 0
	for _, field := range decl.Type.Results.List {
		for range max(1, len(field.Names)) {
			if i == index {
				return field.Type, c.srcOf[decl]
			}
			i++
		}
	}
	return nil, src
}

func auditLiteralOf(expr ast.Expr) (*ast.CompositeLit, bool) {
	if unary, ok := expr.(*ast.UnaryExpr); ok && unary.Op == token.AND {
		expr = unary.X
	}
	lit, ok := expr.(*ast.CompositeLit)
	return lit, ok && lit.Type != nil
}

// unconverted strips parentheses and a string(...) conversion.
func unconverted(expr ast.Expr) ast.Expr {
	for {
		switch e := expr.(type) {
		case *ast.ParenExpr:
			expr = e.X
		case *ast.CallExpr:
			if ident, ok := e.Fun.(*ast.Ident); ok && ident.Name == "string" && len(e.Args) == 1 {
				expr = e.Args[0]
				continue
			}
			return expr
		default:
			return expr
		}
	}
}

func fieldListNames(list *ast.FieldList) map[string]bool {
	names := map[string]bool{}
	if list == nil {
		return names
	}
	for _, field := range list.List {
		for _, name := range field.Names {
			names[name.Name] = true
		}
	}
	return names
}

// auditParamIndex is the position of the named parameter in fn's call, or -1.
func auditParamIndex(fn *ast.FuncDecl, name string) int {
	i := 0
	for _, field := range fn.Type.Params.List {
		if len(field.Names) == 0 {
			i++
		}
		for _, n := range field.Names {
			if n.Name == name {
				return i
			}
			i++
		}
	}
	return -1
}

func paramType(fn *ast.FuncDecl, name string) ast.Expr {
	for _, field := range fn.Type.Params.List {
		for _, n := range field.Names {
			if n.Name == name {
				return field.Type
			}
		}
	}
	return nil
}

// auditBinding is one place a function binds a local name: a value, or the key
// or element of a range.
type auditBinding struct {
	value     ast.Expr
	rangeOver ast.Expr
	rangeKey  bool
	call      *ast.CallExpr // one of several results, by position
	result    int
}

// localValues answers every binding fn's body gives name, and whether it binds
// it at all. A binding this cannot read, such as one of several results, holds
// a nil value, which resolves to nothing.
func localValues(fn *ast.FuncDecl, name string) ([]auditBinding, bool) {
	if fn == nil || fn.Body == nil {
		return nil, false
	}
	var values []auditBinding
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.AssignStmt:
			for i, lhs := range node.Lhs {
				if !isIdentNamed(lhs, name) {
					continue
				}
				found = true
				binding := auditBinding{value: auditAssignedValue(node.Rhs, len(node.Lhs), i)}
				if call, isCall := node.Rhs[0].(*ast.CallExpr); isCall && len(node.Rhs) == 1 && len(node.Lhs) > 1 {
					binding.call, binding.result = call, i
				}
				values = append(values, binding)
			}
		case *ast.ValueSpec:
			for i, ident := range node.Names {
				// A bare declaration holds the zero value until an assignment below.
				if ident.Name == name && len(node.Values) > 0 {
					found = true
					values = append(values, auditBinding{value: auditAssignedValue(node.Values, len(node.Names), i)})
				}
			}
		case *ast.RangeStmt:
			if isIdentNamed(node.Key, name) || isIdentNamed(node.Value, name) {
				found = true
				values = append(values, auditBinding{rangeOver: node.X, rangeKey: isIdentNamed(node.Key, name)})
			}
		}
		return true
	})
	return values, found
}

func auditAssignedValue(rhs []ast.Expr, lhs, i int) ast.Expr {
	if len(rhs) != lhs {
		return nil
	}
	return rhs[i]
}

func assignedIn(fn *ast.FuncDecl, name string) bool {
	_, found := localValues(fn, name)
	return found
}
