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
// The error is followed along EVERY path of control from where it is bound to
// the end of the function (gatekit.Paths): an answer on one arm of an if does
// not answer the other arm, a return that skips the answer leaves it
// unanswered, and a later assignment over the variable loses it. A path on
// which a condition proves the error nil, or recognises its kind through
// errors.Is or errors.As, owes nothing: an error the handler named is one it
// answers on that kind's own terms, and a lane that is down is not among them.
// An error is answered
// when it reaches modelfailure.Write, or a parameter of a function of this
// package that hands that parameter there, on every path; wrapping it, or
// storing it in a value that is then handed there, carries it along.
//
// One returned through an error result is its caller's to answer, and the
// caller is held to the same rule — but only a return that leaves a declared
// function which is not itself a handler has a caller the census can see. A
// handler's own error result goes to whatever adapter wraps it, and a function
// literal's to whoever runs it, an errgroup or a retry loop, neither of which
// the census follows, so neither return answers. Anything else — dropped,
// logged, passed to httperr.Write — is not an answer either.

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// plantedModelHandlers are the handlers the flow test plants, each marking
// the model call the census must name with "// unanswered"; a handler with no
// mark must be answered. A census that asked only whether a model and
// modelfailure.Write both appear somewhere in the handler passes the first
// three.
var plantedModelHandlers = map[string]string{
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
	"a model error answered on one arm and a 500 on the other": `func h(w http.ResponseWriter, r *http.Request, c model.Client, quiet bool) {
	_, err := ai.Ask(r.Context(), c, model.Request{}, nil) // unanswered
	if err != nil {
	if quiet {
		httperr.Write(w, r, err)
	} else {
		modelfailure.Write(w, r, err)
	}
	}
}`,
	"a model error returned past on one path": `func h(w http.ResponseWriter, r *http.Request, c model.Client, quiet bool) {
	err := ask(r, c) // unanswered
	if quiet { return }
	if err != nil { modelfailure.Write(w, r, err) }
}`,
	"a handler's own error result handing it back": `func h(w http.ResponseWriter, r *http.Request, c model.Client) (err error) {
	_, err = ai.Ask(r.Context(), c, model.Request{}, nil) // unanswered
	return
}`,
	"a model error returned from a closure and answered as a 500": `func h(w http.ResponseWriter, r *http.Request, c model.Client) {
	run := func() error {
	_, err := ai.Ask(r.Context(), c, model.Request{}, nil) // unanswered
	return err
	}
	if err := run(); err != nil { httperr.Write(w, r, err) }
}`,
	"a model error returned through an errgroup and answered as a 500": `type group struct{}
func (g *group) Go(f func() error) {}
func (g *group) Wait() error { return nil }
func h(w http.ResponseWriter, r *http.Request, c model.Client) {
	var g group
	g.Go(func() error {
	_, err := ai.Ask(r.Context(), c, model.Request{}, nil) // unanswered
	return err
	})
	if err := g.Wait(); err != nil { httperr.Write(w, r, err) }
}`,
	"a helper swallowing the model error it caught": `func quietly(r *http.Request, c model.Client) string {
	if err := ask(r, c); err != nil { // unanswered
	return ""
	}
	return "drafted"
}
func h(w http.ResponseWriter, r *http.Request, c model.Client) {
	httperr.WriteJSON(w, http.StatusOK, quietly(r, c))
}`,
	"a model error overwritten before it is answered": `func h(w http.ResponseWriter, r *http.Request, c model.Client) {
	_, err := ai.Ask(r.Context(), c, model.Request{}, nil) // unanswered
	err = decode(r)
	if err != nil { modelfailure.Write(w, r, err) }
}`,
	"a helper dropping what the closure it runs returned": `func draft(r *http.Request, c model.Client) error {
	run := func() error {
	_, err := ai.Ask(r.Context(), c, model.Request{}, nil) // unanswered
	return err
	}
	_ = run()
	return decode(r)
}
func h(w http.ResponseWriter, r *http.Request, c model.Client) {
	if err := draft(r, c); err != nil { modelfailure.Write(w, r, err) }
}`,
	"a model error answered on every arm": `func h(w http.ResponseWriter, r *http.Request, c model.Client, quiet bool) {
	err := ask(r, c)
	if err == nil { return }
	if quiet { modelfailure.Write(w, r, fmt.Errorf("quiet: %w", err)); return }
	modelfailure.Write(w, r, err)
}`,
	"a helper handing its model error back on every path": `func twice(r *http.Request, c model.Client) error {
	if err := ask(r, c); err != nil { return err }
	return ask(r, c)
}
func h(w http.ResponseWriter, r *http.Request, c model.Client) {
	if err := twice(r, c); err != nil { modelfailure.Write(w, r, err) }
}`,
	"an error recognised by kind and answered on its own terms": `type unreadable struct{}
func (unreadable) Error() string { return "unreadable" }
func h(w http.ResponseWriter, r *http.Request, c model.Client) {
	err := ask(r, c)
	var u unreadable
	switch {
	case errors.As(err, &u), errors.Is(err, http.ErrHandlerTimeout):
	httperr.Write(w, r, err)
	case err != nil:
	modelfailure.Write(w, r, err)
	}
}`,
	"an error recognised as any error at all": `func h(w http.ResponseWriter, r *http.Request, c model.Client) {
	err := ask(r, c) // unanswered
	var any error
	if errors.As(err, &any) { httperr.Write(w, r, err); return }
	modelfailure.Write(w, r, err)
}`,
}

// Each planted handler is read for the model calls whose error misses
// modelfailure.Write, and must name exactly the marked ones.
func TestTheModelFailureCensusFollowsEachModelErrorToItsAnswer(t *testing.T) {
	prelude := "package planted\nimport (\n\t\"errors\"\n\t\"fmt\"\n\t\"net/http\"\n" +
		"\t\"github.com/margince/margince/backend/internal/modules/ai\"\n" +
		"\t\"github.com/margince/margince/backend/internal/compose/modelfailure\"\n" +
		"\t\"github.com/margince/margince/backend/internal/platform/httperr\"\n" +
		"\tmodel \"" + modelPortPath + "\"\n)\n" +
		"var _ = fmt.Errorf\nvar _ = errors.Is\nvar _ = modelfailure.Write\nvar _ = httperr.Write\n" +
		"func decode(r *http.Request) error { return nil }\n" +
		"func ask(r *http.Request, c model.Client) error { _, err := ai.Ask(r.Context(), c, model.Request{}, nil); return err }\n"
	for name, body := range plantedModelHandlers {
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
				var got []string
				for _, drop := range unit.unanswered {
					got = append(got, drop.at)
				}
				if !unit.reachesModel || unit.modelCalls == 0 || !slices.Equal(got, want) {
					t.Errorf("reaches a model = %v through %d call(s), unanswered at %q; want the calls at %q",
						unit.reachesModel, unit.modelCalls, got, want)
				}
				return
			}
			t.Fatal("the census did not see the planted handler")
		})
	}
}

// A dropped model error belongs to the function that drops it, which is what
// a waiver names: the handler when it drops the error inline, the helper when a
// helper catches it and answers without the model.
func TestAModelErrorDropBelongsToTheFunctionDroppingIt(t *testing.T) {
	src := "package planted\nimport (\n\t\"net/http\"\n" +
		"\t\"github.com/margince/margince/backend/internal/modules/ai\"\n" +
		"\tmodel \"" + modelPortPath + "\"\n)\n" +
		`func quietly(r *http.Request, c model.Client) string {
	if _, err := ai.Ask(r.Context(), c, model.Request{}, nil); err != nil { return "" }
	return "drafted"
}
func viaHelper(w http.ResponseWriter, r *http.Request, c model.Client) { _ = quietly(r, c) }
func inline(w http.ResponseWriter, r *http.Request, c model.Client) {
	_, _ = ai.Ask(r.Context(), c, model.Request{}, nil)
}`
	want := map[string]string{"planted.viaHelper": "planted.quietly", "planted.inline": "planted.inline"}
	for _, unit := range modelHandlerUnits(typeCheckPlanted(t, src)) {
		owner, ok := want[unit.name]
		if !ok {
			continue
		}
		delete(want, unit.name)
		if len(unit.unanswered) != 1 || unit.unanswered[0].owner != owner {
			t.Errorf("%s drops %+v; want one drop owned by %s", unit.name, unit.unanswered, owner)
		}
	}
	if len(want) > 0 {
		t.Errorf("the census did not see the planted handlers %v", want)
	}
}

// modelErrorFlow is one function body read for where its values go.
type modelErrorFlow struct {
	g       *modelCallGraph
	sig     *types.Signature
	body    *ast.BlockStmt
	parents map[ast.Node]ast.Node
	uses    map[types.Object][]*ast.Ident
	// handsBack is whether a return from this body has a caller the census
	// holds to the rule: the body is a declared function, not a handler.
	handsBack bool
}

// flowOf reads body, whose own signature is sig, once.
func (g *modelCallGraph) flowOf(body *ast.BlockStmt, sig *types.Signature) *modelErrorFlow {
	if flow, ok := g.flows[body]; ok {
		return flow
	}
	flow := &modelErrorFlow{
		g: g, sig: sig, body: body, parents: map[ast.Node]ast.Node{},
		uses: map[types.Object][]*ast.Ident{}, handsBack: g.declared[body] && !isHandlerSignature(sig),
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

// record notes a use of a variable.
func (f *modelErrorFlow) record(n ast.Node) {
	if ident, ok := n.(*ast.Ident); ok {
		if obj, ok := f.g.checked.info.Uses[ident].(*types.Var); ok {
			f.uses[obj] = append(f.uses[obj], ident)
		}
	}
}

// modelDrop is one model call whose error is not answered, and the function
// that drops it: the body being read when owner is nil, or a helper beneath it.
type modelDrop struct {
	at    token.Pos
	owner *types.Func
}

// unansweredModelCalls counts the model-reaching calls in body and lists each
// whose error is not answered, and each model call inside this package's
// functions they reach whose error goes nowhere. A handler literal nested in
// body is a handler of its own, read on its own.
func (g *modelCallGraph) unansweredModelCalls(body *ast.BlockStmt, sig *types.Signature) (int, []modelDrop) {
	flow := g.flowOf(body, sig)
	calls := 0
	var unanswered []modelDrop
	ast.Inspect(body, func(n ast.Node) bool {
		if lit, ok := n.(*ast.FuncLit); ok {
			litSig, ok := g.checked.info.TypeOf(lit).(*types.Signature)
			return !ok || !isHandlerSignature(litSig)
		}
		call, ok := n.(*ast.CallExpr)
		if !ok || !g.isModelCall(call) {
			return true
		}
		calls++
		if !flow.answered(call) {
			unanswered = append(unanswered, modelDrop{at: call.Pos()})
		}
		for _, helper := range g.helpersOf(call) {
			unanswered = append(unanswered, g.swallowedIn(helper)...)
		}
		return true
	})
	slices.SortFunc(unanswered, func(a, b modelDrop) int { return int(a.at - b.at) })
	return calls, slices.CompactFunc(unanswered, func(a, b modelDrop) bool { return a.at == b.at })
}

// helpersOf lists this package's functions a model call runs: its callee —
// every implementation here when that is an interface method — and each
// function it is handed by name.
func (g *modelCallGraph) helpersOf(call *ast.CallExpr) []*types.Func {
	var helpers []*types.Func
	if callee := g.calleeOf(call); callee != nil {
		helpers = append(append(helpers, callee), implementationsOf(callee, g.checked.pkg)...)
	}
	for _, arg := range call.Args {
		var ident *ast.Ident
		switch arg := ast.Unparen(arg).(type) {
		case *ast.Ident:
			ident = arg
		case *ast.SelectorExpr:
			ident = arg.Sel
		}
		if f, ok := g.checked.info.Uses[ident].(*types.Func); ident != nil && ok {
			helpers = append(helpers, f.Origin())
		}
	}
	return helpers
}

// swallowedIn lists the model calls in helper, and beneath it, whose error
// neither reaches modelfailure.Write nor is handed back to helper's caller —
// the error a helper catches and drops, owned by the helper that drops it just
// as an error dropped inline is owned by its handler. A helper that is itself
// a handler is read as one, and one outside this package is trusted to hand
// its model error back.
func (g *modelCallGraph) swallowedIn(helper *types.Func) []modelDrop {
	if got, ok := g.swallowed[helper]; ok {
		return got
	}
	// Entered before the walk, so a recursive helper reads "nothing" rather than looping.
	g.swallowed[helper] = nil
	body, ok := g.bodies[helper]
	if !ok || isHandlerSignature(helper.Signature()) {
		return nil
	}
	_, got := g.unansweredModelCalls(body, helper.Signature())
	for i := range got {
		if got[i].owner == nil {
			got[i].owner = helper
		}
	}
	g.swallowed[helper] = got
	return got
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
		return f.assignsOnward(p.Lhs, p.Rhs, child, slots, p)
	case *ast.ValueSpec:
		return f.assignsOnward(identExprs(p.Names), p.Values, child, slots, f.declaring(p))
	}
	return false
}

// declaring is the declaration statement holding spec.
func (f *modelErrorFlow) declaring(spec *ast.ValueSpec) ast.Stmt {
	decl, _ := f.parents[f.parents[spec]].(*ast.DeclStmt)
	return decl
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
func (f *modelErrorFlow) assignsOnward(lhs, rhs []ast.Expr, value ast.Node, slots []int, from ast.Stmt) bool {
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

// flowKey is one variable's value from one statement on.
type flowKey struct {
	obj  types.Object
	from ast.Stmt
}

// reaches reports whether the value obj holds from the statement from — nil
// for a parameter, from the top of the body — reaches an answer on every path
// of control. A return answers it only when returns is set — a caller then
// answers it — which a sink parameter does not accept, since there the caller
// has handed the error over.
func (f *modelErrorFlow) reaches(obj types.Object, from ast.Stmt, returns bool, seen map[flowKey]bool) bool {
	key := flowKey{obj, from}
	if seen[key] {
		return false
	}
	seen[key] = true
	paths := gatekit.Paths{
		Body: f.body,
		Step: func(stmt ast.Stmt) gatekit.Step { return f.step(stmt, obj, returns, seen) },
		Owed: func(cond ast.Expr, outcome bool) bool {
			return !gatekit.ErrorSettled(cond, outcome, f.g.checked.info, func(e ast.Expr) bool {
				return f.rootObject(e) == obj && carriesError(f.g.checked.info.TypeOf(e))
			})
		},
	}
	return !paths.Unsettled(from)
}

// step is what one statement does to obj's value: it answers it through a use,
// hands it back by a bare return, or loses it to an assignment over it.
func (f *modelErrorFlow) step(stmt ast.Stmt, obj types.Object, returns bool, seen map[flowKey]bool) gatekit.Step {
	for _, use := range f.uses[obj] {
		if stmt.Pos() <= use.Pos() && use.End() <= stmt.End() && f.useAnswers(use, returns, seen) {
			return gatekit.Settles
		}
	}
	if ret, ok := stmt.(*ast.ReturnStmt); ok && len(ret.Results) == 0 && returns &&
		f.isNamedErrorResult(obj) && f.handsBackFrom(ret) {
		return gatekit.Settles
	}
	if assign, ok := stmt.(*ast.AssignStmt); ok {
		for _, lhs := range assign.Lhs {
			if ident, ok := ast.Unparen(lhs).(*ast.Ident); ok && f.rootObject(ident) == obj {
				return gatekit.Drops
			}
		}
	}
	return gatekit.Passes
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
			return f.propagates(p.Lhs, p.Rhs, child, p, returns, seen)
		case *ast.ValueSpec:
			return f.propagates(identExprs(p.Names), p.Values, child, f.declaring(p), returns, seen)
		case ast.Stmt, nil:
			return false
		}
		child = parent
	}
}

// propagates follows a value read on the right of an assignment into the
// variable, or every variable, it is assigned to. A target on the left is
// written, not read, and carries nothing.
func (f *modelErrorFlow) propagates(lhs, rhs []ast.Expr, value ast.Node, from ast.Stmt, returns bool, seen map[flowKey]bool) bool {
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

// returnsError reports whether value, one result of ret, lands in an
// error-carrying result of a function whose caller answers it.
func (f *modelErrorFlow) returnsError(ret *ast.ReturnStmt, value ast.Node, slots []int) bool {
	if !f.handsBackFrom(ret) {
		return false
	}
	results := f.sig.Results()
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

// handsBackFrom reports whether ret returns to a caller the census holds to
// the rule: it leaves this body's own declared function, not a literal in it.
func (f *modelErrorFlow) handsBackFrom(ret *ast.ReturnStmt) bool {
	if !f.handsBack {
		return false
	}
	for parent := f.parents[ret]; parent != nil; parent = f.parents[parent] {
		if _, ok := parent.(*ast.FuncLit); ok {
			return false
		}
	}
	return true
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
	answered := flow.reaches(params.At(index), nil, false, map[flowKey]bool{})
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
