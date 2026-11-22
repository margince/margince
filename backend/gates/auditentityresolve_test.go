// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

// What an audit writer's record-type argument can hold, read off the type
// checker. It reads a constant, a conversion, and a parameter of the calling
// function. It reads a local variable or an unexported field by its
// assignments, and a call by its function's returns. It reads an element of a
// package-level list by the list's literal. Anything else is not read,
// and the census reports it.

import (
	"go/ast"
	"go/token"
	"go/types"
	"slices"
)

// auditResolved is what an expression can hold: constant values, and the
// parameters it forwards, each of which makes its function a writer.
type auditResolved struct {
	values []string
	params []auditDoor
}

// auditDoor is a writer, and the parameter its record type arrives at.
type auditDoor struct {
	key   string
	param int
}

func (r auditResolved) and(other auditResolved) auditResolved {
	return auditResolved{values: append(r.values, other.values...), params: append(r.params, other.params...)}
}

type auditResolver struct {
	index *auditIndex
	pkg   *typedPackage
	decl  *ast.FuncDecl // the function the expression sits in; nil at package level
	// forwards is false inside a callee read for its results, whose parameters
	// stand for arguments this census does not follow.
	forwards bool
	visiting map[types.Object]bool
}

func (x *auditIndex) resolver(use auditFuncUse) *auditResolver {
	return &auditResolver{index: x, pkg: use.pkg, decl: use.decl, forwards: true, visiting: map[types.Object]bool{}}
}

func (r *auditResolver) resolve(expr ast.Expr) (auditResolved, bool) {
	expr = ast.Unparen(expr)
	info := r.pkg.info
	if value, ok := auditConstString(info, expr); ok {
		return auditResolved{values: []string{value}}, true
	}
	switch node := expr.(type) {
	case *ast.CallExpr:
		if len(node.Args) == 1 && info.Types[node.Fun].IsType() {
			return r.resolve(node.Args[0])
		}
		return r.resolveResult(node, 0)
	case *ast.Ident:
		v, ok := info.Uses[node].(*types.Var)
		if !ok || r.decl == nil || v.IsField() || r.visiting[v] {
			return auditResolved{}, false
		}
		if at := r.paramAt(v); at >= 0 {
			if !r.forwards || r.changes(v) {
				return auditResolved{}, false
			}
			return auditResolved{params: []auditDoor{{auditFuncKey(info, r.decl), at}}}, true
		}
		r.visiting[v] = true
		defer delete(r.visiting, v)
		return r.resolveLocal(v)
	case *ast.IndexExpr:
		return r.packageListNamed(node.X)
	case *ast.SelectorExpr:
		field, ok := info.Uses[node.Sel].(*types.Var)
		if !ok || !field.IsField() || r.visiting[field.Origin()] {
			return auditResolved{}, false
		}
		r.visiting[field.Origin()] = true
		defer delete(r.visiting, field.Origin())
		return r.resolveField(field.Origin())
	}
	return auditResolved{}, false
}

// paramAt is v's position among decl's parameters, or -1.
func (r *auditResolver) paramAt(v *types.Var) int {
	obj, ok := r.pkg.info.Defs[r.decl.Name].(*types.Func)
	if !ok {
		return -1
	}
	params := obj.Signature().Params()
	for i := range params.Len() {
		if params.At(i) == v {
			return i
		}
	}
	return -1
}

// changes reports whether decl assigns v or lets something else assign it.
func (r *auditResolver) changes(v *types.Var) bool {
	found := false
	r.eachWrite(v, func(*ast.Ident, ast.Node) { found = true })
	return found
}

// eachWrite calls write for every place decl assigns v or takes its address,
// with the statement that does it.
func (r *auditResolver) eachWrite(v *types.Var, write func(*ast.Ident, ast.Node)) {
	info := r.pkg.info
	is := func(e ast.Expr) (*ast.Ident, bool) {
		ident, ok := ast.Unparen(e).(*ast.Ident)
		return ident, ok && (info.Uses[ident] == v || info.Defs[ident] == v)
	}
	ast.Inspect(r.decl.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.AssignStmt:
			for _, lhs := range node.Lhs {
				if ident, ok := is(lhs); ok {
					write(ident, node)
				}
			}
		case *ast.ValueSpec:
			for _, name := range node.Names {
				if ident, ok := is(name); ok {
					write(ident, node)
				}
			}
		case *ast.IncDecStmt:
			if ident, ok := is(node.X); ok {
				write(ident, node)
			}
		case *ast.UnaryExpr:
			if ident, ok := is(node.X); ok && node.Op == token.AND {
				write(ident, node)
			}
		case *ast.RangeStmt:
			for _, e := range []ast.Expr{node.Key, node.Value} {
				if ident, ok := is(e); ok && e != nil {
					write(ident, node)
				}
			}
		}
		return true
	})
}

// resolveLocal reads a variable declared in decl by every value assigned to
// it. A variable declared any other way, such as a range or a type switch
// variable or a closure's parameter, is not read.
func (r *auditResolver) resolveLocal(v *types.Var) (auditResolved, bool) {
	var out auditResolved
	declared, ok := false, true
	r.eachWrite(v, func(ident *ast.Ident, at ast.Node) {
		if r.pkg.info.Defs[ident] == v {
			declared = true
		}
		read, readable := r.assigned(ident, at)
		ok = ok && readable
		out = out.and(read)
	})
	return out, ok && declared
}

// assigned reads the value one statement gives ident.
func (r *auditResolver) assigned(ident *ast.Ident, at ast.Node) (auditResolved, bool) {
	var lhs []*ast.Ident
	var rhs []ast.Expr
	switch node := at.(type) {
	case *ast.AssignStmt:
		if node.Tok != token.ASSIGN && node.Tok != token.DEFINE {
			return auditResolved{}, false
		}
		for _, e := range node.Lhs {
			name, _ := ast.Unparen(e).(*ast.Ident)
			lhs = append(lhs, name)
		}
		rhs = node.Rhs
	case *ast.ValueSpec:
		if len(node.Values) == 0 {
			// The zero value names no record type.
			return auditResolved{}, true
		}
		lhs, rhs = node.Names, node.Values
	case *ast.RangeStmt:
		return r.rangedOver(ident, node)
	default:
		return auditResolved{}, false
	}
	at2 := slices.Index(lhs, ident)
	switch {
	case at2 < 0:
		return auditResolved{}, false
	case len(rhs) == len(lhs):
		return r.resolve(rhs[at2])
	case len(rhs) == 1:
		call, ok := ast.Unparen(rhs[0]).(*ast.CallExpr)
		if !ok {
			return auditResolved{}, false
		}
		return r.resolveResult(call, at2)
	}
	return auditResolved{}, false
}

// resolveResult reads result i of a call to a module function with a body,
// by every return statement of that body. A call through an interface reads
// every module method that implements it.
func (r *auditResolver) resolveResult(call *ast.CallExpr, i int) (auditResolved, bool) {
	callee := auditCallee(r.pkg.info, call)
	if callee == nil || r.visiting[callee] {
		return auditResolved{}, false
	}
	r.visiting[callee] = true
	defer delete(r.visiting, callee)
	keys := []string{callee.Origin().FullName()}
	if recv := callee.Signature().Recv(); recv != nil && types.IsInterface(recv.Type()) {
		iface, _ := recv.Type().Underlying().(*types.Interface)
		keys = r.index.implementations(callee, iface)
	}
	var out auditResolved
	for _, key := range keys {
		read, ok := r.returnsOf(key, i)
		if !ok {
			return auditResolved{}, false
		}
		out = out.and(read)
	}
	return out, len(keys) > 0
}

// returnsOf reads result i of the function declared under key.
func (r *auditResolver) returnsOf(key string, i int) (auditResolved, bool) {
	decl := r.index.decls[key]
	if decl == nil || decl.Body == nil {
		return auditResolved{}, false
	}
	pkg := r.index.declPkg[decl]
	obj, isFunc := pkg.info.Defs[decl.Name].(*types.Func)
	if !isFunc {
		return auditResolved{}, false
	}
	inner := &auditResolver{index: r.index, pkg: pkg, decl: decl, visiting: r.visiting}
	results := obj.Signature().Results().Len()
	var out auditResolved
	ok, returns := true, false
	ast.Inspect(decl.Body, func(n ast.Node) bool {
		if _, lit := n.(*ast.FuncLit); lit {
			return false
		}
		ret, isRet := n.(*ast.ReturnStmt)
		if !isRet || !ok {
			return ok
		}
		returns = true
		var read auditResolved
		readable := false
		switch {
		case len(ret.Results) == results:
			read, readable = inner.resolve(ret.Results[i])
		case len(ret.Results) == 1:
			if nested, isCall := ast.Unparen(ret.Results[0]).(*ast.CallExpr); isCall {
				read, readable = inner.resolveResult(nested, i)
			}
		}
		ok = ok && readable
		out = out.and(read)
		return true
	})
	return out, ok && returns
}

// auditCallee is the module function a call names directly, or nil.
func auditCallee(info *types.Info, call *ast.CallExpr) *types.Func {
	var name *ast.Ident
	switch fun := ast.Unparen(call.Fun).(type) {
	case *ast.Ident:
		name = fun
	case *ast.SelectorExpr:
		name = fun.Sel
	default:
		return nil
	}
	fn, ok := info.Uses[name].(*types.Func)
	if !ok {
		return nil
	}
	return fn
}

// packageLevel reads a value written outside every function: a constant only.
func (r *auditResolver) packageLevel(value ast.Expr) (auditResolved, bool) {
	if s, ok := auditConstString(r.pkg.info, value); ok {
		return auditResolved{values: []string{s}}, true
	}
	return auditResolved{}, false
}

// rangedOver reads a range statement's value variable over a package-level
// list. Its package must never change the list, and its initializer must list
// only constants.
func (r *auditResolver) rangedOver(ident *ast.Ident, node *ast.RangeStmt) (auditResolved, bool) {
	if node.Tok != token.DEFINE || node.Value != ident {
		return auditResolved{}, false
	}
	return r.packageListNamed(node.X)
}

// packageListNamed reads every element of the package-level slice, array or
// map that expr names.
func (r *auditResolver) packageListNamed(expr ast.Expr) (auditResolved, bool) {
	name, ok := ast.Unparen(expr).(*ast.Ident)
	if !ok {
		return auditResolved{}, false
	}
	list, ok := r.pkg.info.Uses[name].(*types.Var)
	if !ok || list.Parent() != list.Pkg().Scope() || r.visiting[list] {
		return auditResolved{}, false
	}
	r.visiting[list] = true
	defer delete(r.visiting, list)
	return r.packageList(list)
}

// packageList reads a package-level variable's elements. The list must be
// unexported, so only its package can name it. Every use there must range
// over it or read one element; any other use could change it.
func (r *auditResolver) packageList(list *types.Var) (auditResolved, bool) {
	if list.Exported() {
		return auditResolved{}, false
	}
	var init *ast.CompositeLit
	changed := false
	for _, file := range r.pkg.files {
		var stack []ast.Node
		ast.Inspect(file, func(n ast.Node) bool {
			if n == nil {
				stack = stack[:len(stack)-1]
				return true
			}
			stack = append(stack, n)
			ident, ok := n.(*ast.Ident)
			if !ok {
				return true
			}
			if r.pkg.info.Defs[ident] == list {
				init = packageListInit(stack, ident)
			} else if r.pkg.info.Uses[ident] == list && !readsOnly(stack) {
				changed = true
			}
			return true
		})
	}
	if init == nil || changed {
		return auditResolved{}, false
	}
	var out auditResolved
	for _, elt := range init.Elts {
		if kv, keyed := elt.(*ast.KeyValueExpr); keyed {
			elt = kv.Value
		}
		value, ok := r.packageLevel(elt)
		if !ok {
			return auditResolved{}, false
		}
		out = out.and(value)
	}
	return out, true
}

// packageListInit is the composite literal a package-level declaration gives
// ident, or nil.
func packageListInit(stack []ast.Node, ident *ast.Ident) *ast.CompositeLit {
	spec, ok := stack[len(stack)-2].(*ast.ValueSpec)
	if !ok {
		return nil
	}
	for i, name := range spec.Names {
		if name == ident && i < len(spec.Values) {
			lit, _ := ast.Unparen(spec.Values[i]).(*ast.CompositeLit)
			return lit
		}
	}
	return nil
}

// readsOnly reports whether the name on top of stack is ranged over, or read
// at one index that nothing assigns or takes the address of.
func readsOnly(stack []ast.Node) bool {
	name := stack[len(stack)-1]
	switch parent := stack[len(stack)-2].(type) {
	case *ast.RangeStmt:
		return parent.X == name
	case *ast.IndexExpr:
		if parent.X != name {
			return false
		}
		switch holder := stack[len(stack)-3].(type) {
		case *ast.AssignStmt:
			return !slices.Contains(holder.Lhs, ast.Expr(parent))
		case *ast.UnaryExpr:
			return holder.Op != token.AND
		case *ast.IncDecStmt:
			return false
		}
		return true
	}
	return false
}
