// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

// How a call through an interface reaches a module method. The module is one
// type universe. So the type checker finds a promoted method, and compares a
// signature with its aliases resolved.

import (
	"go/ast"
	"go/types"
)

type auditIfaceMethod struct {
	key    string
	method *types.Func
	iface  *types.Interface
}

func (x *auditIndex) indexInterface(info *types.Info, node *ast.InterfaceType) {
	iface, ok := info.Types[node].Type.(*types.Interface)
	if !ok {
		return
	}
	for method := range iface.ExplicitMethods() {
		method := method
		x.ifaceMethods[method.Name()] = append(x.ifaceMethods[method.Name()], auditIfaceMethod{method.FullName(), method, iface})
	}
}

// indexNamed records the named types a package declares, local ones too.
func (x *auditIndex) indexNamed(info *types.Info) {
	for _, obj := range info.Defs {
		name, ok := obj.(*types.TypeName)
		if !ok || name.IsAlias() {
			continue
		}
		named, ok := name.Type().(*types.Named)
		if ok && !types.IsInterface(named) {
			x.named = append(x.named, named)
		}
	}
}

// methodExprShift is 1 when the innermost name on stack is the method of a
// method expression such as T.Write, whose first argument is the receiver.
func methodExprShift(info *types.Info, stack []ast.Node) int {
	if len(stack) < 2 {
		return 0
	}
	sel, ok := stack[len(stack)-2].(*ast.SelectorExpr)
	if !ok || sel.Sel != stack[len(stack)-1] {
		return 0
	}
	if selection := info.Selections[sel]; selection != nil && selection.Kind() == types.MethodExpr {
		return 1
	}
	return 0
}

// interfacesFor lists the interface methods a call could reach the writer
// through: those some module type implements with the writer as its method.
// A method with the writer's name and signature that no type is read to reach
// it through is returned as unread.
func (x *auditIndex) interfacesFor(key string) (reached, unread []string) {
	decl := x.decls[key]
	if decl == nil || decl.Recv == nil {
		return nil, nil
	}
	writer, ok := x.declPkg[decl].info.Defs[decl.Name].(*types.Func)
	if !ok {
		return nil, nil
	}
	for _, candidate := range x.ifaceMethods[decl.Name.Name] {
		hit := false
		for _, named := range x.implementersOf(candidate.iface) {
			hit = hit || auditMethodKey(named, candidate.method) == key
		}
		switch {
		case hit:
			reached = append(reached, candidate.key)
		case types.Identical(writer.Type(), candidate.method.Type()) && len(x.implementersOf(candidate.iface)) == 0:
			unread = append(unread, candidate.key)
		}
	}
	return reached, unread
}

// implementations lists the module methods a call to method through iface
// can run: one per module type implementing iface.
func (x *auditIndex) implementations(method *types.Func, iface *types.Interface) []string {
	var out []string
	for _, named := range x.implementersOf(iface) {
		if key := auditMethodKey(named, method); key != "" {
			out = append(out, key)
		}
	}
	return out
}

func (x *auditIndex) implementersOf(iface *types.Interface) []*types.Named {
	if found, ok := x.implementers[iface]; ok {
		return found
	}
	var found []*types.Named
	for _, named := range x.named {
		if auditImplements(named, iface) {
			found = append(found, named)
		}
	}
	x.implementers[iface] = found
	return found
}

// auditImplements reports whether a value or pointer of named has every
// method of iface, promoted ones included, with an identical signature. It
// is read by lookup, since types.Implements leaves a generic type unspecified.
func auditImplements(named *types.Named, iface *types.Interface) bool {
	for method := range iface.Methods() {
		method := method
		found, ok := auditLookupMethod(named, method)
		if !ok || !types.Identical(found.Type(), method.Type()) {
			return false
		}
	}
	return iface.NumMethods() > 0
}

func auditLookupMethod(named *types.Named, method *types.Func) (*types.Func, bool) {
	obj, _, _ := types.LookupFieldOrMethod(named, true, method.Pkg(), method.Name())
	fn, ok := obj.(*types.Func)
	return fn, ok
}

// auditMethodKey is the full name of the method named's method set runs for
// method, or "".
func auditMethodKey(named *types.Named, method *types.Func) string {
	fn, ok := auditLookupMethod(named, method)
	if !ok {
		return ""
	}
	return fn.Origin().FullName()
}
