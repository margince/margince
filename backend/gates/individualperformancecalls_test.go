// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

package gates

// How the individual-performance path walk resolves one call: what it runs,
// with which parameters bound, and whether it writes a figure to the response.

import (
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"strconv"
	"strings"
)

// refusalWriters are the response writers that answer a refusal or read the
// request, and so hand out no figure. Every other writer outside the walked
// packages counts as one that writes a figure.
var refusalWriters = map[string]bool{
	"httperr.Write": true, "httperr.NotImplemented": true, "httperr.NotImplementedBecause": true,
	"httperr.Unauthorized": true, "httperr.ServiceUnavailable": true, "httperr.Decode": true,
	"httperr.DecodeOrRefusal": true, "httperr.DecodeClaiming": true, "httperr.DecodeClosed": true,
	"httperr.DecodeJudged": true, "httperr.IfMatchVersion": true, "httperr.Unavailable": true,
	"httperr.WriteMultipartRefusal": true, "httperr.WriteNotModified": true, "modelfailure.Write": true,
	"http.Error": true, "http.NotFound": true, "http.Redirect": true, "http.SetCookie": true,
	"http.MaxBytesReader": true,
}

// successStatuses are the statuses a response with a figure is written with;
// a write naming any other status is a refusal.
var successStatuses = map[string]bool{
	"StatusOK": true, "StatusCreated": true, "StatusAccepted": true, "StatusNonAuthoritativeInfo": true,
	"StatusNoContent": true, "StatusResetContent": true, "StatusPartialContent": true, "StatusMultiStatus": true,
}

// run is one function a call runs, with its func-typed parameters bound to
// what this call site passes. A parameter bound to nil is one the walk cannot
// resolve.
type run struct {
	fn    fnValue
	bound map[*types.Var][]fnValue
}

// expr walks the calls in expr in the order they run. A literal does not run
// where it is written.
func (w *pathWalk) expr(expr ast.Expr, s pathState) pathState {
	if expr == nil {
		return s
	}
	ast.Inspect(expr, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.CallExpr:
			s = w.call(node, s)
			return false
		}
		return true
	})
	return s
}

// call walks one call after its operands. It checks when it reads the switch,
// or when everything it runs checks on every success return.
func (w *pathWalk) call(call *ast.CallExpr, s pathState) pathState {
	s = w.expr(call.Fun, s)
	for _, arg := range call.Args {
		s = w.expr(arg, s)
	}
	info := w.frame.info
	if tv, ok := info.Types[call.Fun]; ok && (tv.IsType() || tv.IsBuiltin()) {
		return s
	}
	callee := calledFunc(call, info)
	switch {
	case callee != nil && w.sw.g.readerSet[callee.Origin()]:
		s.checked = true
		return s
	case callee != nil && w.sw.g.bareSet[callee.Origin()]:
		s.bare = true
		return s
	}
	runs, resolved, external := w.runs(call, callee)
	if w.writesFigure(call, resolved && !external) {
		w.verdict.writes.add(call.Pos(), s.covered())
	}
	guards, bare := resolved && len(runs) > 0, resolved && len(runs) > 0
	for _, next := range runs {
		verdict := w.sw.walk(next.fn, next.bound, pathState{checked: s.checked, bare: s.bare}, w.frame.depth+1)
		w.verdict.writes.merge(verdict.writes)
		guards, bare = guards && verdict.guards, bare && verdict.bare
	}
	s.checked = s.checked || guards
	if !external || len(runs) > 0 {
		s.bare = !guards && bare
	}
	return s
}

// runs resolves what call runs. A function outside the walked packages runs
// the func values it is handed, and so does one on a stand-in type. That is
// how a transaction runs its body.
func (w *pathWalk) runs(call *ast.CallExpr, callee *types.Func) (runs []run, resolved, external bool) {
	info := w.frame.info
	if lit, ok := ast.Unparen(call.Fun).(*ast.FuncLit); ok {
		value := fnValue{lit: lit, env: w.frame, info: info}
		return []run{{value, w.bind(value, call)}}, true, false
	}
	var values []fnValue
	switch {
	case callee == nil && calledVar(call.Fun, info) != nil:
		values, resolved = w.varValues(calledVar(call.Fun, info))
	case callee == nil:
		return w.handedFuncs(call), true, true
	case isInterfaceMethod(callee) || w.sw.g.decls[callee.Origin()] != nil:
		values, resolved = w.funcValues(callee)
	default:
		return w.handedFuncs(call), true, true
	}
	for _, value := range values {
		runs = append(runs, run{value, w.bind(value, call)})
	}
	return runs, resolved, false
}

// handedFuncs are the func values call passes, each run where it is handed.
// One the walk cannot resolve makes the call one that does not check.
func (w *pathWalk) handedFuncs(call *ast.CallExpr) []run {
	var runs []run
	for _, arg := range call.Args {
		if _, isFunc := w.frame.info.TypeOf(arg).(*types.Signature); !isFunc {
			continue
		}
		values, ok := w.values(arg)
		if !ok {
			return nil
		}
		for _, value := range values {
			runs = append(runs, run{value, w.bind(value, call)})
		}
	}
	return runs
}

// bind maps each func-typed parameter of fn to what call passes for it.
func (w *pathWalk) bind(fn fnValue, call *ast.CallExpr) map[*types.Var][]fnValue {
	var sig *types.Signature
	if fn.lit != nil {
		sig, _ = fn.info.TypeOf(fn.lit).(*types.Signature)
	} else {
		sig, _ = fn.decl.Type().(*types.Signature)
	}
	if sig == nil {
		return nil
	}
	var bound map[*types.Var][]fnValue
	for i := range min(sig.Params().Len(), len(call.Args)) {
		param := sig.Params().At(i)
		if !isFuncTyped(param) {
			continue
		}
		if bound == nil {
			bound = map[*types.Var][]fnValue{}
		}
		values, ok := w.values(call.Args[i])
		if !ok {
			values = nil
		}
		bound[param] = values
	}
	return bound
}

// values resolves an expression of func type to the functions it may hold.
func (w *pathWalk) values(expr ast.Expr) ([]fnValue, bool) {
	info := w.frame.info
	var obj types.Object
	switch e := ast.Unparen(expr).(type) {
	case *ast.FuncLit:
		return []fnValue{{lit: e, env: w.frame, info: info}}, true
	case *ast.CallExpr:
		return w.returnedValues(e)
	case *ast.Ident:
		obj = info.Uses[e]
	case *ast.SelectorExpr:
		if sel, ok := info.Selections[e]; ok {
			obj = sel.Obj()
		} else {
			obj = info.Uses[e.Sel]
		}
	}
	switch o := obj.(type) {
	case *types.Func:
		return w.funcValues(o)
	case *types.Var:
		return w.varValues(o)
	}
	return nil, false
}

// funcValues is a declared function, or each wired implementation of an
// interface method.
func (w *pathWalk) funcValues(fn *types.Func) ([]fnValue, bool) {
	if isInterfaceMethod(fn) {
		recv, _ := fn.Type().(*types.Signature)
		iface, _ := recv.Recv().Type().Underlying().(*types.Interface)
		impls, ok := w.sw.g.implementations(iface, fn)
		values := make([]fnValue, 0, len(impls))
		for _, impl := range impls {
			values = append(values, fnValue{decl: impl})
		}
		return values, ok
	}
	if w.sw.g.decls[fn.Origin()] == nil {
		return nil, false
	}
	return []fnValue{{decl: fn.Origin()}}, true
}

// varValues is what a func-typed variable holds: what this call site bound a
// parameter to, else every value the code ever stores in it. A literal stored
// in a local keeps this frame; one stored in a field or package variable keeps
// no bindings.
func (w *pathWalk) varValues(v *types.Var) ([]fnValue, bool) {
	for frame := w.frame; frame != nil; frame = frame.parent {
		if values, ok := frame.bound[v]; ok {
			return values, values != nil
		}
	}
	origin := v.Origin()
	sites := w.sw.g.stored[origin]
	if slot, isParam := w.sw.g.paramOf[origin]; isParam && len(sites) == 0 {
		return w.argumentValues(origin, slot)
	}
	if len(sites) == 0 || w.sw.resolving[origin] {
		return nil, false
	}
	w.sw.resolving[origin] = true
	defer delete(w.sw.resolving, origin)
	local := !v.IsField() && !isPackageVar(v)
	var out []fnValue
	for _, site := range sites {
		resolver := w
		if !local {
			resolver = &pathWalk{sw: w.sw, frame: &pathFrame{info: site.info, depth: w.frame.depth}}
		}
		values, ok := resolver.values(site.expr)
		if !ok {
			return nil, false
		}
		out = append(out, values...)
	}
	return out, true
}

// returnedValues is what a call of a declared function that builds a func
// value hands back: every func value its returns name. A reader built by a
// constructor, as compose builds the tools' readers, is resolved this way.
func (w *pathWalk) returnedValues(call *ast.CallExpr) ([]fnValue, bool) {
	fn := calledFunc(call, w.frame.info)
	if fn == nil || w.sw.g.decls[fn] == nil || w.frame.depth > maxWalkDepth {
		return nil, false
	}
	decl, info := w.sw.g.decls[fn], w.sw.g.infos[fn]
	resolver := &pathWalk{sw: w.sw, frame: &pathFrame{info: info, depth: w.frame.depth + 1}}
	var out []fnValue
	resolved := true
	ast.Inspect(decl.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.ReturnStmt:
			if len(node.Results) != 1 {
				resolved = false
				return false
			}
			values, ok := resolver.values(node.Results[0])
			resolved = resolved && ok
			out = append(out, values...)
		}
		return resolved
	})
	return out, resolved && len(out) > 0
}

// argumentValues is what an unbound parameter holds: every value any call of
// its function passes for it. A constructor handing a callback to a field is
// resolved this way.
func (w *pathWalk) argumentValues(param *types.Var, slot paramSlot) ([]fnValue, bool) {
	sites := w.sw.g.calls[slot.fn]
	if len(sites) == 0 || w.sw.resolving[param] {
		return nil, false
	}
	w.sw.resolving[param] = true
	defer delete(w.sw.resolving, param)
	var out []fnValue
	for _, site := range sites {
		if slot.index >= len(site.call.Args) {
			return nil, false
		}
		resolver := &pathWalk{sw: w.sw, frame: &pathFrame{info: site.info, depth: w.frame.depth}}
		values, ok := resolver.values(site.call.Args[slot.index])
		if !ok {
			return nil, false
		}
		out = append(out, values...)
	}
	return out, true
}

// writesFigure reports a call that writes a figure to the response. That is
// w.Write, or an outside writer handed the response writer. A refusal writer
// or a refusal status writes none. A call the walk follows answers for its
// own writes.
func (w *pathWalk) writesFigure(call *ast.CallExpr, followed bool) bool {
	if sel, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr); ok && w.isWriter(sel.X) {
		return sel.Sel.Name == "Write"
	}
	if followed || !slices.ContainsFunc(call.Args, w.isWriter) {
		return false
	}
	if refusalWriters[importedCallee(call, w.frame.info)] {
		return false
	}
	return !slices.ContainsFunc(call.Args, isRefusalStatus)
}

func (w *pathWalk) isWriter(expr ast.Expr) bool {
	ident, ok := ast.Unparen(expr).(*ast.Ident)
	if !ok {
		return false
	}
	v, ok := w.frame.info.Uses[ident].(*types.Var)
	return ok && w.sw.g.writers[v]
}

// isRefusalStatus reports an http.Status constant or a literal code outside
// successStatuses.
func isRefusalStatus(arg ast.Expr) bool {
	switch e := ast.Unparen(arg).(type) {
	case *ast.SelectorExpr:
		pkg, ok := e.X.(*ast.Ident)
		return ok && pkg.Name == "http" && strings.HasPrefix(e.Sel.Name, "Status") && !successStatuses[e.Sel.Name]
	case *ast.BasicLit:
		code, err := strconv.Atoi(e.Value)
		return e.Kind == token.INT && err == nil && code >= 300
	}
	return false
}

func calledVar(fun ast.Expr, info *types.Info) *types.Var {
	switch e := ast.Unparen(fun).(type) {
	case *ast.Ident:
		v, _ := info.Uses[e].(*types.Var)
		return v
	case *ast.SelectorExpr:
		if sel, ok := info.Selections[e]; ok {
			v, _ := sel.Obj().(*types.Var)
			return v
		}
		v, _ := info.Uses[e.Sel].(*types.Var)
		return v
	}
	return nil
}

func isInterfaceMethod(fn *types.Func) bool {
	sig, ok := fn.Type().(*types.Signature)
	return ok && sig.Recv() != nil && types.IsInterface(sig.Recv().Type())
}
