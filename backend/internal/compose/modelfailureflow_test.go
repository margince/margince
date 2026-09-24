// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A handler answers a model failure through modelfailure.Write only when the
// error its model-reaching call returned is the error it hands there. Reaching
// a model somewhere and calling modelfailure.Write somewhere else proves
// nothing: the model's error can still leave through httperr.Write as an opaque
// 500 while an unrelated error takes the modelfailure path. So inside each
// handler the census follows every model-reaching call's error from where the
// call binds it to where it goes.
//
// The following is by position and block, not by full control flow: a use of
// a variable reads a value assigned before it unless a later assignment, made
// in a block that also holds the use, has replaced it. An assignment in one
// branch of an if does not replace the value for a use after the if. An error is answered
// when it reaches modelfailure.Write, or a parameter of a function of this
// package that hands that parameter there; wrapping it, or storing it in a
// value that is then handed there, carries it along. One returned through an
// error result is its caller's to answer, and the caller is held to the same
// rule. Anything else — dropped, logged, passed to httperr.Write — is not.

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"strings"
	"testing"
)

// Each planted handler marks the model call the census must name with
// "// unanswered"; a handler with no mark must be answered. A census that
// asked only whether a model and modelfailure.Write both appear somewhere in
// the handler passes the first three.
func TestTheModelFailureCensusFollowsEachModelErrorToItsAnswer(t *testing.T) {
	prelude := "package planted\nimport (\n\t\"fmt\"\n\t\"net/http\"\n" +
		"\t\"github.com/margince/margince/backend/internal/modules/ai\"\n" +
		"\t\"github.com/margince/margince/backend/internal/compose/modelfailure\"\n" +
		"\t\"github.com/margince/margince/backend/internal/platform/httperr\"\n" +
		"\tmodel \"" + modelPortPath + "\"\n)\n" +
		"var _ = fmt.Errorf\nvar _ = modelfailure.Write\nvar _ = httperr.Write\n" +
		"func decode(r *http.Request) error { return nil }\n" +
		"func ask(r *http.Request, c model.Client) error { _, err := ai.Ask(r.Context(), c, model.Request{}, nil); return err }\n"
	for name, body := range map[string]string{
		"a model error to httperr beside an unrelated modelfailure": `func h(w http.ResponseWriter, r *http.Request, c model.Client) {
	if _, err := ai.Ask(r.Context(), c, model.Request{}, nil); err != nil { // unanswered
		httperr.Write(w, r, err)
		return
	}
	if err := decode(r); err != nil { modelfailure.Write(w, r, err) }
}`,
		"one variable reused for an unrelated error": `func h(w http.ResponseWriter, r *http.Request, c model.Client) {
	_, err := ai.Ask(r.Context(), c, model.Request{}, nil) // unanswered
	if err != nil { httperr.Write(w, r, err); return }
	err = decode(r)
	if err != nil { modelfailure.Write(w, r, err) }
}`,
		"a model error dropped": `func h(w http.ResponseWriter, r *http.Request, c model.Client) {
	_, _ = ai.Ask(r.Context(), c, model.Request{}, nil) // unanswered
	modelfailure.Write(w, r, decode(r))
}`,
		"a helper handed the error that answers it as a 500": `func h(w http.ResponseWriter, r *http.Request, c model.Client) {
	if err := ask(r, c); err != nil { fail(w, r, err) } // unanswered
}
func fail(w http.ResponseWriter, r *http.Request, err error) { httperr.Write(w, r, err) }`,
		"an error carried out in a struct and answered as a 500": `type result struct{ err error }
func carry(r *http.Request, c model.Client) result { return result{err: ask(r, c)} }
func h(w http.ResponseWriter, r *http.Request, c model.Client) {
	res := carry(r, c) // unanswered
	httperr.Write(w, r, res.err)
}`,
		"a wrapped error answered": `func h(w http.ResponseWriter, r *http.Request, c model.Client) {
	err := ask(r, c)
	if err != nil { err = fmt.Errorf("drafting: %w", err); modelfailure.Write(w, r, err) }
}`,
		"a helper handed the error that answers it": `func h(w http.ResponseWriter, r *http.Request, c model.Client) {
	if err := ask(r, c); err != nil { fail(w, r, err) }
}
func fail(w http.ResponseWriter, r *http.Request, err error) { modelfailure.Write(w, r, err) }`,
		"a struct carrying the error answered": `type result struct{ err error }
func carry(r *http.Request, c model.Client) result { return result{err: ask(r, c)} }
func h(w http.ResponseWriter, r *http.Request, c model.Client) {
	res := carry(r, c)
	modelfailure.Write(w, r, res.err)
}`,
		"an error assigned in either branch and answered after both": `func h(w http.ResponseWriter, r *http.Request, c model.Client, cached bool) {
	var err error
	if cached {
		err = decode(r)
	} else {
		err = ask(r, c)
	}
	if err != nil { modelfailure.Write(w, r, err) }
}`,
		"a named result handed back by a bare return": `func h(w http.ResponseWriter, r *http.Request, c model.Client) (err error) {
	_, err = ai.Ask(r.Context(), c, model.Request{}, nil)
	return
}`,
	} {
		t.Run(name, func(t *testing.T) {
			src := prelude + body
			var want []string
			for i, line := range strings.Split(src, "\n") {
				if strings.Contains(line, "// unanswered") {
					want = append(want, fmt.Sprintf("planted.go:%d", i+1))
				}
			}
			for _, unit := range modelHandlerUnits(typeCheckPlanted(t, src)) {
				if unit.name != "planted.h" {
					continue
				}
				if !unit.reachesModel || unit.modelCalls == 0 || !slices.Equal(unit.unanswered, want) {
					t.Errorf("reaches a model = %v through %d call(s), unanswered at %q; want the calls at %q",
						unit.reachesModel, unit.modelCalls, unit.unanswered, want)
				}
				return
			}
			t.Fatal("the census did not see the planted handler")
		})
	}
}

// modelErrorFlow is one function body read for where its values go.
type modelErrorFlow struct {
	g       *modelCallGraph
	sig     *types.Signature
	parents map[ast.Node]ast.Node
	uses    map[types.Object][]*ast.Ident
	// assigned is, per variable, every statement assigning it.
	assigned map[types.Object][]*ast.AssignStmt
	bare     []*ast.ReturnStmt
}

// flowOf reads body, whose own signature is sig, once.
func (g *modelCallGraph) flowOf(body *ast.BlockStmt, sig *types.Signature) *modelErrorFlow {
	if flow, ok := g.flows[body]; ok {
		return flow
	}
	flow := &modelErrorFlow{
		g: g, sig: sig, parents: map[ast.Node]ast.Node{},
		uses: map[types.Object][]*ast.Ident{}, assigned: map[types.Object][]*ast.AssignStmt{},
	}
	var stack []ast.Node
	ast.Inspect(body, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		if len(stack) > 0 {
			flow.parents[n] = stack[len(stack)-1]
		}
		stack = append(stack, n)
		flow.record(n)
		return true
	})
	g.flows[body] = flow
	return flow
}

// record notes what one node tells the flow: a use, an assignment, a bare return.
func (f *modelErrorFlow) record(n ast.Node) {
	info := f.g.checked.info
	switch n := n.(type) {
	case *ast.Ident:
		if obj, ok := info.Uses[n].(*types.Var); ok {
			f.uses[obj] = append(f.uses[obj], n)
		}
	case *ast.AssignStmt:
		for _, lhs := range n.Lhs {
			if obj := f.rootObject(lhs); obj != nil {
				f.assigned[obj] = append(f.assigned[obj], n)
			}
		}
	case *ast.ReturnStmt:
		if len(n.Results) == 0 {
			f.bare = append(f.bare, n)
		}
	}
}

// unansweredModelCalls counts the model-reaching calls in body and lists the
// position of each whose error is not answered.
func (g *modelCallGraph) unansweredModelCalls(body *ast.BlockStmt, sig *types.Signature) (int, []token.Pos) {
	flow := g.flowOf(body, sig)
	calls := 0
	var unanswered []token.Pos
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || !g.isModelCall(call) {
			return true
		}
		calls++
		if !flow.answered(call) {
			unanswered = append(unanswered, call.Pos())
		}
		return true
	})
	return calls, unanswered
}

// isModelCall reports whether call can reach a model: its callee can, or a
// function it is handed — named or literal — can.
func (g *modelCallGraph) isModelCall(call *ast.CallExpr) bool {
	if callee := g.calleeOf(call); callee != nil {
		if isModelFailureWrite(callee) {
			return false
		}
		if g.visit(callee).model {
			return true
		}
	}
	for _, arg := range call.Args {
		for _, ref := range g.refs(ast.Unparen(arg)) {
			if g.visit(ref).model {
				return true
			}
		}
	}
	return false
}

// calleeOf is the declared function call invokes, or nil for a call through a
// function value, a conversion or a builtin.
func (g *modelCallGraph) calleeOf(call *ast.CallExpr) *types.Func {
	fun := ast.Unparen(call.Fun)
	if index, ok := fun.(*ast.IndexExpr); ok {
		fun = index.X
	}
	var ident *ast.Ident
	switch fun := fun.(type) {
	case *ast.Ident:
		ident = fun
	case *ast.SelectorExpr:
		ident = fun.Sel
	default:
		return nil
	}
	if f, ok := g.checked.info.Uses[ident].(*types.Func); ok {
		return f.Origin()
	}
	return nil
}

// answered reports whether every error call returns reaches an answer. A call
// that returns no error has none to lose.
func (f *modelErrorFlow) answered(call *ast.CallExpr) bool {
	slots := errorSlots(f.g.checked.info.TypeOf(call))
	return len(slots) == 0 || f.carried(call, slots)
}

// carried reports whether the error slots of expr's value each reach an answer.
func (f *modelErrorFlow) carried(expr ast.Expr, slots []int) bool {
	child, parent := f.climb(expr)
	switch p := parent.(type) {
	case *ast.CallExpr:
		// Handed on: answered by a sink parameter, or carried by the call's
		// own error, which is then followed the same way.
		index := nodeIndex(p.Args, child)
		if index >= 0 && f.g.isSinkArg(p, index) {
			return true
		}
		onward := errorSlots(f.g.checked.info.TypeOf(p))
		return index >= 0 && len(onward) > 0 && f.carried(p, onward)
	case *ast.ReturnStmt:
		return f.returnsError(p, child, slots)
	case *ast.AssignStmt:
		return f.assignsOnward(p.Lhs, p.Rhs, child, slots, p.End())
	case *ast.ValueSpec:
		return f.assignsOnward(identExprs(p.Names), p.Values, child, slots, p.End())
	}
	return false
}

// climb walks up from expr through the expressions that only hold its value —
// parentheses, a composite literal, an address-of — to the node that does
// something with it, returning that node and the child it was reached through.
func (f *modelErrorFlow) climb(expr ast.Expr) (ast.Node, ast.Node) {
	var child ast.Node = expr
	for {
		parent := f.parents[child]
		switch parent.(type) {
		case *ast.ParenExpr, *ast.CompositeLit, *ast.KeyValueExpr, *ast.UnaryExpr, *ast.StarExpr:
			child = parent
		default:
			return child, parent
		}
	}
}

// assignsOnward follows each error slot of value, assigned to lhs, from the end
// of the statement to wherever the variable it lands in goes.
func (f *modelErrorFlow) assignsOnward(lhs, rhs []ast.Expr, value ast.Node, slots []int, from token.Pos) bool {
	targets := make([]ast.Expr, 0, len(slots))
	if len(rhs) == 1 && len(lhs) > 1 {
		for _, slot := range slots {
			targets = append(targets, lhs[slot])
		}
	} else if index := nodeIndex(rhs, value); index >= 0 && index < len(lhs) {
		targets = append(targets, lhs[index])
	}
	if len(targets) == 0 {
		return false
	}
	for _, target := range targets {
		obj := f.rootObject(target)
		if obj == nil || !f.reaches(obj, from, true, map[flowKey]bool{}) {
			return false
		}
	}
	return true
}

// flowKey is one variable's value from one assignment on.
type flowKey struct {
	obj  types.Object
	from token.Pos
}

// reaches reports whether the value obj holds from the assignment ending at
// from reaches an answer through a use no later assignment replaced it for. A
// return answers it
// only when returns is set — a caller then answers it — which a sink parameter
// does not accept, since there the caller has handed the error over.
func (f *modelErrorFlow) reaches(obj types.Object, from token.Pos, returns bool, seen map[flowKey]bool) bool {
	key := flowKey{obj, from}
	if seen[key] {
		return false
	}
	seen[key] = true
	for _, use := range f.uses[obj] {
		if use.Pos() > from && !f.replaced(obj, from, use) && f.useAnswers(use, returns, seen) {
			return true
		}
	}
	if returns && f.isNamedErrorResult(obj) {
		for _, ret := range f.bare {
			if ret.Pos() > from && !f.replaced(obj, from, ret) {
				return true
			}
		}
	}
	return false
}

// useAnswers reports whether one read of a variable hands its value to an
// answer, climbing from the read through every call it is an argument of.
func (f *modelErrorFlow) useAnswers(use *ast.Ident, returns bool, seen map[flowKey]bool) bool {
	var child ast.Node = use
	for {
		parent := f.parents[child]
		switch p := parent.(type) {
		case *ast.CallExpr:
			if index := nodeIndex(p.Args, child); index >= 0 && f.g.isSinkArg(p, index) {
				return true
			}
		case *ast.ReturnStmt:
			return returns && f.returnsError(p, child, []int{0})
		case *ast.AssignStmt:
			return f.propagates(p.Lhs, p.Rhs, child, p.End(), returns, seen)
		case *ast.ValueSpec:
			return f.propagates(identExprs(p.Names), p.Values, child, p.End(), returns, seen)
		case ast.Stmt, nil:
			return false
		}
		child = parent
	}
}

// propagates follows a value read on the right of an assignment into the
// variable, or every variable, it is assigned to. A target on the left is
// written, not read, and carries nothing.
func (f *modelErrorFlow) propagates(lhs, rhs []ast.Expr, value ast.Node, from token.Pos, returns bool, seen map[flowKey]bool) bool {
	index := nodeIndex(rhs, value)
	if index < 0 {
		return false
	}
	targets := lhs
	if len(lhs) == len(rhs) {
		targets = lhs[index : index+1]
	}
	for _, target := range targets {
		if obj := f.rootObject(target); obj != nil && f.reaches(obj, from, returns, seen) {
			return true
		}
	}
	return false
}

// replaced reports whether the value obj holds from from has been replaced by
// the time use reads it: an assignment between the two, in a block that also
// holds use, so every path to use ran it.
func (f *modelErrorFlow) replaced(obj types.Object, from token.Pos, use ast.Node) bool {
	for _, stmt := range f.assigned[obj] {
		if stmt.End() <= from || stmt.End() > use.Pos() {
			continue
		}
		if block := f.enclosingBlock(stmt); block.Pos() <= use.Pos() && use.End() <= block.End() {
			return true
		}
	}
	return false
}

// enclosingBlock is the innermost block, case or select clause holding n.
func (f *modelErrorFlow) enclosingBlock(n ast.Node) ast.Node {
	for parent := f.parents[n]; parent != nil; parent = f.parents[parent] {
		switch parent.(type) {
		case *ast.BlockStmt, *ast.CaseClause, *ast.CommClause:
			return parent
		}
	}
	return n
}

// returnsError reports whether value, one result of ret, lands in an
// error-carrying result of the function ret returns from.
func (f *modelErrorFlow) returnsError(ret *ast.ReturnStmt, value ast.Node, slots []int) bool {
	results := f.enclosingSignature(ret).Results()
	indexes := slots
	if index := nodeIndex(ret.Results, value); index >= 0 && len(ret.Results) == results.Len() {
		indexes = []int{index}
	}
	for _, index := range indexes {
		if index < results.Len() && carriesError(results.At(index).Type()) {
			return true
		}
	}
	return false
}

// enclosingSignature is the signature of the innermost function holding n.
func (f *modelErrorFlow) enclosingSignature(n ast.Node) *types.Signature {
	for parent := f.parents[n]; parent != nil; parent = f.parents[parent] {
		if lit, ok := parent.(*ast.FuncLit); ok {
			if sig, ok := f.g.checked.info.TypeOf(lit).(*types.Signature); ok {
				return sig
			}
		}
	}
	return f.sig
}

// isNamedErrorResult reports whether obj is a named error result of the body's
// own function, which a bare return hands back.
func (f *modelErrorFlow) isNamedErrorResult(obj types.Object) bool {
	for result := range f.sig.Results().Variables() {
		if result == obj && carriesError(result.Type()) {
			return true
		}
	}
	return false
}

// rootObject is the variable an assignment target writes into: x for x, x.f,
// x[i] and *x; nil for the blank identifier.
func (f *modelErrorFlow) rootObject(expr ast.Expr) types.Object {
	for {
		switch e := ast.Unparen(expr).(type) {
		case *ast.Ident:
			if obj, ok := f.g.checked.info.Defs[e].(*types.Var); ok {
				return obj
			}
			if obj, ok := f.g.checked.info.Uses[e].(*types.Var); ok {
				return obj
			}
			return nil
		case *ast.SelectorExpr:
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

// sinkKey is one parameter of one function.
type sinkKey struct {
	f     *types.Func
	index int
}

// isSinkArg reports whether the argument at index of call reaches
// modelfailure.Write: it is that function's error, or a parameter of a
// function of this package that hands it there.
func (g *modelCallGraph) isSinkArg(call *ast.CallExpr, index int) bool {
	callee := g.calleeOf(call)
	if callee == nil {
		return false
	}
	params := callee.Signature().Params()
	if index >= params.Len() {
		return false
	}
	if isModelFailureWrite(callee) {
		return carriesError(params.At(index).Type())
	}
	key := sinkKey{callee, index}
	if answered, ok := g.sinks[key]; ok {
		return answered
	}
	// Entered before the walk, so a recursive helper reads "no" rather than looping.
	g.sinks[key] = false
	body, ok := g.bodies[callee]
	if !ok {
		return false
	}
	flow := g.flowOf(body, callee.Signature())
	answered := flow.reaches(params.At(index), body.Lbrace, false, map[flowKey]bool{})
	g.sinks[key] = answered
	return answered
}

// errorSlots lists the positions of a call's results that carry an error.
func errorSlots(t types.Type) []int {
	var slots []int
	if tuple, ok := t.(*types.Tuple); ok {
		for i := range tuple.Len() {
			if carriesError(tuple.At(i).Type()) {
				slots = append(slots, i)
			}
		}
		return slots
	}
	if t != nil && carriesError(t) {
		slots = append(slots, 0)
	}
	return slots
}

var errorInterface = types.Universe.Lookup("error").Type().Underlying().(*types.Interface)

// carriesError reports whether a value of t is an error, or a struct holding
// one in a field, which is how a result can carry a model failure out without
// an error result.
func carriesError(t types.Type) bool {
	if types.Implements(t, errorInterface) {
		return true
	}
	if ptr, ok := types.Unalias(t).(*types.Pointer); ok {
		t = ptr.Elem()
	}
	st, ok := types.Unalias(t).Underlying().(*types.Struct)
	if !ok {
		return false
	}
	for field := range st.Fields() {
		if types.Implements(field.Type(), errorInterface) {
			return true
		}
	}
	return false
}

func identExprs(names []*ast.Ident) []ast.Expr {
	exprs := make([]ast.Expr, len(names))
	for i, name := range names {
		exprs[i] = name
	}
	return exprs
}

// nodeIndex is the position of n among exprs, or -1.
func nodeIndex(exprs []ast.Expr, n ast.Node) int {
	for i, expr := range exprs {
		if ast.Node(expr) == n {
			return i
		}
	}
	return -1
}
