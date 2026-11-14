// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

package gates

// Which values of the individual-performance walk need nothing more from the
// switch. A value is covered when it was made on a checked path. It is also
// covered when made only from covered parts: the request, a figure-free call,
// a zero value, or a pure transform of covered values. Any other value is a
// figure, so a response is judged by what it writes.

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// purePackages are the outside packages whose functions only transform what
// they are handed. A call into any other outside package, such as a database
// driver or an HTTP client, answers a figure.
var purePackages = map[string]bool{
	"bytes": true, "encoding/json": true, "errors": true, "fmt": true, "maps": true, "math": true,
	"slices": true, "sort": true, "strconv": true, "strings": true,
	"time": true, "unicode": true, "unicode/utf8": true,
}

// pureMethods are the methods that only read or reformat their receiver, as
// a request answers its context or a time its UTC form. A method a stand-in
// left untyped is known by its name alone. A context's Value is none of
// them: middleware may have put anything there.
var pureMethods = map[string]bool{
	"Before": true, "After": true, "Bytes": true, "Context": true, "Encode": true, "Equal": true,
	"Format": true, "Get": true, "IsZero": true, "Len": true, "Query": true, "String": true,
	"UTC": true, "Unix": true, "WriteString": true,
}

// isCovered reports an expression whose value is covered in s.
func (w *pathWalk) isCovered(expr ast.Expr, s pathState) bool {
	if s.checked || expr == nil {
		return true
	}
	info := w.frame.info
	expr = ast.Unparen(expr)
	if tv, ok := info.Types[expr]; ok && tv.Value != nil {
		return constantCovered(tv.Value)
	}
	if tv, ok := info.Types[expr]; ok && tv.IsType() {
		return true
	}
	switch e := expr.(type) {
	case *ast.Ident:
		return w.identCovered(info.Uses[e], s)
	case *ast.BasicLit:
		return false
	case *ast.FuncLit:
		// A callback an outside call runs may hand it anything.
		return false
	case *ast.CompositeLit:
		for _, elt := range e.Elts {
			if kv, ok := elt.(*ast.KeyValueExpr); ok {
				elt = kv.Value
			}
			if !w.isCovered(elt, s) {
				return false
			}
		}
		return true
	case *ast.UnaryExpr:
		return w.isCovered(e.X, s)
	case *ast.StarExpr:
		return w.isCovered(e.X, s)
	case *ast.SliceExpr:
		return w.isCovered(e.X, s)
	case *ast.TypeAssertExpr:
		return w.isCovered(e.X, s)
	case *ast.IndexExpr:
		return w.isCovered(e.X, s)
	case *ast.IndexListExpr:
		return w.isCovered(e.X, s)
	case *ast.SelectorExpr:
		if qualified(e, info) {
			return w.identCovered(info.Uses[e.Sel], s)
		}
		return w.isCovered(e.X, s)
	case *ast.BinaryExpr:
		return w.operandCovered(e.X, s) && w.operandCovered(e.Y, s)
	case *ast.CallExpr:
		return w.callCovered(e, s)
	}
	return false
}

// operandCovered reads a constant beside a value as covered, so a figure is
// what the other operand is: len(rows)+1 is covered where rows is.
func (w *pathWalk) operandCovered(expr ast.Expr, s pathState) bool {
	if tv, ok := w.frame.info.Types[ast.Unparen(expr)]; ok && tv.Value != nil {
		return true
	}
	return w.isCovered(expr, s)
}

func (w *pathWalk) identCovered(obj types.Object, s pathState) bool {
	switch o := obj.(type) {
	case *types.Var:
		return !isPackageVar(o) && s.covered[o]
	case *types.Const:
		return constantCovered(o.Val())
	case *types.Nil, *types.Func, *types.TypeName, *types.Builtin, *types.PkgName:
		return true
	}
	return false
}

// constantCovered reads a number other than zero as a figure. A name, a flag
// or a zero carries no one's work.
func constantCovered(value constant.Value) bool {
	switch value.Kind() {
	case constant.Int, constant.Float, constant.Complex:
		return constant.Sign(value) == 0
	}
	return true
}

// callCovered is a conversion or builtin over covered values, or a call whose
// results the walk found covered.
func (w *pathWalk) callCovered(call *ast.CallExpr, s pathState) bool {
	tv, ok := w.frame.info.Types[call.Fun]
	if ok && (tv.IsType() || tv.IsBuiltin()) {
		for _, arg := range call.Args {
			if !w.isCovered(arg, s) {
				return false
			}
		}
		return true
	}
	return w.covers[call]
}

// pureCall reports an outside call that only transforms covered values. That
// is a pure package's function, a reader of the request, or a pure method of
// a covered value, each handed nothing else. A database handle is never covered, so a query
// on one answers a figure.
func (w *pathWalk) pureCall(call *ast.CallExpr, callee *types.Func, s pathState) bool {
	sel, isSel := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	method := isSel && !qualified(sel, w.frame.info)
	pure := refusalWriters[importedCallee(call, w.frame.info)] || purePackages[calleePackage(call, callee, w.frame.info)]
	if method {
		pure = pureMethods[sel.Sel.Name]
	}
	if !pure {
		return false
	}
	if method && !w.isCovered(sel.X, s) {
		return false
	}
	for _, arg := range call.Args {
		if !w.isCovered(arg, s) && !w.isWriter(arg) {
			return false
		}
	}
	return true
}

// calleePackage is the import path of the package call's function is
// declared in, read off the import where a stand-in left the callee untyped.
func calleePackage(call *ast.CallExpr, callee *types.Func, info *types.Info) string {
	if callee != nil && callee.Pkg() != nil {
		return callee.Pkg().Path()
	}
	if sel, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr); ok && qualified(sel, info) {
		if pkg, ok := info.Uses[ast.Unparen(sel.X).(*ast.Ident)].(*types.PkgName); ok {
			return pkg.Imported().Path()
		}
	}
	return ""
}

// payloadCovered reports a write whose every argument besides the response
// writer and its status is covered.
func (w *pathWalk) payloadCovered(call *ast.CallExpr, s pathState) bool {
	for _, arg := range call.Args {
		if w.isWriter(arg) || isStatus(arg) {
			continue
		}
		if !w.isCovered(arg, s) {
			return false
		}
	}
	return true
}

// isStatus reports an http.Status constant or a literal code.
func isStatus(arg ast.Expr) bool {
	switch e := ast.Unparen(arg).(type) {
	case *ast.SelectorExpr:
		pkg, ok := e.X.(*ast.Ident)
		return ok && pkg.Name == "http" && strings.HasPrefix(e.Sel.Name, "Status")
	case *ast.BasicLit:
		code, err := strconv.Atoi(e.Value)
		return e.Kind == token.INT && err == nil && code >= 100 && code < 600
	}
	return false
}

// resultsCovered reports a return whose values besides its error are covered.
func (w *pathWalk) resultsCovered(results []ast.Expr, s pathState) bool {
	values := w.shape.results
	if w.shape.errLast {
		values--
	}
	switch {
	case s.checked || values <= 0:
		return true
	case len(results) == 0 && len(w.shape.named) < values:
		return false
	case len(results) == 0:
		for _, obj := range w.shape.named[:values] {
			if obj == nil || !s.covered[obj] {
				return false
			}
		}
		return true
	case len(results) == 1 && w.shape.results > 1:
		return w.isCovered(results[0], s)
	}
	for _, result := range results[:min(values, len(results))] {
		if !w.isCovered(result, s) {
			return false
		}
	}
	return true
}

// entryCovered is what a run of fn starts with covered. That is each
// parameter call passes a covered value, and for a literal its frame's. A
// callback handed to an outside callee gets its parameters from that callee,
// as a transaction hands its body the transaction, so none is covered.
func (w *pathWalk) entryCovered(fn fnValue, call *ast.CallExpr, handed bool, s pathState) map[types.Object]bool {
	var out map[types.Object]bool
	if fn.lit != nil {
		out = maps.Clone(s.covered)
	}
	sig := signatureOf(fn)
	if sig == nil || handed {
		return out
	}
	if sel, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr); ok && sig.Recv() != nil && w.isCovered(sel.X, s) {
		out = withMember(out, sig.Recv(), true)
	}
	params := sig.Params()
	for i := range params.Len() {
		args := call.Args[min(i, len(call.Args)):min(i+1, len(call.Args))]
		if sig.Variadic() && i == params.Len()-1 {
			args = call.Args[min(i, len(call.Args)):]
		}
		covered := true
		for _, arg := range args {
			covered = covered && w.isCovered(arg, s)
		}
		if covered {
			out = withMember(out, params.At(i), true)
		}
	}
	return out
}

// rootCovered is what a handler starts with covered: its parameters, which
// hold the request. The receiver holds what outlives one request, as a cache.
func rootCovered(fn *types.Func) map[types.Object]bool {
	out := map[types.Object]bool{}
	sig, ok := fn.Type().(*types.Signature)
	if !ok {
		return out
	}
	for i := range sig.Params().Len() {
		out[sig.Params().At(i)] = true
	}
	return out
}

// coveredMask packs which of fn's receiver and parameters start covered, so a
// verdict is reused only from an entry that covers the same ones.
func coveredMask(fn fnValue, covered map[types.Object]bool) uint64 {
	sig := signatureOf(fn)
	if sig == nil {
		return 0
	}
	var mask uint64
	if sig.Recv() != nil && covered[sig.Recv()] {
		mask |= 1
	}
	for i := range min(sig.Params().Len(), 63) {
		if covered[sig.Params().At(i)] {
			mask |= 1 << (i + 1)
		}
	}
	return mask
}

// rootOf is the variable an assignment through a field, index or pointer
// changes, as out in out.Items[i] = v.
func rootOf(expr ast.Expr, info *types.Info) types.Object {
	for {
		switch e := ast.Unparen(expr).(type) {
		case *ast.Ident:
			return info.Uses[e]
		case *ast.SelectorExpr:
			if qualified(e, info) {
				return nil
			}
			expr = e.X
		case *ast.IndexExpr:
			expr = e.X
		case *ast.StarExpr:
			expr = e.X
		default:
			return nil
		}
	}
}

// uncoverAssigned uncovers each outer variable lit assigns. A literal may run
// later, or never, so what it stores is unknown where it is written. A run the
// walk follows sets them again from what the literal leaves.
func (w *pathWalk) uncoverAssigned(lit *ast.FuncLit, s pathState) pathState {
	ast.Inspect(lit.Body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, lhs := range assign.Lhs {
			if obj := rootOf(lhs, w.frame.info); obj != nil && (obj.Pos() < lit.Pos() || obj.Pos() > lit.End()) {
				s.covered = withMember(s.covered, obj, false)
			}
		}
		return true
	})
	return s
}

// qualified reports a name another package declares, as http.StatusOK. Any
// other selector reads a field or method of its operand, whose type a stand-in
// import may leave unread.
func qualified(sel *ast.SelectorExpr, info *types.Info) bool {
	ident, ok := ast.Unparen(sel.X).(*ast.Ident)
	if !ok {
		return false
	}
	_, isPkg := info.Uses[ident].(*types.PkgName)
	return isPkg
}

// slotsOf lists fn's receiver, nil where it has none, then its parameters:
// the places a caller can hand a value in by reference.
func slotsOf(fn fnValue) []*types.Var {
	sig := signatureOf(fn)
	if sig == nil {
		return nil
	}
	slots := []*types.Var{sig.Recv()}
	for i := range sig.Params().Len() {
		slots = append(slots, sig.Params().At(i))
	}
	return slots
}

// keepSlots records which slots one way out leaves covered.
func (w *pathWalk) keepSlots(s pathState) {
	if w.verdict.kept == nil {
		w.verdict.kept = slices.Repeat([]bool{true}, len(w.slots))
	}
	for i, slot := range w.slots {
		w.verdict.kept[i] = w.verdict.kept[i] && (slot == nil || s.covered[slot])
	}
}

// slotKept is kept for a run that never returned, which keeps every slot.
func slotKept(fn fnValue, kept []bool) []bool {
	if kept == nil {
		return slices.Repeat([]bool{true}, len(slotsOf(fn)))
	}
	return kept
}

// stored keeps covered each variable call can store into only where the call
// leaves it so. A call stores through its receiver, an address, or a
// reference it is handed. A walked call does where every run keeps that slot; an
// outside one where its results are covered, as a row scan's are not.
func (w *pathWalk) stored(call *ast.CallExpr, covers bool, kept [][]bool, s pathState) pathState {
	keeps := func(slot int) bool {
		if kept == nil {
			return covers
		}
		for _, run := range kept {
			if slot < len(run) && !run[slot] {
				return false
			}
		}
		return true
	}
	if sel, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr); ok && !qualified(sel, w.frame.info) {
		s = w.storedInto(sel.X, keeps(0), s)
	}
	for i, arg := range call.Args {
		arg = ast.Unparen(arg)
		if unary, ok := arg.(*ast.UnaryExpr); ok && unary.Op == token.AND {
			s = w.storedInto(unary.X, keeps(i+1), s)
		} else if isReference(w.frame.info.TypeOf(arg)) {
			s = w.storedInto(arg, keeps(i+1), s)
		}
	}
	return s
}

// storedInto uncovers the local variable target reaches, unless the call
// keeps it covered. A receiver counts whatever its type, since a pointer
// receiver may change it.
func (w *pathWalk) storedInto(target ast.Expr, keeps bool, s pathState) pathState {
	obj := rootOf(target, w.frame.info)
	if v, ok := obj.(*types.Var); !ok || isPackageVar(v) {
		return s
	}
	return pathState{checked: s.checked, covered: withMember(s.covered, obj, s.checked || (s.covered[obj] && keeps)), nonNil: s.nonNil, answers: s.answers}
}

// isReference reports a type through which a callee can change what its
// caller holds. An address taken with & is read apart from this.
func isReference(t types.Type) bool {
	if t == nil || !validType(t) {
		return false
	}
	switch t.Underlying().(type) {
	case *types.Pointer, *types.Slice, *types.Map, *types.Chan:
		return true
	}
	return false
}

// intersectCovered keeps what is covered in both.
func intersectCovered(a, b map[types.Object]bool) map[types.Object]bool {
	var out map[types.Object]bool
	for obj := range a {
		if b[obj] {
			out = withMember(out, obj, true)
		}
	}
	return out
}
