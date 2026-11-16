// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

package gates

// The path walk of the individual-performance gate. A figure counts as
// checked only when every path to it passed a read of the switch, or the
// value written is covered (individualperformancevalues_test.go).
//
// Branches join so that a check on one does not cover its sibling. A call
// checks when all it may run checks on every success return. For a func-typed
// parameter, that is the value this call site passes.

import (
	"go/ast"
	"go/token"
	"go/types"
	"maps"
	"path"
	"slices"
	"strings"
)

// maxWalkDepth bounds how deep the walk follows calls whose bindings make a
// result it cannot reuse. A deeper call is read as one that does not check.
const maxWalkDepth = 40

var errorType = types.Universe.Lookup("error").Type()

// fnValue is one function a call may run: a declaration, or a literal. A
// literal keeps the frame it was written in, for its free parameters.
type fnValue struct {
	decl *types.Func
	lit  *ast.FuncLit
	env  *pathFrame
	info *types.Info
}

// pathFrame is one walked call and what its call site binds each func-typed
// parameter to, and each string parameter it passes a constant. A literal's
// frame links to the frame it was written in.
type pathFrame struct {
	info   *types.Info
	bound  map[*types.Var][]fnValue
	names  map[*types.Var]string
	parent *pathFrame
	depth  int
}

// figurePoints counts the places a walk hands out a figure, and names those no
// check came before.
type figurePoints struct {
	count     int
	unchecked []token.Pos
}

// add counts one figure. A branch is walked for each way into it, so it can
// reach one place again; unchecked holds no place twice.
func (p *figurePoints) add(at token.Pos, checked bool) {
	p.count++
	if !checked && !slices.Contains(p.unchecked, at) {
		p.unchecked = append(p.unchecked, at)
	}
}

func (p *figurePoints) merge(other figurePoints) {
	p.count += other.count
	for _, at := range other.unchecked {
		p.add(at, false)
		p.count--
	}
}

// walkVerdict is what walking one function from one entry state found.
// guards says every success return passed a check, and bare says each that did
// not hands out only covered values. writes are the response writes that carry
// a figure; returns are the returns that carry a value. answers marks each
// result that carries the switch's answer on every success return, and
// captured holds the outer variables a literal leaves holding one. outer is
// whether a literal leaves each outer variable covered.
type walkVerdict struct {
	guards   bool
	bare     bool
	writes   figurePoints
	returns  figurePoints
	answers  []bool
	captured map[types.Object]bool
	outer    map[types.Object]bool
	// kept says, for the receiver and each parameter, that every way out
	// leaves it covered, so what a caller handed in by reference still is.
	kept  []bool
	exits int
}

type walkKey struct {
	decl    *types.Func
	lit     *ast.FuncLit
	checked bool
	covered uint64
}

// switchWalk walks the graph, reusing the verdict of a call whose parameters
// are bound to nothing.
type switchWalk struct {
	g         *switchGraph
	memo      map[walkKey]walkVerdict
	active    map[walkKey]bool
	resolving map[*types.Var]bool
}

func newSwitchWalk(g *switchGraph) *switchWalk {
	return &switchWalk{g: g, memo: map[walkKey]walkVerdict{}, active: map[walkKey]bool{}, resolving: map[*types.Var]bool{}}
}

// walk reads fn from an entry state. A call already on the stack answers as
// one that does not check, so recursion cannot lend itself a check.
func (sw *switchWalk) walk(next run, entry pathState, depth int) walkVerdict {
	fn := next.fn
	key := walkKey{fn.decl, fn.lit, entry.checked, coveredMask(fn, entry.covered)}
	reusable := len(next.bound) == 0 && len(next.names) == 0 && fn.env == nil && len(entry.answers) == 0
	if verdict, ok := sw.memo[key]; ok && reusable {
		return verdict
	}
	if sw.active[key] || depth > maxWalkDepth {
		return walkVerdict{}
	}
	sw.active[key] = true
	defer delete(sw.active, key)

	ft, body, info := sw.parts(fn)
	if body == nil {
		return walkVerdict{}
	}
	w := &pathWalk{
		sw: sw, frame: &pathFrame{info: info, bound: next.bound, names: next.names, parent: fn.env, depth: depth},
		shape: shapeOf(ft, info), verdict: walkVerdict{guards: true, bare: true}, lit: fn.lit,
		answerCalls: map[*ast.CallExpr]callAnswer{}, discarded: map[*ast.CallExpr]bool{}, hits: map[types.Object]bool{},
		floorCalls: map[*ast.CallExpr]int{}, floors: map[types.Object]bool{}, covers: map[*ast.CallExpr]bool{},
	}
	start := pathState{checked: entry.checked, covered: maps.Clone(entry.covered), answers: entry.answers}
	for _, named := range w.shape.named {
		start.covered = withMember(start.covered, named, named != nil)
	}
	if fn.lit != nil {
		w.outerAtEntry = entry.covered
	}
	w.slots = slotsOf(fn)
	end, falls := w.block(body.List, start)
	if falls {
		w.exit(nil, end, body.Rbrace)
	}
	w.finishAnswers()
	if reusable {
		sw.memo[key] = w.verdict
	}
	return w.verdict
}

func (sw *switchWalk) parts(fn fnValue) (*ast.FuncType, *ast.BlockStmt, *types.Info) {
	if fn.lit != nil {
		return fn.lit.Type, fn.lit.Body, fn.info
	}
	decl := sw.g.decls[fn.decl]
	if decl == nil {
		return nil, nil, nil
	}
	return decl.Type, decl.Body, sw.g.infos[fn.decl]
}

// resultShape is how many results a function declares, whether the last is
// an error, and the named results a bare return hands back.
type resultShape struct {
	results int
	errLast bool
	named   []types.Object
}

func shapeOf(ft *ast.FuncType, info *types.Info) resultShape {
	var shape resultShape
	if ft.Results == nil {
		return shape
	}
	for _, field := range ft.Results.List {
		shape.results += max(1, len(field.Names))
		for _, name := range field.Names {
			shape.named = append(shape.named, info.Defs[name])
		}
	}
	last := ft.Results.List[len(ft.Results.List)-1]
	shape.errLast = types.Identical(info.TypeOf(last.Type), errorType)
	return shape
}

// pathState is what holds on one path. checked says a check was passed.
// covered holds the variables whose value needs nothing more from the switch.
// nonNil holds the error variables known not to be nil. answers holds the
// variables that hold the switch's answer: it checks only on a branch a
// condition on it decides.
type pathState struct {
	checked bool
	covered map[types.Object]bool
	nonNil  map[types.Object]bool
	answers map[types.Object]bool
}

func (s pathState) withNonNil(obj types.Object) pathState {
	s.nonNil = withMember(s.nonNil, obj, true)
	return s
}

// withMember is set with obj in it or not, leaving set itself unchanged.
func withMember(set map[types.Object]bool, obj types.Object, in bool) map[types.Object]bool {
	if set[obj] == in {
		return set
	}
	next := maps.Clone(set)
	if next == nil {
		next = map[types.Object]bool{}
	}
	if in {
		next[obj] = true
	} else {
		delete(next, obj)
	}
	return next
}

func (s pathState) same(other pathState) bool {
	return s.checked == other.checked && maps.Equal(s.covered, other.covered) && maps.Equal(s.nonNil, other.nonNil) && maps.Equal(s.answers, other.answers)
}

// breakTarget is a statement a break leaves, and for a loop one a continue
// goes back to. Each keeps the states that left it that way.
type breakTarget struct {
	label     string
	loop      bool
	breakers  []pathState
	continues []pathState
}

// pathWalk walks the body of one function. answerCalls holds what each call
// it walked answers, for the condition or assignment that reads it.
type pathWalk struct {
	sw          *switchWalk
	frame       *pathFrame
	shape       resultShape
	verdict     walkVerdict
	targets     []*breakTarget
	lit         *ast.FuncLit
	answerCalls map[*ast.CallExpr]callAnswer
	discarded   map[*ast.CallExpr]bool
	// hits holds the ok variables of comma-ok reads of held caches.
	hits map[types.Object]bool
	// floorCalls holds the result each floor call answers in; floors holds
	// the variables that hold one.
	floorCalls map[*ast.CallExpr]int
	floors     map[types.Object]bool
	// covers holds whether each call walked answers covered results, and
	// outerAtEntry what a literal's frame held covered when it was run.
	covers       map[*ast.CallExpr]bool
	outerAtEntry map[types.Object]bool
	slots        []*types.Var
}

func (w *pathWalk) block(stmts []ast.Stmt, s pathState) (pathState, bool) {
	for _, stmt := range stmts {
		var falls bool
		if s, falls = w.stmt(stmt, s, ""); !falls {
			return s, false
		}
	}
	return s, true
}

// stmt walks one statement and says whether control falls out of its end.
func (w *pathWalk) stmt(stmt ast.Stmt, s pathState, label string) (pathState, bool) {
	switch node := stmt.(type) {
	case *ast.BlockStmt:
		return w.block(node.List, s)
	case *ast.ExprStmt:
		if call, ok := ast.Unparen(node.X).(*ast.CallExpr); ok {
			w.discarded[call] = true
		}
		return w.expr(node.X, s), !stopsThePath(node.X, w.frame.info)
	case *ast.AssignStmt:
		if call, ok := ast.Unparen(node.Rhs[0]).(*ast.CallExpr); ok && len(node.Rhs) == 1 && allBlank(node.Lhs) {
			w.discarded[call] = true
		}
		for _, rhs := range node.Rhs {
			s = w.expr(rhs, s)
		}
		w.sw.g.observeWrite(node, s.checked || !slices.ContainsFunc(node.Rhs, func(rhs ast.Expr) bool { return !w.isCovered(rhs, s) }))
		s = w.assign(node.Lhs, node.Rhs, node.Tok, s)
	case *ast.DeclStmt:
		s = w.expr(declValues(node), s)
		s = w.declare(node, s)
	case *ast.IncDecStmt:
		s = w.expr(node.X, s)
	case *ast.SendStmt:
		s = w.expr(node.Value, s)
	case *ast.ReturnStmt:
		for _, result := range node.Results {
			s = w.expr(result, s)
		}
		w.exit(node.Results, s, node.Pos())
		return s, false
	case *ast.IfStmt:
		return w.ifStmt(node, s)
	case *ast.ForStmt:
		return w.loop(node.Init, node.Cond, node.Body, s, label)
	case *ast.RangeStmt:
		return w.rangeLoop(node, s, label)
	case *ast.SwitchStmt:
		return w.switchStmt(node.Init, node.Tag, node.Body, s, label)
	case *ast.TypeSwitchStmt:
		return w.switchStmt(node.Init, nil, node.Body, s, label)
	case *ast.SelectStmt:
		return w.switchStmt(nil, nil, node.Body, s, label)
	case *ast.LabeledStmt:
		return w.stmt(node.Stmt, s, node.Label.Name)
	case *ast.BranchStmt:
		w.branch(node, s)
		return s, false
	}
	return s, true
}

// declValues gathers the initial values of a var block as one expression list.
func declValues(decl *ast.DeclStmt) ast.Expr {
	gen, ok := decl.Decl.(*ast.GenDecl)
	if !ok {
		return nil
	}
	values := &ast.CompositeLit{}
	for _, spec := range gen.Specs {
		if vs, ok := spec.(*ast.ValueSpec); ok {
			values.Elts = append(values.Elts, vs.Values...)
		}
	}
	return values
}

// ifStmt walks each branch once for each way into it, so a check one way
// proves does not stand for another. In err != nil || !allowed, the first
// way in knows err is set, and the second knows the answer.
func (w *pathWalk) ifStmt(node *ast.IfStmt, s pathState) (pathState, bool) {
	if node.Init != nil {
		s, _ = w.stmt(node.Init, s, "")
	}
	s = w.expr(node.Cond, s)
	var ends []pathState
	var falls []bool
	for _, entry := range w.cases(node.Cond, true, s) {
		end, fell := w.block(node.Body.List, entry)
		ends, falls = append(ends, end), append(falls, fell)
	}
	for _, entry := range w.cases(node.Cond, false, s) {
		end, fell := entry, true
		if node.Else != nil {
			end, fell = w.stmt(node.Else, entry, "")
		}
		ends, falls = append(ends, end), append(falls, fell)
	}
	return joinPaths(s, ends, falls)
}

// loop walks a body that may run no time at all. What follows keeps covered
// only what is covered on entry and at every way an iteration ends. A pass
// starts from what the passes before it left, so the body is walked again
// until that settles.
func (w *pathWalk) loop(init ast.Stmt, cond ast.Expr, body *ast.BlockStmt, s pathState, label string) (pathState, bool) {
	if init != nil {
		s, _ = w.stmt(init, s, "")
	}
	s = w.expr(cond, s)
	return w.passes(body, s, label, func(top pathState) pathState { return top }), true
}

// rangeLoop is loop over a range, whose key and value are covered where what
// it ranges over is.
func (w *pathWalk) rangeLoop(node *ast.RangeStmt, s pathState, label string) (pathState, bool) {
	s = w.expr(node.X, s)
	over := w.isCovered(node.X, s)
	bindKeys := func(top pathState) pathState {
		for _, key := range []ast.Expr{node.Key, node.Value} {
			if obj := assignedObject(key, w.frame.info); key != nil && obj != nil {
				top.covered = withMember(top.covered, obj, over || top.checked)
			}
		}
		return top
	}
	return w.passes(node.Body, s, label, bindKeys), true
}

// maxPasses bounds how often a loop body is walked before its covered set
// settles. Each pass only removes variables, so a body settles within as
// many passes as it assigns variables in a chain.
const maxPasses = 4

func (w *pathWalk) passes(body *ast.BlockStmt, s pathState, label string, top func(pathState) pathState) pathState {
	for range maxPasses {
		target := &breakTarget{label: label, loop: true}
		w.targets = append(w.targets, target)
		end, falls := w.block(body.List, top(s))
		w.targets = w.targets[:len(w.targets)-1]
		ends := slices.Concat(target.breakers, target.continues)
		if falls {
			ends = append(ends, end)
		}
		next := s.covered
		for _, e := range ends {
			next = intersectCovered(next, e.covered)
		}
		if maps.Equal(next, s.covered) {
			return s
		}
		s.covered = next
	}
	s.covered = nil
	return s
}

// switchStmt walks each clause from the state after the tag. Without a
// default clause the switch may run none of them.
func (w *pathWalk) switchStmt(init ast.Stmt, tag ast.Expr, body *ast.BlockStmt, s pathState, label string) (pathState, bool) {
	if init != nil {
		s, _ = w.stmt(init, s, "")
	}
	s = w.expr(tag, s)
	target := &breakTarget{label: label}
	w.targets = append(w.targets, target)
	var ends []pathState
	var falls []bool
	exhaustive := false
	for _, clause := range body.List {
		var list []ast.Expr
		var stmts []ast.Stmt
		switch c := clause.(type) {
		case *ast.CaseClause:
			list, stmts = c.List, c.Body
		case *ast.CommClause:
			stmts, exhaustive = c.Body, true
			if c.Comm != nil {
				stmts = append([]ast.Stmt{c.Comm}, stmts...)
			}
		}
		exhaustive = exhaustive || (list == nil && isDefault(clause))
		entry := s
		for _, expr := range list {
			entry = w.expr(expr, entry)
		}
		if tag == nil && len(list) == 1 {
			entry = w.joinedCase(list[0], entry)
		}
		end, fell := w.block(stmts, entry)
		ends, falls = append(ends, end), append(falls, fell)
	}
	w.targets = w.targets[:len(w.targets)-1]
	if !exhaustive {
		ends, falls = append(ends, s), append(falls, true)
	}
	for _, breaker := range target.breakers {
		ends, falls = append(ends, breaker), append(falls, true)
	}
	return joinPaths(s, ends, falls)
}

func isDefault(clause ast.Stmt) bool {
	c, ok := clause.(*ast.CaseClause)
	return ok && c.List == nil
}

// branch hands a break's state to the statement it leaves, and a continue's
// to its loop. A goto ends the path here.
func (w *pathWalk) branch(node *ast.BranchStmt, s pathState) {
	for i := len(w.targets) - 1; i >= 0; i-- {
		target := w.targets[i]
		if node.Label != nil && node.Label.Name != target.label {
			continue
		}
		switch {
		case node.Tok == token.BREAK:
			target.breakers = append(target.breakers, s)
			return
		case node.Tok == token.CONTINUE && target.loop:
			target.continues = append(target.continues, s)
			return
		}
	}
}

// joinPaths is the state after the paths that fall out of a statement: a check
// holds only if it holds on each.
func joinPaths(before pathState, ends []pathState, falls []bool) (pathState, bool) {
	joined, someFall := pathState{checked: true, nonNil: before.nonNil}, false
	for i, end := range ends {
		if !falls[i] {
			continue
		}
		if !someFall {
			joined.answers, joined.covered = end.answers, end.covered
		}
		joined.covered = intersectCovered(joined.covered, end.covered)
		for obj := range joined.answers {
			joined.answers = withMember(joined.answers, obj, end.answers[obj])
		}
		joined.checked, someFall = joined.checked && end.checked, true
	}
	if !someFall {
		return before, false
	}
	return joined, true
}

// stopsThePath reports a call that never returns.
func stopsThePath(expr ast.Expr, info *types.Info) bool {
	call, ok := ast.Unparen(expr).(*ast.CallExpr)
	if !ok {
		return false
	}
	if ident, ok := ast.Unparen(call.Fun).(*ast.Ident); ok {
		_, builtin := info.Uses[ident].(*types.Builtin)
		return builtin && ident.Name == "panic"
	}
	name := importedCallee(call, info)
	return name == "os.Exit" || strings.HasPrefix(name, "log.Fatal")
}

// exit records one way out of the function. A return of an error known not to
// be nil is a refusal, and hands out nothing. A return that hands back the
// error of a call that checks has passed that check if the error is nil.
func (w *pathWalk) exit(results []ast.Expr, s pathState, at token.Pos) {
	// What a refusal stored through a reference stays stored, so every way
	// out counts for what it leaves the caller holding.
	w.keepOuter(s)
	w.keepSlots(s)
	if w.shape.errLast && len(results) == w.shape.results && w.refuses(results[len(results)-1], s, w.carriesValue(results)) {
		return
	}
	answered := w.returnAnswers(results, s)
	checked := s.checked || (w.shape.errLast && answered[w.shape.results-1])
	covered := checked || w.resultsCovered(results, s)
	if !checked {
		w.verdict.guards = false
		w.verdict.bare = w.verdict.bare && covered
	}
	if w.carriesValue(results) {
		w.verdict.returns.add(at, covered)
	}
	if w.verdict.exits == 0 {
		w.verdict.answers, w.verdict.captured = answered, w.outerAnswers(s)
	}
	for i := range w.verdict.answers {
		w.verdict.answers[i] = w.verdict.answers[i] && answered[i]
	}
	for obj := range w.verdict.captured {
		w.verdict.captured = withMember(w.verdict.captured, obj, s.answers[obj])
	}
	w.verdict.exits++
}

// keepOuter records, for a literal, which outer variables one way out leaves
// covered: those covered when it ran and those it covered. Each stays
// covered only where every way out leaves it so.
func (w *pathWalk) keepOuter(s pathState) {
	if w.lit == nil {
		return
	}
	if w.verdict.outer != nil {
		for obj, kept := range w.verdict.outer {
			w.verdict.outer[obj] = kept && s.covered[obj]
		}
		return
	}
	w.verdict.outer = map[types.Object]bool{}
	for _, set := range []map[types.Object]bool{w.outerAtEntry, s.covered} {
		for obj := range set {
			if obj.Pos() < w.lit.Pos() || obj.Pos() > w.lit.End() {
				w.verdict.outer[obj] = s.covered[obj]
			}
		}
	}
}

// finishAnswers settles the error result: it carries the answer when every
// success return passed a check, so the caller checks where it is nil.
func (w *pathWalk) finishAnswers() {
	if w.verdict.answers == nil {
		w.verdict.answers = make([]bool, w.shape.results)
	}
	if w.shape.errLast {
		w.verdict.answers[w.shape.results-1] = w.verdict.guards
	}
}

// returnAnswers marks each result of a return that holds the switch's
// answer. That is a variable holding one, a reader's call, or one result of
// a call whose results it passes on whole.
func (w *pathWalk) returnAnswers(results []ast.Expr, s pathState) []bool {
	answered := make([]bool, w.shape.results)
	switch {
	case len(results) == 0:
		for i, obj := range w.shape.named {
			answered[i] = obj != nil && s.answers[obj]
		}
	case len(results) == w.shape.results:
		for i, result := range results {
			answered[i] = w.isAnswer(result, s)
		}
	case len(results) == 1:
		if call, ok := ast.Unparen(results[0]).(*ast.CallExpr); ok {
			for i := range answered {
				answered[i] = w.answerCalls[call].at(i, w.shape.results)
			}
		}
	}
	return answered
}

// outerAnswers are the variables a literal's state holds an answer in that
// were declared outside it, which its caller keeps when the literal is run.
func (w *pathWalk) outerAnswers(s pathState) map[types.Object]bool {
	if w.lit == nil {
		return nil
	}
	var out map[types.Object]bool
	for obj := range s.answers {
		if obj.Pos() < w.lit.Pos() || obj.Pos() > w.lit.End() {
			out = withMember(out, obj, true)
		}
	}
	return out
}

// carriesValue reports a return that hands out something besides its error,
// where every such result is not spelled as a zero value.
func (w *pathWalk) carriesValue(results []ast.Expr) bool {
	values := w.shape.results
	if w.shape.errLast {
		values--
	}
	if values == 0 {
		return false
	}
	if len(results) != w.shape.results {
		return true
	}
	for _, result := range results[:values] {
		if !spelledZero(result) {
			return true
		}
	}
	return false
}

func spelledZero(expr ast.Expr) bool {
	switch e := ast.Unparen(expr).(type) {
	case *ast.Ident:
		return e.Name == "nil" || e.Name == "false"
	case *ast.BasicLit:
		return e.Value == "0" || e.Value == `""` || e.Value == "``"
	case *ast.CompositeLit:
		return len(e.Elts) == 0
	}
	return false
}

// refuses reports an error result that cannot be nil. That is a variable a
// condition proved set, a literal, a sentinel, or a new error. It is also a
// call whose types and returns prove one, or one of those wrapped. Beside a
// value that is not spelled zero, only an error a condition proved set is one.
func (w *pathWalk) refuses(expr ast.Expr, s pathState, valued bool) bool {
	info, g := w.frame.info, w.sw.g
	switch e := ast.Unparen(expr).(type) {
	case *ast.Ident:
		obj := info.Uses[e]
		return obj != nil && (s.nonNil[obj] || (!valued && g.isSentinel(obj)))
	case *ast.CompositeLit:
		return !valued
	case *ast.UnaryExpr:
		_, lit := ast.Unparen(e.X).(*ast.CompositeLit)
		return !valued && e.Op == token.AND && lit
	case *ast.SelectorExpr:
		return !valued && (g.isSentinel(info.Uses[e.Sel]) || g.declaredSentinel(e, info))
	case *ast.CallExpr:
		if !valued && (slices.Contains(g.config.refusalConstructors, g.outsideCallee(e, info)) || g.alwaysRefuses(calledFunc(e, info))) {
			return true
		}
		return slices.ContainsFunc(e.Args, func(arg ast.Expr) bool {
			_, literal := ast.Unparen(arg).(*ast.CompositeLit)
			return !literal && w.refuses(arg, s, valued)
		})
	}
	return false
}

func isPackageVar(obj types.Object) bool {
	v, ok := obj.(*types.Var)
	return ok && v.Pkg() != nil && v.Parent() == v.Pkg().Scope()
}

// maxCases bounds the ways into one branch the walk tells apart. Past it they
// are joined, and a check holds only if it holds on each.
const maxCases = 16

// cases are the states inside a branch of cond, one for each way into it.
// An error compared with nil, or one errors.Is or errors.As matched, is known
// set. A condition on an answer decides it: an error answer checks where it
// is nil, and a bool answer checks on either side.
func (w *pathWalk) cases(cond ast.Expr, taken bool, s pathState) []pathState {
	info := w.frame.info
	switch e := ast.Unparen(cond).(type) {
	case *ast.UnaryExpr:
		if e.Op == token.NOT {
			return w.cases(e.X, !taken, s)
		}
	case *ast.BinaryExpr:
		return w.binaryCases(e, taken, s)
	case *ast.CallExpr:
		name := importedCallee(e, info)
		if taken && (name == "errors.Is" || name == "errors.As") && len(e.Args) == 2 && !isNilIdent(ast.Unparen(e.Args[1])) {
			if ident, ok := ast.Unparen(e.Args[0]).(*ast.Ident); ok && info.Uses[ident] != nil {
				return []pathState{s.withNonNil(info.Uses[ident])}
			}
		}
		s.checked = s.checked || (taken && w.answerCalls[e].at(0, 1)) || (!taken && w.floorHeld(e))
	case *ast.IndexExpr:
		s.checked = s.checked || (!taken && w.floorHeld(e))
	case *ast.Ident:
		s.checked = s.checked || (taken && (s.answers[info.Uses[e]] || w.hits[info.Uses[e]]))
	}
	return []pathState{s}
}

func (w *pathWalk) binaryCases(e *ast.BinaryExpr, taken bool, s pathState) []pathState {
	then := func(states []pathState, y bool) []pathState {
		var out []pathState
		for _, state := range states {
			out = append(out, w.cases(e.Y, y, state)...)
		}
		return out
	}
	switch {
	case e.Op == token.LAND && taken, e.Op == token.LOR && !taken:
		return distinctCases(s, then(w.cases(e.X, taken, s), taken))
	case e.Op == token.LAND, e.Op == token.LOR:
		return distinctCases(s, append(w.cases(e.X, taken, s), then(w.cases(e.X, !taken, s), taken)...))
	case e.Op == token.EQL || e.Op == token.NEQ:
		isNil := (e.Op == token.EQL) == taken
		if obj := comparedWithNil(e, w.frame.info); obj != nil && !isNil {
			return []pathState{s.withNonNil(obj)}
		} else if obj != nil {
			s.checked = s.checked || s.answers[obj]
		}
		if call := calledBesideNil(e); call != nil && isNil {
			s.checked = s.checked || w.answerCalls[call].at(0, 1)
		}
	}
	return []pathState{s}
}

// distinctCases drops a way in that holds what another does, and joins them
// all past maxCases.
func distinctCases(before pathState, states []pathState) []pathState {
	var out []pathState
	for _, state := range states {
		if !slices.ContainsFunc(out, state.same) {
			out = append(out, state)
		}
	}
	if len(out) > maxCases {
		joined, _ := joinPaths(before, out, slices.Repeat([]bool{true}, len(out)))
		return []pathState{joined}
	}
	return out
}

// joinedCase is the state inside a switch clause cond selects, every way in
// joined.
func (w *pathWalk) joinedCase(cond ast.Expr, s pathState) pathState {
	ways := w.cases(cond, true, s)
	joined, _ := joinPaths(s, ways, slices.Repeat([]bool{true}, len(ways)))
	return joined
}

// floorHeld reports a condition that is true where the floor withholds a
// figure: thin[i], or slices.Contains(thin, true), over a floor's answer.
func (w *pathWalk) floorHeld(cond ast.Expr) bool {
	var held ast.Expr
	switch e := ast.Unparen(cond).(type) {
	case *ast.IndexExpr:
		held = e.X
	case *ast.CallExpr:
		if index, isFloor := w.floorCalls[e]; isFloor {
			return index == 0
		}
		if importedCallee(e, w.frame.info) != "slices.Contains" || len(e.Args) != 2 {
			return false
		}
		if value := w.frame.info.Types[e.Args[1]].Value; value == nil || value.String() != "true" {
			return false
		}
		held = e.Args[0]
	}
	switch e := ast.Unparen(held).(type) {
	case *ast.Ident:
		return w.floors[w.frame.info.Uses[e]]
	case *ast.CallExpr:
		index, isFloor := w.floorCalls[e]
		return isFloor && index == 0
	}
	return false
}

// calledBesideNil is the call a comparison with nil reads, as in
// auth.Require(ctx) != nil.
func calledBesideNil(e *ast.BinaryExpr) *ast.CallExpr {
	x, y := ast.Unparen(e.X), ast.Unparen(e.Y)
	if isNilIdent(x) {
		x, y = y, x
	}
	call, ok := x.(*ast.CallExpr)
	if !ok || !isNilIdent(y) {
		return nil
	}
	return call
}

func comparedWithNil(e *ast.BinaryExpr, info *types.Info) types.Object {
	x, y := ast.Unparen(e.X), ast.Unparen(e.Y)
	if ident, ok := x.(*ast.Ident); ok && ident.Name == "nil" {
		x = y
	} else if ident, ok := y.(*ast.Ident); !ok || ident.Name != "nil" {
		return nil
	}
	if ident, ok := x.(*ast.Ident); ok {
		return info.Uses[ident]
	}
	return nil
}

// importedCallee spells a call of an imported function as "pkg.Name", the
// package named by the last element of its path; anything else is "".
func importedCallee(call *ast.CallExpr, info *types.Info) string {
	sel, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	ident, ok := ast.Unparen(sel.X).(*ast.Ident)
	if !ok {
		return ""
	}
	if pkg, ok := info.Uses[ident].(*types.PkgName); ok {
		return path.Base(pkg.Imported().Path()) + "." + sel.Sel.Name
	}
	return ""
}
