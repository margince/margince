// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

import (
	"go/ast"
	"go/token"
	"maps"
)

type fieldCodeExpr struct {
	expr ast.Expr
	unit *goUnit
	in   *ast.FuncDecl
}

// Error helpers relay a field code through locals, tuple returns and methods.
func (c *emittedCensus) readFieldFlow(expr ast.Expr, unit *goUnit, in *ast.FuncDecl) bool {
	if c.fieldDepth > 12 {
		return false
	}
	c.fieldDepth++
	defer func() { c.fieldDepth-- }()
	switch e := expr.(type) {
	case *ast.SelectorExpr:
		return c.pluralFieldRelay(e) || c.readRouteProblemRelay(e, unit, in) || c.readIndexedField(e, unit, in)
	case *ast.Ident:
		if e.Obj == nil {
			return false
		}
		if assign, ok := e.Obj.Decl.(*ast.AssignStmt); ok && len(assign.Rhs) == 1 {
			if call, called := assign.Rhs[0].(*ast.CallExpr); called {
				for index, lhs := range assign.Lhs {
					if name, ok := lhs.(*ast.Ident); ok && name.Obj == e.Obj {
						return c.readFieldResult(call, index, unit, in)
					}
				}
			}
			c.read(assign.Rhs[0], unit, in)
			return true
		}
		found := false
		for _, assignment := range c.assigns {
			if assignment.in != in {
				continue
			}
			for index, lhs := range assignment.stmt.Lhs {
				name, ok := lhs.(*ast.Ident)
				if !ok || name.Obj != e.Obj {
					continue
				}
				if len(assignment.stmt.Rhs) == 1 {
					if call, ok := assignment.stmt.Rhs[0].(*ast.CallExpr); ok {
						found = c.readFieldResult(call, index, unit, in) || found
					}
				} else if index < len(assignment.stmt.Rhs) {
					c.read(assignment.stmt.Rhs[index], unit, in)
					found = true
				}
			}
		}
		return found
	case *ast.CallExpr:
		return c.readFieldResult(e, 0, unit, in)
	case *ast.BinaryExpr:
		if e.Op != token.ADD {
			return false
		}
		values, ok := c.fieldStringValues(e, unit, in, 0)
		if !ok {
			return false
		}
		for _, value := range values {
			if value != "" {
				c.codes[value] = c.at(unit, e)
			}
		}
		return true
	}
	return false
}

func (c *emittedCensus) readFieldResult(call *ast.CallExpr, index int, unit *goUnit, in *ast.FuncDecl) bool {
	if method, ok := call.Fun.(*ast.SelectorExpr); ok && method.Sel.Name == "FieldFault" {
		return true
	}
	key, ok := c.callee(unit, call.Fun)
	if ok && key.dir == "internal/shared/kernel/textcut" && key.name == "BytesMarked" && len(call.Args) > 0 {
		c.read(call.Args[0], unit, in)
		return true
	}
	if !ok {
		return false
	}
	if method, ok := call.Fun.(*ast.SelectorExpr); ok && key.method {
		if typ, owner, typed := c.typeOf(method.X, unit, 0); typed {
			if _, declaredIn, known := c.structOf(typ, owner); known {
				key.dir = declaredIn.dir
			}
		}
	}
	fn := c.funcs[key]
	if fn == nil || fn.Body == nil {
		return false
	}
	old := c.fieldBindings
	c.fieldBindings = maps.Clone(old)
	if c.fieldBindings == nil {
		c.fieldBindings = map[*ast.Object]fieldCodeExpr{} //nolint:staticcheck // Parser objects bind local assignments in this syntax-only census.
	}
	defer func() { c.fieldBindings = old }()
	position := 0
	for _, parameter := range fn.Type.Params.List {
		for _, name := range parameter.Names {
			if position < len(call.Args) {
				c.fieldBindings[name.Obj] = fieldCodeExpr{call.Args[position], unit, in}
			}
			position++
		}
	}
	read := false
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		ret, ok := node.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		if index < len(ret.Results) {
			c.read(ret.Results[index], c.owner[fn], fn)
			read = true
		}
		return true
	})
	return read
}

// Concatenated codes keep the finite literals their callers supply.
func (c *emittedCensus) fieldStringValues(expr ast.Expr, unit *goUnit, in *ast.FuncDecl, depth int) ([]string, bool) {
	if depth > 12 {
		return nil, false
	}
	if value, ok := c.constant(expr, unit, 0); ok {
		return []string{value}, true
	}
	switch e := expr.(type) {
	case *ast.Ident:
		if bound, ok := c.fieldBindings[e.Obj]; ok {
			return c.fieldStringValues(bound.expr, bound.unit, bound.in, depth+1)
		}
		if index, ok := codeParamIndex(in, e); ok {
			var out []string
			for _, site := range c.calls[codeFuncKey{dir: unit.dir, name: in.Name.Name, method: in.Recv != nil}] {
				if index >= len(site.call.Args) {
					return nil, false
				}
				values, known := c.fieldStringValues(site.call.Args[index], site.unit, site.in, depth+1)
				if !known {
					return nil, false
				}
				out = append(out, values...)
			}
			return out, len(out) > 0
		}
	case *ast.BinaryExpr:
		if e.Op != token.ADD {
			return nil, false
		}
		left, lok := c.fieldStringValues(e.X, unit, in, depth+1)
		right, rok := c.fieldStringValues(e.Y, unit, in, depth+1)
		if !lok || !rok {
			return nil, false
		}
		var out []string
		for _, a := range left {
			for _, b := range right {
				out = append(out, a+b)
			}
		}
		return out, true
	}
	return nil, false
}

// FieldFaults serializes the Code of its declared row type.
func (c *emittedCensus) readPluralFieldSources() {
	seen := map[string]bool{}
	for key, fn := range c.funcs {
		if !key.method || key.name != "FieldFaults" || fn.Type.Results == nil {
			continue
		}
		array, ok := fn.Type.Results.List[0].Type.(*ast.ArrayType)
		if !ok {
			continue
		}
		owner := c.owner[fn]
		name, known := c.literalType(owner, array.Elt)
		if !known || seen[name] {
			continue
		}
		seen[name] = true
		shape, _, typed := c.structOf(array.Elt, owner)
		if !typed {
			continue
		}
		for _, set := range c.composite[name] {
			if code, ok := fieldValue(set.lit, shape, "Code"); ok {
				c.read(code, set.unit, set.in)
			}
		}
	}
}

func (c *emittedCensus) pluralFieldRelay(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Code" {
		return false
	}
	base, ok := sel.X.(*ast.Ident)
	if !ok || base.Obj == nil {
		return false
	}
	assigned, ok := assignedValue(base)
	if !ok {
		return false
	}
	loop, ok := assigned.(*ast.UnaryExpr)
	if !ok || loop.Op != token.RANGE {
		return false
	}
	rows, ok := loop.X.(*ast.Ident)
	if !ok {
		return false
	}
	call, ok := assignedCall(rows)
	if !ok {
		return false
	}
	method, ok := call.Fun.(*ast.SelectorExpr)
	return ok && method.Sel.Name == "FieldFaults"
}

// A decoded route Problem can be relayed into a validation field unchanged.
func (c *emittedCensus) readRouteProblemRelay(expr ast.Expr, unit *goUnit, in *ast.FuncDecl) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Code" || in == nil {
		return false
	}
	base, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	shape, _, typed := c.typeOf(base, unit, 0)
	object, ok := shape.(*ast.StructType)
	if !typed || !ok {
		return false
	}
	tagged := map[string]bool{}
	for _, field := range object.Fields.List {
		if field.Tag != nil {
			tagged[field.Tag.Value] = true
		}
	}
	if !tagged["`json:\"code\"`"] || !tagged["`json:\"detail\"`"] {
		return false
	}
	decoded := false
	ast.Inspect(in.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || len(call.Args) != 2 {
			return true
		}
		method, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || method.Sel.Name != "Unmarshal" {
			return true
		}
		pointer, ok := call.Args[1].(*ast.UnaryExpr)
		if !ok || pointer.Op != token.AND {
			return true
		}
		target, ok := pointer.X.(*ast.Ident)
		decoded = decoded || (ok && target.Obj == base.Obj)
		return true
	})
	if !decoded {
		return false
	}
	source := newEmittedCensus(c.t, c.units)
	source.readSources()
	c.faults = append(c.faults, source.faults...)
	maps.Copy(c.codes, source.codes)
	return true
}

func (c *emittedCensus) readIndexedField(sel *ast.SelectorExpr, unit *goUnit, in *ast.FuncDecl) bool {
	base, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	assigned, ok := assignedValue(base)
	if !ok {
		return false
	}
	indexed, ok := assigned.(*ast.IndexExpr)
	if !ok {
		return false
	}
	table, ok := indexed.X.(*ast.Ident)
	if !ok || table.Obj == nil {
		return false
	}
	declaration, ok := table.Obj.Decl.(*ast.ValueSpec)
	if !ok || len(declaration.Values) != 1 {
		return false
	}
	literal, ok := declaration.Values[0].(*ast.CompositeLit)
	if !ok {
		return false
	}
	typ, ok := literal.Type.(*ast.MapType)
	if !ok {
		return false
	}
	shape, _, typed := c.structOf(typ.Value, unit)
	if !typed {
		return false
	}
	read := false
	for _, element := range literal.Elts {
		key, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		row, ok := key.Value.(*ast.CompositeLit)
		if !ok {
			continue
		}
		if value, found := fieldValue(row, shape, sel.Sel.Name); found {
			c.read(value, unit, in)
			read = true
		}
	}
	return read
}

// The parser represents a range binding as an assignment of a range expression.
func (c *emittedCensus) rangeElementType(expr ast.Expr, unit *goUnit) (ast.Expr, *goUnit, bool) {
	typ, owner, known := c.typeOf(expr, unit, 0)
	if !known {
		return nil, nil, false
	}
	for range 8 {
		switch collection := typ.(type) {
		case *ast.ArrayType:
			return collection.Elt, owner, true
		case *ast.MapType:
			return collection.Value, owner, true
		}
		named, declaredIn, ok := c.namedType(typ, owner)
		if !ok {
			return nil, nil, false
		}
		typ, owner = named, declaredIn
	}
	return nil, nil, false
}
