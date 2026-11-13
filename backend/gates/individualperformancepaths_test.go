// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

package gates

// The path walk of the individual-performance gate. A figure counts as
// checked only when every path to it passed a read of the switch.
//
// Branches join so that a check on one does not cover its sibling. A call
// checks when all it may run checks on every success return. For a func-typed
// parameter, that is the value this call site passes.

import (
	"go/ast"
	"go/token"
	"go/types"
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

func (p *figurePoints) add(at token.Pos, checked bool) {
	p.count++
	if !checked {
		p.unchecked = append(p.unchecked, at)
	}
}

func (p *figurePoints) merge(other figurePoints) {
	p.count += other.count
	p.unchecked = append(p.unchecked, other.unchecked...)
}

// walkVerdict is what walking one function from one entry state found.
// guards says every success return passed a check, and bare says each that did
// not carries no figure. writes are the response writes that carry a figure;
// returns are the returns that carry a value.
type walkVerdict struct {
	guards  bool
	bare    bool
	writes  figurePoints
	returns figurePoints
}

type walkKey struct {
	decl          *types.Func
	lit           *ast.FuncLit
	checked, bare bool
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
	key := walkKey{fn.decl, fn.lit, entry.checked, entry.bare}
	reusable := len(next.bound) == 0 && len(next.names) == 0 && fn.env == nil
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
		shape: shapeOf(ft, info), verdict: walkVerdict{guards: true, bare: true},
	}
	// Only a function that returns nothing keeps the caller's figure-free
	// state, as a writer of what it was handed does. Any other makes its own.
	start := pathState{checked: entry.checked, bare: entry.bare && w.shape.results == 0}
	end, falls := w.block(body.List, start)
	if falls {
		w.exit(nil, end, body.Rbrace)
	}
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

// resultShape is how many results a function declares and whether the last
// is an error.
type resultShape struct {
	results int
	errLast bool
	named   bool
}

func shapeOf(ft *ast.FuncType, info *types.Info) resultShape {
	var shape resultShape
	if ft.Results == nil {
		return shape
	}
	for _, field := range ft.Results.List {
		shape.results += max(1, len(field.Names))
		shape.named = shape.named || len(field.Names) > 0
	}
	last := ft.Results.List[len(ft.Results.List)-1]
	shape.errLast = types.Identical(info.TypeOf(last.Type), errorType)
	return shape
}

// pathState is what holds on one path. checked says a check was passed. bare
// says all it holds came from a call that answers no figure. nonNil holds the
// error variables known not to be nil.
type pathState struct {
	checked bool
	bare    bool
	nonNil  map[types.Object]bool
}

// covered reports a path whose output needs nothing more from the switch.
func (s pathState) covered() bool { return s.checked || s.bare }

func (s pathState) withNonNil(obj types.Object) pathState {
	next := map[types.Object]bool{obj: true}
	for known := range s.nonNil {
		next[known] = true
	}
	s.nonNil = next
	return s
}

// breakTarget is a statement a break leaves, and for a loop one a continue
// goes back to. Each keeps the states that left it that way.
type breakTarget struct {
	label     string
	loop      bool
	breakers  []pathState
	continues []pathState
}

// pathWalk walks the body of one function.
type pathWalk struct {
	sw      *switchWalk
	frame   *pathFrame
	shape   resultShape
	verdict walkVerdict
	targets []*breakTarget
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
		return w.expr(node.X, s), !stopsThePath(node.X, w.frame.info)
	case *ast.AssignStmt:
		for _, rhs := range node.Rhs {
			s = w.expr(rhs, s)
		}
	case *ast.DeclStmt:
		s = w.expr(declValues(node), s)
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
		return w.loop(nil, node.X, node.Body, s, label)
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

func (w *pathWalk) ifStmt(node *ast.IfStmt, s pathState) (pathState, bool) {
	if node.Init != nil {
		s, _ = w.stmt(node.Init, s, "")
	}
	s = w.expr(node.Cond, s)
	thenState, thenFalls := w.block(node.Body.List, narrowed(node.Cond, true, s, w.frame.info))
	elseState, elseFalls := narrowed(node.Cond, false, s, w.frame.info), true
	if node.Else != nil {
		elseState, elseFalls = w.stmt(node.Else, elseState, "")
	}
	return joinPaths(s, []pathState{thenState, elseState}, []bool{thenFalls, elseFalls})
}

// loop walks a body that may run no time at all. What follows holds only what
// held on entry and at every way an iteration ends.
func (w *pathWalk) loop(init ast.Stmt, cond ast.Expr, body *ast.BlockStmt, s pathState, label string) (pathState, bool) {
	if init != nil {
		s, _ = w.stmt(init, s, "")
	}
	s = w.expr(cond, s)
	target := &breakTarget{label: label, loop: true}
	w.targets = append(w.targets, target)
	end, falls := w.block(body.List, s)
	w.targets = w.targets[:len(w.targets)-1]
	ends := slices.Concat(target.breakers, target.continues)
	if falls {
		ends = append(ends, end)
	}
	for _, e := range ends {
		s.bare = s.bare && e.bare
	}
	return s, true
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
	joined, someFall := pathState{checked: true, bare: true, nonNil: before.nonNil}, false
	for i, end := range ends {
		if falls[i] {
			joined.checked, joined.bare, someFall = joined.checked && end.checked, joined.bare && end.bare, true
		}
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
// be nil is a refusal, and hands out nothing.
func (w *pathWalk) exit(results []ast.Expr, s pathState, at token.Pos) {
	if w.shape.errLast && len(results) == w.shape.results && w.refuses(results[len(results)-1], s, w.carriesValue(results)) {
		return
	}
	if !s.checked {
		w.verdict.guards = false
		w.verdict.bare = w.verdict.bare && s.bare
	}
	if w.carriesValue(results) {
		w.verdict.returns.add(at, s.covered())
	}
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

// refusalConstructors are outside functions that always answer a new error.
// The stand-in importer leaves their bodies unread, so they are declared here.
// A return beside a value that is not spelled zero is no refusal through them.
var refusalConstructors = map[string]bool{"fmt.Errorf": true, "errors.New": true, "httperr.Validation": true}

// refuses reports an error result that cannot be nil. That is a variable a
// condition proved set, a literal, a sentinel, a new error, or a wrapped one.
func (w *pathWalk) refuses(expr ast.Expr, s pathState, valued bool) bool {
	info := w.frame.info
	switch e := ast.Unparen(expr).(type) {
	case *ast.Ident:
		obj := info.Uses[e]
		return obj != nil && (s.nonNil[obj] || (isPackageVar(obj) && isSentinelName(obj.Name())))
	case *ast.CompositeLit:
		return true
	case *ast.UnaryExpr:
		_, lit := ast.Unparen(e.X).(*ast.CompositeLit)
		return e.Op == token.AND && lit
	case *ast.SelectorExpr:
		return qualifiesAPackage(e.X, info) && isSentinelName(e.Sel.Name)
	case *ast.CallExpr:
		return (!valued && refusalConstructors[importedCallee(e, info)]) || slices.ContainsFunc(e.Args, func(arg ast.Expr) bool {
			_, literal := ast.Unparen(arg).(*ast.CompositeLit)
			return !literal && w.refuses(arg, s, valued)
		})
	}
	return false
}

// qualifiesAPackage reports a selector's left side naming an import. An import
// whose path ends in a version (pgx/v5) leaves its name unresolved under the
// stand-in importer, and is read as one too.
func qualifiesAPackage(x ast.Expr, info *types.Info) bool {
	ident, ok := ast.Unparen(x).(*ast.Ident)
	if !ok {
		return false
	}
	obj := info.Uses[ident]
	_, isPkg := obj.(*types.PkgName)
	return isPkg || obj == nil
}

// isSentinelName reports a package error spelled the Go way, errX or ErrX. The
// name is read because errors.New under a stand-in import leaves no type.
func isSentinelName(name string) bool {
	return strings.HasPrefix(name, "err") || strings.HasPrefix(name, "Err")
}

func isPackageVar(obj types.Object) bool {
	v, ok := obj.(*types.Var)
	return ok && v.Pkg() != nil && v.Parent() == v.Pkg().Scope()
}

// narrowed is the state inside a branch of cond: an error compared with nil,
// or one errors.Is or errors.As matched, is known set.
func narrowed(cond ast.Expr, taken bool, s pathState, info *types.Info) pathState {
	switch e := ast.Unparen(cond).(type) {
	case *ast.UnaryExpr:
		if e.Op == token.NOT {
			return narrowed(e.X, !taken, s, info)
		}
	case *ast.BinaryExpr:
		switch {
		case e.Op == token.LAND && taken, e.Op == token.LOR && !taken:
			return narrowed(e.Y, taken, narrowed(e.X, taken, s, info), info)
		case (e.Op == token.NEQ && taken) || (e.Op == token.EQL && !taken):
			if obj := comparedWithNil(e, info); obj != nil {
				return s.withNonNil(obj)
			}
		}
	case *ast.CallExpr:
		name := importedCallee(e, info)
		if taken && (name == "errors.Is" || name == "errors.As") && len(e.Args) == 2 && !isNilIdent(ast.Unparen(e.Args[1])) {
			if ident, ok := ast.Unparen(e.Args[0]).(*ast.Ident); ok && info.Uses[ident] != nil {
				return s.withNonNil(info.Uses[ident])
			}
		}
	}
	return s
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
