// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

// Typing the values the code census meets without type information: which
// declared type a variable or field has, and whether its Code is a carrier's.

import (
	"go/ast"
	"slices"
	"strings"
)

// typeOf answers the declared type of expr and the file that spells it. A
// field declared in one file and filled in another is then typed alike.
func (c *emittedCensus) typeOf(expr ast.Expr, unit *goUnit, depth int) (ast.Expr, *goUnit, bool) {
	if depth > 8 {
		return nil, nil, false
	}
	switch e := expr.(type) {
	case *ast.ParenExpr:
		return c.typeOf(e.X, unit, depth+1)
	case *ast.StarExpr:
		return c.typeOf(e.X, unit, depth+1)
	case *ast.Ident:
		return c.identType(e, unit)
	case *ast.TypeAssertExpr:
		return e.Type, unit, e.Type != nil
	case *ast.IndexExpr:
		container, owner, typed := c.typeOf(e.X, unit, depth+1)
		switch shape := container.(type) {
		case *ast.ArrayType:
			return shape.Elt, owner, typed
		case *ast.MapType:
			return shape.Value, owner, typed
		}
	case *ast.SelectorExpr:
		holder, owner, typed := c.typeOf(e.X, unit, depth+1)
		if !typed {
			return nil, nil, false
		}
		return c.fieldType(holder, owner, e.Sel.Name, depth+1)
	}
	return nil, nil, false
}

// identType answers the type a variable is declared, received, built or
// returned as.
func (c *emittedCensus) identType(ident *ast.Ident, unit *goUnit) (ast.Expr, *goUnit, bool) {
	if ident.Obj == nil {
		return nil, nil, false
	}
	switch decl := ident.Obj.Decl.(type) {
	case *ast.ValueSpec:
		if decl.Type != nil {
			return decl.Type, unit, true
		}
		index := slices.IndexFunc(decl.Names, func(name *ast.Ident) bool { return name.Name == ident.Name })
		if index >= 0 && index < len(decl.Values) {
			return c.valueType(decl.Values[index], unit)
		}
		return nil, nil, false
	case *ast.Field:
		return decl.Type, unit, true
	}
	if built, ok := assignedValue(ident); ok {
		return c.valueType(built, unit)
	}
	return nil, nil, false
}

// valueType answers the type a value is built or returned as.
func (c *emittedCensus) valueType(value ast.Expr, unit *goUnit) (ast.Expr, *goUnit, bool) {
	if ref, isRef := value.(*ast.UnaryExpr); isRef {
		value = ref.X
	}
	if lit, isLit := value.(*ast.CompositeLit); isLit && lit.Type != nil {
		return lit.Type, unit, true
	}
	if asserted, isAssert := value.(*ast.TypeAssertExpr); isAssert && asserted.Type != nil {
		return asserted.Type, unit, true
	}
	call, ok := value.(*ast.CallExpr)
	if !ok {
		return nil, nil, false
	}
	if generic, isGeneric := call.Fun.(*ast.IndexExpr); isGeneric {
		return generic.Index, unit, true
	}
	key, known := c.callee(unit, call.Fun)
	fn := c.funcs[key]
	if !known || fn == nil || fn.Type.Results == nil || len(fn.Type.Results.List) == 0 {
		return nil, nil, false
	}
	return fn.Type.Results.List[0].Type, c.owner[fn], true
}

// fieldType answers the type of field on a struct this census declares,
// through embedded structs. An embedded field is named by its type.
func (c *emittedCensus) fieldType(holder ast.Expr, owner *goUnit, field string, depth int) (ast.Expr, *goUnit, bool) {
	shape, declaredIn, ok := c.structOf(holder, owner)
	if !ok || depth > 8 {
		return nil, nil, false
	}
	for _, declared := range shape.Fields.List {
		for _, name := range declared.Names {
			if name.Name == field {
				return declared.Type, declaredIn, true
			}
		}
		if len(declared.Names) == 0 && embeddedFieldName(declared.Type) == field {
			return declared.Type, declaredIn, true
		}
	}
	for _, declared := range shape.Fields.List {
		if len(declared.Names) == 0 {
			if typ, in, found := c.fieldType(declared.Type, declaredIn, field, depth+1); found {
				return typ, in, true
			}
		}
	}
	return nil, nil, false
}

// structOf answers the shape a named type of this census declares.
func (c *emittedCensus) structOf(typ ast.Expr, unit *goUnit) (*ast.StructType, *goUnit, bool) {
	if star, ok := typ.(*ast.StarExpr); ok {
		typ = star.X
	}
	name, ok := c.literalType(unit, typ)
	dir, typeName, _ := strings.Cut(name, ".")
	shape := c.structs[dir][typeName]
	if !ok || shape == nil {
		return nil, nil, false
	}
	return shape, c.structAt[shape], true
}

// namedType answers what a non-struct type this census declares names.
func (c *emittedCensus) namedType(typ ast.Expr, unit *goUnit) (ast.Expr, *goUnit, bool) {
	if star, ok := typ.(*ast.StarExpr); ok {
		typ = star.X
	}
	name, ok := c.literalType(unit, typ)
	declared, known := c.named[name]
	return declared.typ, declared.unit, ok && known
}

// embeddedFieldName answers the name an embedded field is reached by.
func embeddedFieldName(typ ast.Expr) string {
	if star, ok := typ.(*ast.StarExpr); ok {
		typ = star.X
	}
	switch named := typ.(type) {
	case *ast.Ident:
		return named.Name
	case *ast.SelectorExpr:
		return named.Sel.Name
	}
	return ""
}

// holdsCarrierCode reports a type whose Code is a body carrier's: the
// carrier itself, an alias or defined type of one, or a struct embedding one.
func (c *emittedCensus) holdsCarrierCode(typ ast.Expr, unit *goUnit, depth int) bool {
	if c.isCarrier(typ, unit) {
		return true
	}
	if named, declaredIn, ok := c.namedType(typ, unit); ok && depth <= 8 {
		return c.holdsCarrierCode(named, declaredIn, depth+1)
	}
	shape, declaredIn, ok := c.structOf(typ, unit)
	if !ok || depth > 8 {
		return false
	}
	for _, declared := range shape.Fields.List {
		if len(declared.Names) == 0 && c.holdsCarrierCode(declared.Type, declaredIn, depth+1) {
			return true
		}
	}
	return false
}

// assignedValue answers the one expression a `:=` gave ident.
func assignedValue(ident *ast.Ident) (ast.Expr, bool) {
	if ident.Obj == nil {
		return nil, false
	}
	assign, ok := ident.Obj.Decl.(*ast.AssignStmt)
	if !ok || len(assign.Rhs) != 1 {
		return nil, false
	}
	return assign.Rhs[0], true
}

func assignedCall(ident *ast.Ident) (*ast.CallExpr, bool) {
	value, ok := assignedValue(ident)
	call, isCall := value.(*ast.CallExpr)
	if !ok || !isCall {
		return &ast.CallExpr{}, false
	}
	return call, true
}

func (c *emittedCensus) isCarrier(typ ast.Expr, unit *goUnit) bool {
	if star, ok := typ.(*ast.StarExpr); ok {
		typ = star.X
	}
	name, ok := c.literalType(unit, typ)
	dir, typeName, _ := strings.Cut(name, ".")
	return ok && dir == problemWriterDir && bodyCodeCarriers[typeName]
}
