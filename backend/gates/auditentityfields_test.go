// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

// Struct fields, read by every write the module makes to them. A field a
// decoder or scanner could fill is not read: such a write shows in no syntax.
// A decoder reaches a value handed to a parameter of interface type, or a type
// a generic function is instantiated with. A scanner reaches a field whose
// address is taken.

import (
	"go/ast"
	"go/token"
	"go/types"
)

// auditFieldWrite is one write to a field and the value it stores. The value is
// nil when the write cannot be read, as for an address taken.
type auditFieldWrite struct {
	pkg   *typedPackage
	decl  *ast.FuncDecl
	value ast.Expr
}

// auditFields is the module's field writes, and the named types a decoder
// can reach.
type auditFields struct {
	writes    map[*types.Var][]auditFieldWrite
	owner     map[*types.Var]*types.Named
	decodable map[*types.Named]bool
}

func (x *auditIndex) fields() *auditFields {
	if x.fieldIndex != nil {
		return x.fieldIndex
	}
	f := &auditFields{writes: map[*types.Var][]auditFieldWrite{}, owner: map[*types.Var]*types.Named{}, decodable: map[*types.Named]bool{}}
	for _, named := range x.named {
		f.own(named, named.Underlying())
	}
	for i := range x.pkgs {
		pkg := &x.pkgs[i]
		for _, inst := range pkg.info.Instances {
			for t := range inst.TypeArgs.Types() {
				f.markDecodable(t, map[types.Type]bool{})
			}
		}
		for _, file := range pkg.files {
			for _, decl := range file.Decls {
				fn, _ := decl.(*ast.FuncDecl)
				ast.Inspect(decl, func(n ast.Node) bool {
					f.record(pkg, fn, n)
					return true
				})
			}
		}
	}
	x.fieldIndex = f
	return f
}

// own maps each field of a struct, and of the unnamed structs inside it, to
// the named type holding it.
func (f *auditFields) own(named *types.Named, t types.Type) {
	st, ok := t.(*types.Struct)
	if !ok {
		return
	}
	for field := range st.Fields() {
		field := field.Origin()
		if _, seen := f.owner[field]; seen {
			continue
		}
		f.owner[field] = named
		f.own(named, types.Unalias(field.Type()))
	}
}

func (f *auditFields) record(pkg *typedPackage, fn *ast.FuncDecl, n ast.Node) {
	info := pkg.info
	write := func(field *types.Var, value ast.Expr) {
		f.writes[field] = append(f.writes[field], auditFieldWrite{pkg: pkg, decl: fn, value: value})
	}
	switch node := n.(type) {
	case *ast.CompositeLit:
		f.recordComposite(info, node, write)
	case *ast.AssignStmt:
		for i, lhs := range node.Lhs {
			if field := auditSelectedField(info, lhs); field != nil {
				var value ast.Expr
				if node.Tok == token.ASSIGN && len(node.Rhs) == len(node.Lhs) {
					value = node.Rhs[i]
				}
				write(field, value)
			}
		}
	case *ast.UnaryExpr:
		if field := auditSelectedField(info, node.X); field != nil && node.Op == token.AND {
			write(field, nil)
		}
	case *ast.IncDecStmt:
		if field := auditSelectedField(info, node.X); field != nil {
			write(field, nil)
		}
	case *ast.RangeStmt:
		for _, e := range []ast.Expr{node.Key, node.Value} {
			if field := auditSelectedField(info, e); e != nil && field != nil {
				write(field, nil)
			}
		}
	case *ast.CallExpr:
		f.recordDecoderArgs(info, node)
	}
}

func (f *auditFields) recordComposite(info *types.Info, lit *ast.CompositeLit, write func(*types.Var, ast.Expr)) {
	tv, ok := info.Types[lit]
	if !ok {
		return
	}
	st, ok := tv.Type.Underlying().(*types.Struct)
	if !ok {
		return
	}
	for i, elt := range lit.Elts {
		kv, keyed := elt.(*ast.KeyValueExpr)
		if !keyed {
			if i < st.NumFields() {
				write(st.Field(i).Origin(), elt)
			}
			continue
		}
		key, isIdent := kv.Key.(*ast.Ident)
		for field := range st.Fields() {
			if isIdent && field.Name() == key.Name {
				write(field.Origin(), kv.Value)
			}
		}
	}
}

// recordDecoderArgs marks the named types inside each argument a call hands
// to a parameter of interface type.
func (f *auditFields) recordDecoderArgs(info *types.Info, call *ast.CallExpr) {
	sig, ok := types.Unalias(info.Types[call.Fun].Type).(*types.Signature)
	if !ok {
		return
	}
	params := sig.Params()
	for i, arg := range call.Args {
		var param types.Type
		switch {
		case sig.Variadic() && i >= params.Len()-1 && call.Ellipsis == token.NoPos:
			slice, _ := params.At(params.Len() - 1).Type().(*types.Slice)
			if slice != nil {
				param = slice.Elem()
			}
		case i < params.Len():
			param = params.At(i).Type()
		}
		if param != nil && types.IsInterface(param) {
			f.markDecodable(info.Types[arg].Type, map[types.Type]bool{})
		}
	}
}

func (f *auditFields) markDecodable(t types.Type, seen map[types.Type]bool) {
	if t == nil || seen[t] {
		return
	}
	seen[t] = true
	switch typ := types.Unalias(t).(type) {
	case *types.Named:
		f.decodable[typ.Origin()] = true
		f.markDecodable(typ.Underlying(), seen)
	case *types.Pointer:
		f.markDecodable(typ.Elem(), seen)
	case *types.Slice:
		f.markDecodable(typ.Elem(), seen)
	case *types.Array:
		f.markDecodable(typ.Elem(), seen)
	case *types.Map:
		f.markDecodable(typ.Key(), seen)
		f.markDecodable(typ.Elem(), seen)
	case *types.Chan:
		f.markDecodable(typ.Elem(), seen)
	case *types.Struct:
		for field := range typ.Fields() {
			f.markDecodable(field.Type(), seen)
		}
	}
}

// auditSelectedField is the field e selects, or nil.
func auditSelectedField(info *types.Info, e ast.Expr) *types.Var {
	sel, ok := ast.Unparen(e).(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	field, ok := info.Uses[sel.Sel].(*types.Var)
	if !ok || !field.IsField() {
		return nil
	}
	return field.Origin()
}

// resolveField reads a field by every value the module writes to it. An
// exported field must belong to a named type no decoder reaches; an
// unexported one no decoder can fill.
func (r *auditResolver) resolveField(field *types.Var) (auditResolved, bool) {
	fields := r.index.fields()
	if field.Exported() {
		owner, known := fields.owner[field]
		if !known || fields.decodable[owner.Origin()] {
			return auditResolved{}, false
		}
	}
	writes := fields.writes[field]
	if len(writes) == 0 {
		return auditResolved{}, false
	}
	var out auditResolved
	for _, w := range writes {
		if w.value == nil {
			return auditResolved{}, false
		}
		inner := &auditResolver{index: r.index, pkg: w.pkg, decl: w.decl, forwards: true, visiting: r.visiting}
		var read auditResolved
		var ok bool
		if w.decl == nil {
			read, ok = inner.packageLevel(w.value)
		} else {
			read, ok = inner.resolve(w.value)
		}
		if !ok {
			return auditResolved{}, false
		}
		out = out.and(read)
	}
	return out, true
}
