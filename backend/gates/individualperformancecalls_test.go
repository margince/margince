// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

package gates

// How the individual-performance path walk resolves one call: what it runs,
// with which parameters bound, and whether it writes a figure to the response.

import (
	"go/ast"
	"go/constant"
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
// resolve. names holds each string parameter passed a constant.
type run struct {
	fn    fnValue
	bound map[*types.Var][]fnValue
	names map[*types.Var]string
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
			s = w.uncoverAssigned(node, s)
			return false
		case *ast.CallExpr:
			s = w.call(node, s)
			return false
		}
		return true
	})
	return s
}

// callAnswer is what a call answers of the switch: each result that carries
// the answer. lastErr marks the last result, for a call whose results a
// stand-in leaves unknown.
type callAnswer struct {
	results []bool
	lastErr bool
}

// at reports the answer at result i of a call read as n results.
func (a callAnswer) at(i, n int) bool {
	if a.lastErr && i == n-1 {
		return true
	}
	return len(a.results) == n && a.results[i]
}

// call walks one call after its operands. A reader's call answers the
// switch, which checks only where a condition reads that answer. So does a
// call whose error carries a check. Any other call checks when everything it
// runs checks on every success return. Its results are covered when a
// figure-free call answers them, or what it runs hands out only covered values.
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
	if callee != nil {
		if index, isReader := w.sw.g.readerSet[callee.Origin()]; isReader {
			return w.readerCall(call, callee, index, s)
		}
		if index, isFloor := w.sw.g.floorSet[callee.Origin()]; isFloor {
			w.floorCalls[call] = index
			return s
		}
		if w.sw.g.bareSet[callee.Origin()] {
			w.covers[call] = true
			return s
		}
	}
	runs, resolved, external := w.runs(call, callee)
	runs = w.dispatched(call, runs)
	if w.writesFigure(call, resolved && !external) {
		w.verdict.writes.add(call.Pos(), s.checked || w.payloadCovered(call, s))
	}
	guards, bare := resolved && len(runs) > 0, resolved && len(runs) > 0
	answer := callAnswer{}
	var captured, outer map[types.Object]bool
	var kept [][]bool
	for i, next := range runs {
		entry := pathState{checked: s.checked, covered: w.entryCovered(next.fn, call, external, s), answers: w.answeredParams(next.fn, call, s)}
		verdict := w.sw.walk(next, entry, w.frame.depth+1)
		w.verdict.writes.merge(verdict.writes)
		guards, bare = guards && verdict.guards, bare && verdict.bare
		answer.results = andAnswers(answer.results, verdict.answers, i == 0)
		captured = keptAnswers(captured, verdict.captured, i == 0)
		outer = keptAnswers(outer, verdict.outer, i == 0)
		kept = append(kept, slotKept(next.fn, verdict.kept))
	}
	for obj := range captured {
		s.answers = withMember(s.answers, obj, true)
	}
	for obj, kept := range outer {
		s.covered = withMember(s.covered, obj, kept)
	}
	covers := (external && len(runs) == 0 && w.pureCall(call, callee, s)) || (!guards && bare)
	if errLast, known := errorLast(info.TypeOf(call)); guards && (errLast || (!known && external)) {
		answer.lastErr = true
	} else {
		s.checked = s.checked || guards
	}
	w.answerCalls[call] = answer
	w.covers[call] = covers
	if external || !resolved {
		kept = nil
	}
	return w.stored(call, covers, kept, s)
}

// readerCall records what a reader answers. Some answer is neither a bool
// nor an error, as the reader's own scope is. Such a reader checks where it
// is made, unless its result is dropped.
func (w *pathWalk) readerCall(call *ast.CallExpr, reader *types.Func, index int, s pathState) pathState {
	sig, ok := reader.Type().(*types.Signature)
	if !ok || index >= sig.Results().Len() {
		return s
	}
	answer := callAnswer{results: make([]bool, sig.Results().Len())}
	answer.results[index] = true
	w.answerCalls[call] = answer
	result := sig.Results().At(index).Type()
	if !types.Identical(result, types.Typ[types.Bool]) && !types.Identical(result, errorType) && !w.discarded[call] {
		s.checked = true
	}
	return s
}

// errorLast reports whether a call's results end in an error, and false for
// known when a stand-in left its type unread.
func errorLast(t types.Type) (errLast, known bool) {
	switch results := t.(type) {
	case nil:
		return false, false
	case *types.Tuple:
		return results.Len() > 0 && types.Identical(results.At(results.Len()-1).Type(), errorType), true
	case *types.Basic:
		return false, results.Kind() != types.Invalid
	}
	return types.Identical(t, errorType), true
}

func andAnswers(acc, next []bool, first bool) []bool {
	if first {
		return slices.Clone(next)
	}
	for i := range acc {
		acc[i] = acc[i] && i < len(next) && next[i]
	}
	return acc
}

func keptAnswers(acc, next map[types.Object]bool, first bool) map[types.Object]bool {
	if first {
		return next
	}
	for obj := range acc {
		acc = withMember(acc, obj, next[obj])
	}
	return acc
}

// assign keeps, for each variable lhs names, whether it now holds an answer
// and whether its value is covered. A store through a field, index or pointer
// keeps its variable covered only when what it stores is.
func (w *pathWalk) assign(lhs, rhs []ast.Expr, tok token.Token, s pathState) pathState {
	for i, target := range lhs {
		covered := w.assignedCovered(lhs, rhs, i, s)
		if _, plain := ast.Unparen(target).(*ast.Ident); !plain || (tok != token.ASSIGN && tok != token.DEFINE) {
			if root := rootOf(target, w.frame.info); root != nil {
				s.covered = withMember(s.covered, root, s.covered[root] && covered)
			}
		}
		obj := assignedObject(target, w.frame.info)
		if obj == nil {
			continue
		}
		if _, plain := ast.Unparen(target).(*ast.Ident); plain && (tok == token.ASSIGN || tok == token.DEFINE) {
			s.covered = withMember(s.covered, obj, covered)
		}
		answered := false
		if len(rhs) == len(lhs) {
			answered = w.isAnswer(rhs[i], s)
		} else if call, ok := ast.Unparen(rhs[0]).(*ast.CallExpr); ok && len(rhs) == 1 {
			answered = w.answerCalls[call].at(i, len(lhs))
		}
		s.answers = withMember(s.answers, obj, answered)
		s.nonNil = withMember(s.nonNil, obj, false)
		delete(w.hits, obj)
		delete(w.floors, obj)
		if call, ok := ast.Unparen(rhs[0]).(*ast.CallExpr); ok && len(rhs) == 1 {
			if index, isFloor := w.floorCalls[call]; isFloor && index == i {
				w.floors[obj] = true
			}
		}
	}
	if hit := w.cacheHit(lhs, rhs); hit != nil {
		w.hits[hit] = true
	}
	return s
}

// declare walks a var block. A variable declared without a value holds its
// zero value, which is covered.
func (w *pathWalk) declare(decl *ast.DeclStmt, s pathState) pathState {
	gen, ok := decl.Decl.(*ast.GenDecl)
	if !ok {
		return s
	}
	for _, spec := range gen.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		lhs := make([]ast.Expr, len(vs.Names))
		for i, name := range vs.Names {
			lhs[i] = name
			if len(vs.Values) == 0 {
				s.covered = withMember(s.covered, w.frame.info.Defs[name], true)
			}
		}
		if len(vs.Values) > 0 {
			s = w.assign(lhs, vs.Values, token.DEFINE, s)
		}
	}
	return s
}

// assignedCovered reports whether the value lhs[i] receives is covered. One
// call answering every target covers each where its results are covered; a
// comma-ok read covers its ok, a bool.
func (w *pathWalk) assignedCovered(lhs, rhs []ast.Expr, i int, s pathState) bool {
	switch {
	case s.checked:
		return true
	case len(rhs) == len(lhs):
		return w.isCovered(rhs[i], s)
	case len(rhs) == 1 && i > 0:
		if _, isCall := ast.Unparen(rhs[0]).(*ast.CallExpr); !isCall {
			return true
		}
	}
	return len(rhs) == 1 && w.isCovered(rhs[0], s)
}

// isAnswer reports an expression that holds the switch's answer.
func (w *pathWalk) isAnswer(expr ast.Expr, s pathState) bool {
	switch e := ast.Unparen(expr).(type) {
	case *ast.Ident:
		obj := w.frame.info.Uses[e]
		return obj != nil && s.answers[obj]
	case *ast.CallExpr:
		return w.answerCalls[e].at(0, 1)
	}
	return false
}

func allBlank(lhs []ast.Expr) bool {
	return !slices.ContainsFunc(lhs, func(e ast.Expr) bool {
		ident, ok := e.(*ast.Ident)
		return !ok || ident.Name != "_"
	})
}

// runs resolves what call runs. A function outside the walked packages runs
// the func values it is handed only when the config declares it synchronous.
// A transaction is one: it runs its body before it returns. Any other may
// keep, defer, skip or repeat a callback, so none lends its check.
func (w *pathWalk) runs(call *ast.CallExpr, callee *types.Func) (runs []run, resolved, external bool) {
	info := w.frame.info
	if lit, ok := ast.Unparen(call.Fun).(*ast.FuncLit); ok {
		value := fnValue{lit: lit, env: w.frame, info: info}
		return []run{w.bind(value, call)}, true, false
	}
	var values []fnValue
	switch {
	case callee == nil && calledVar(call.Fun, info) != nil:
		values, resolved = w.varValues(calledVar(call.Fun, info))
	case callee != nil && (isInterfaceMethod(callee) || w.sw.g.decls[callee.Origin()] != nil):
		values, resolved = w.funcValues(callee)
	case w.sw.g.synchronous[w.sw.g.outsideCallee(call, info)]:
		return w.handedFuncs(call), true, true
	default:
		return nil, true, true
	}
	for _, value := range values {
		runs = append(runs, w.bind(value, call))
	}
	return runs, resolved, false
}

// handedFuncs are the func values call passes, each run where it is handed.
// One the walk cannot resolve makes the call one that does not check. The
// outside caller chooses what it hands each, so their parameters stay unbound.
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
			runs = append(runs, run{fn: value})
		}
	}
	return runs
}

// dispatched replaces a run of the tool registry's dispatcher with the Handle
// of each tool whose spec names the dispatcher's second argument. A name the
// walk cannot read as a constant leaves the dispatcher, which checks nothing.
func (w *pathWalk) dispatched(call *ast.CallExpr, runs []run) []run {
	out := make([]run, 0, len(runs))
	for _, next := range runs {
		var handlers []*types.Func
		if verb, ok := w.constantString(call, 1); ok && w.sw.g.dispatchSet[next.fn.decl] {
			handlers = w.sw.g.toolHandlers(verb)
		}
		if len(handlers) == 0 {
			out = append(out, next)
		}
		for _, handler := range handlers {
			out = append(out, run{fn: fnValue{decl: handler}})
		}
	}
	return out
}

// constantString reads a call's argument as a string constant: a literal, a
// constant, or a parameter this walk's call sites passed one.
func (w *pathWalk) constantString(call *ast.CallExpr, index int) (string, bool) {
	if index >= len(call.Args) {
		return "", false
	}
	arg := ast.Unparen(call.Args[index])
	if value := w.frame.info.Types[arg].Value; value != nil && value.Kind() == constant.String {
		return constant.StringVal(value), true
	}
	ident, ok := arg.(*ast.Ident)
	if !ok {
		return "", false
	}
	v, ok := w.frame.info.Uses[ident].(*types.Var)
	for frame := w.frame; ok && !w.sw.g.reassigned[v] && frame != nil; frame = frame.parent {
		if name, found := frame.names[v]; found {
			return name, true
		}
	}
	return "", false
}

// bind maps each func-typed parameter of fn to what call passes for it, and
// each string parameter to the constant it passes.
func (w *pathWalk) bind(fn fnValue, call *ast.CallExpr) run {
	sig := signatureOf(fn)
	out := run{fn: fn}
	if sig == nil {
		return out
	}
	for i := range min(sig.Params().Len(), len(call.Args)) {
		param := sig.Params().At(i)
		if name, ok := w.constantString(call, i); ok && types.Identical(param.Type(), types.Typ[types.String]) {
			if out.names == nil {
				out.names = map[*types.Var]string{}
			}
			out.names[param] = name
		}
		if !isFuncTyped(param) {
			continue
		}
		if out.bound == nil {
			out.bound = map[*types.Var][]fnValue{}
		}
		values, ok := w.values(call.Args[i])
		if !ok {
			values = nil
		}
		out.bound[param] = values
	}
	return out
}

func signatureOf(fn fnValue) *types.Signature {
	var sig *types.Signature
	if fn.lit != nil {
		sig, _ = fn.info.TypeOf(fn.lit).(*types.Signature)
	} else {
		sig, _ = fn.decl.Type().(*types.Signature)
	}
	return sig
}

// answeredParams are the parameters of fn that call hands an answer, so a
// helper that tests the error it is passed decides it.
func (w *pathWalk) answeredParams(fn fnValue, call *ast.CallExpr, s pathState) map[types.Object]bool {
	sig := signatureOf(fn)
	if sig == nil {
		return nil
	}
	var out map[types.Object]bool
	for i := range min(sig.Params().Len(), len(call.Args)) {
		if w.isAnswer(call.Args[i], s) {
			out = withMember(out, sig.Params().At(i), true)
		}
	}
	return out
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
			return values, values != nil && !w.sw.g.reassigned[v]
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
