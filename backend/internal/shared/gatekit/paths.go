// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gatekit

// Following one value along every path of control a function body can take,
// for the censuses that must know a value reached where it had to go on EVERY
// path, not merely on one. A census that asked whether some statement after a
// binding hands the value on is satisfied by one branch of an if while the
// other branch drops it; this walk is what makes the other branch count.
//
// The walk reads the syntax Go gives it — blocks, if, for, range, switch,
// select, break and continue — and resolves no values. Where it cannot tell
// where a path goes (a goto, a labelled break or continue, a fallthrough) it
// assumes the worst: the path leaves unsettled. A census built on it can then
// only err towards naming too much, never towards reading a smaller tree.

import (
	"go/ast"
	"go/token"
	"go/types"
)

// Step is what one statement does to the value being followed.
type Step int

const (
	// Passes leaves the value as it was, still owed.
	Passes Step = iota
	// Settles is the statement the value had to reach: every path through it
	// is done.
	Settles
	// Drops loses the value — it is overwritten, or the function returns
	// without it — so the path through it is unsettled.
	Drops
)

// Paths follows a value through one function body.
type Paths struct {
	// Body is the function body the value lives in. A path that reaches its
	// end, or a return Step does not settle, leaves unsettled. When the value
	// is bound inside a function literal in Body, that literal's end is the end.
	Body *ast.BlockStmt
	// Step judges one simple statement: an assignment, a call, a declaration,
	// a defer, a go, a send, a return, or the init of an if, for or switch.
	// A function literal inside the statement is part of it, not walked.
	Step func(ast.Stmt) Step
	// Owed reports whether the value is still owed once cond has evaluated to
	// outcome: false for `err != nil` evaluating false, since a nil error has
	// nowhere it must go. Nil means every condition leaves it owed.
	Owed func(cond ast.Expr, outcome bool) bool
}

// Unsettled reports whether some path of control from just after start — a
// statement inside Body, or nil for the top of Body — leaves the function
// without passing a statement Step settles.
func (p Paths) Unsettled(start ast.Stmt) bool {
	w := &pathWalk{Paths: p, start: start, parents: map[ast.Node]ast.Node{}}
	var stack []ast.Node
	ast.Inspect(p.Body, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		if len(stack) > 0 {
			w.parents[n] = stack[len(stack)-1]
		}
		stack = append(stack, n)
		return true
	})
	if start == nil {
		out := w.list(p.Body.List)
		return out.exits || out.falls
	}
	return w.leave(start, pathOut{falls: true})
}

// Proves reports whether cond evaluating to outcome proves what fact says of
// one of its atoms, reading through !, && and ||: `a && b` true proves what
// either half true proves, and false only what both halves false prove.
func Proves(cond ast.Expr, outcome bool, fact func(atom ast.Expr, outcome bool) bool) bool {
	switch c := ast.Unparen(cond).(type) {
	case *ast.UnaryExpr:
		if c.Op == token.NOT {
			return Proves(c.X, !outcome, fact)
		}
	case *ast.BinaryExpr:
		switch c.Op {
		case token.LAND:
			if outcome {
				return Proves(c.X, true, fact) || Proves(c.Y, true, fact)
			}
			return Proves(c.X, false, fact) && Proves(c.Y, false, fact)
		case token.LOR:
			if outcome {
				return Proves(c.X, true, fact) && Proves(c.Y, true, fact)
			}
			return Proves(c.X, false, fact) || Proves(c.Y, false, fact)
		}
	}
	return fact(cond, outcome)
}

// ErrorSettled reports whether cond evaluating to outcome proves the error
// subject recognises needs nothing more on that path: it is nil (`err == nil`
// true, `err != nil` false), or errors.Is or errors.As has recognised its kind,
// which the code then answers on that kind's own terms — a page it could not
// read as a 422, a rival replica's claim as nothing wrong. It is the Owed a
// census following an error hands its Paths, read through !, && and ||.
//
// Recognising a kind is a claim about which error this is, so an errors.As
// into the bare error interface, or an empty one, which every error matches,
// recognises nothing. The errors package is found through info, so a local
// value that happens to be named errors is not it.
func ErrorSettled(cond ast.Expr, outcome bool, info *types.Info, subject func(ast.Expr) bool) bool {
	return Proves(cond, outcome, func(atom ast.Expr, outcome bool) bool {
		return nilWhen(atom, outcome, subject) || (outcome && recognisesKind(atom, info, subject))
	})
}

// nilWhen is ErrorSettled's comparison half, for one atom.
func nilWhen(atom ast.Expr, outcome bool, subject func(ast.Expr) bool) bool {
	c, ok := ast.Unparen(atom).(*ast.BinaryExpr)
	if !ok {
		return false
	}
	switch c.Op {
	case token.EQL:
		return outcome && comparesToNil(c, subject)
	case token.NEQ:
		return !outcome && comparesToNil(c, subject)
	}
	return false
}

// recognisesKind reports whether atom is errors.Is or errors.As asked of the
// subject, into anything narrower than every error.
func recognisesKind(atom ast.Expr, info *types.Info, subject func(ast.Expr) bool) bool {
	call, ok := ast.Unparen(atom).(*ast.CallExpr)
	if !ok || len(call.Args) != 2 || !subject(call.Args[0]) {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	name, ok := info.Uses[pkg].(*types.PkgName)
	if !ok || name.Imported().Path() != "errors" {
		return false
	}
	switch sel.Sel.Name {
	case "Is":
		return true
	case "As":
		target, ok := info.TypeOf(call.Args[1]).(*types.Pointer)
		if !ok {
			// errors.As takes a pointer, so a target without one is a target
			// the check could not type — an import standing in empty.
			return true
		}
		iface, ok := types.Unalias(target.Elem()).Underlying().(*types.Interface)
		return !ok || (!iface.Empty() && !types.Identical(iface, errorInterface))
	}
	return false
}

var errorInterface = ErrorInterface()

// ErrorInterface is the predeclared error interface, for a census asking
// whether a type is an error or whether an errors.As target could hold any.
func ErrorInterface() *types.Interface {
	iface, ok := types.Universe.Lookup("error").Type().Underlying().(*types.Interface)
	if !ok {
		panic("gatekit: the predeclared error type is not an interface; go/types changed its universe")
	}
	return iface
}

func comparesToNil(c *ast.BinaryExpr, subject func(ast.Expr) bool) bool {
	isNil := func(e ast.Expr) bool {
		ident, ok := ast.Unparen(e).(*ast.Ident)
		return ok && ident.Name == "nil"
	}
	return (isNil(c.Y) && subject(c.X)) || (isNil(c.X) && subject(c.Y))
}

// pathOut is how the unsettled paths through a statement leave it: into the
// statement after it, out of the innermost loop or switch by break, to the
// next iteration by continue, or out of the function.
type pathOut struct{ falls, breaks, continues, exits bool }

func (o pathOut) join(other pathOut) pathOut {
	return pathOut{
		falls: o.falls || other.falls, breaks: o.breaks || other.breaks,
		continues: o.continues || other.continues, exits: o.exits || other.exits,
	}
}

type pathWalk struct {
	Paths
	start   ast.Stmt
	parents map[ast.Node]ast.Node
}

// list walks a statement list entered unsettled.
func (w *pathWalk) list(stmts []ast.Stmt) pathOut {
	var out pathOut
	for _, stmt := range stmts {
		next := w.stmt(stmt)
		out = out.join(pathOut{breaks: next.breaks, continues: next.continues, exits: next.exits})
		if !next.falls || out.exits {
			return out
		}
	}
	out.falls = true
	return out
}

// stmt walks one statement entered unsettled. Reaching start again — a loop
// coming round — binds a new value over the one still owed.
func (w *pathWalk) stmt(stmt ast.Stmt) pathOut {
	if stmt == w.start && stmt != nil {
		return pathOut{exits: true}
	}
	switch s := stmt.(type) {
	case *ast.BlockStmt:
		return w.list(s.List)
	case *ast.LabeledStmt:
		return w.stmt(s.Stmt)
	case *ast.IfStmt:
		return w.then(w.simple(s.Init), func() pathOut { return w.branches(s) })
	case *ast.ForStmt:
		return w.then(w.simple(s.Init), func() pathOut { return w.loop(s.Cond == nil, s.Body) })
	case *ast.RangeStmt:
		return w.loop(false, s.Body)
	case *ast.SwitchStmt:
		return w.then(w.simple(s.Init), func() pathOut { return w.clauses(s.Body, s.Tag == nil) })
	case *ast.TypeSwitchStmt:
		return w.then(w.simple(s.Init), func() pathOut {
			return w.then(w.simple(s.Assign), func() pathOut { return w.clauses(s.Body, false) })
		})
	case *ast.SelectStmt:
		return w.clauses(s.Body, false)
	case *ast.BranchStmt:
		return branch(s)
	case *ast.ReturnStmt:
		if w.Step(s) == Settles {
			return pathOut{}
		}
		return pathOut{exits: true}
	}
	return w.simple(stmt)
}

// branch is where a break, continue, goto or fallthrough sends a path. Only
// an unlabelled break or continue is followed; the rest leave unsettled.
func branch(s *ast.BranchStmt) pathOut {
	if s.Label == nil {
		switch s.Tok {
		case token.BREAK:
			return pathOut{breaks: true}
		case token.CONTINUE:
			return pathOut{continues: true}
		}
	}
	return pathOut{exits: true}
}

// simple walks one statement Step judges; a nil one passes.
func (w *pathWalk) simple(stmt ast.Stmt) pathOut {
	if stmt == nil {
		return pathOut{falls: true}
	}
	if stmt == w.start {
		return pathOut{exits: true}
	}
	switch w.Step(stmt) {
	case Settles:
		return pathOut{}
	case Drops:
		return pathOut{exits: true}
	}
	return pathOut{falls: true}
}

// then runs rest on the paths first lets fall through.
func (w *pathWalk) then(first pathOut, rest func() pathOut) pathOut {
	if !first.falls || first.exits {
		return first
	}
	next := rest()
	return pathOut{
		falls: next.falls, breaks: first.breaks || next.breaks,
		continues: first.continues || next.continues, exits: next.exits,
	}
}

// branches walks an if after its init, skipping the arm its condition proves
// the value needs nothing on.
func (w *pathWalk) branches(s *ast.IfStmt) pathOut {
	var out pathOut
	if w.owed(s.Cond, true) {
		out = out.join(w.list(s.Body.List))
	}
	if w.owed(s.Cond, false) {
		if s.Else == nil {
			out.falls = true
		} else {
			out = out.join(w.stmt(s.Else))
		}
	}
	return out
}

func (w *pathWalk) owed(cond ast.Expr, outcome bool) bool {
	return w.Owed == nil || w.Owed(cond, outcome)
}

// loop walks a loop body entered from above. The body may run no times, and
// every iteration enters it the same way, so one walk stands for all; the loop
// is left by its condition failing, or by a break when it has no condition.
func (w *pathWalk) loop(forever bool, body *ast.BlockStmt) pathOut {
	out := w.list(body.List)
	return pathOut{falls: out.breaks || !forever, exits: out.exits}
}

// clauses walks the cases of a switch or select. A tagless switch case whose
// expressions prove the value owes nothing needs nothing walked, nor does any
// case after one whose failing proved it.
func (w *pathWalk) clauses(body *ast.BlockStmt, tagless bool) pathOut {
	var out pathOut
	settledBelow, hasDefault, isSelect := false, false, false
	for _, clause := range body.List {
		switch c := clause.(type) {
		case *ast.CaseClause:
			hasDefault = hasDefault || c.List == nil
			walk := !settledBelow
			if tagless && c.List != nil {
				owedIn, settles := w.caseOwed(c.List)
				walk = walk && owedIn
				settledBelow = settledBelow || settles
			}
			if walk {
				out = out.join(w.list(c.Body))
			}
		case *ast.CommClause:
			isSelect = true
			out = out.join(w.list(append([]ast.Stmt{c.Comm}, c.Body...)))
		}
	}
	falls := out.falls || out.breaks || (!hasDefault && !isSelect && !settledBelow)
	return pathOut{falls: falls, continues: out.continues, exits: out.exits}
}

// caseOwed reads the expressions of a tagless switch case: whether the value
// is still owed when the case runs, and whether the case failing proves it
// owes nothing in every case below. A case runs when any of its expressions is
// true, so it owes nothing only when each of them proves that; one proving it
// false settles every case below.
func (w *pathWalk) caseOwed(exprs []ast.Expr) (owedIn, settlesBelow bool) {
	for _, expr := range exprs {
		owedIn = owedIn || w.owed(expr, true)
		settlesBelow = settlesBelow || !w.owed(expr, false)
	}
	return owedIn, settlesBelow
}

// leave follows the paths leaving node outwards, to the end of the function.
func (w *pathWalk) leave(node ast.Node, out pathOut) bool {
	if out.exits {
		return true
	}
	if !out.falls && !out.breaks && !out.continues {
		return false
	}
	switch p := w.parents[node].(type) {
	case *ast.BlockStmt:
		return w.leaveBlock(p, node, out)
	case *ast.CaseClause:
		return w.leave(p, w.rest(p.Body, node, out))
	case *ast.CommClause:
		return w.leave(p, w.leaveComm(p, node, out))
	case *ast.LabeledStmt:
		return w.leave(p, out)
	case *ast.IfStmt:
		return w.leave(p, w.leaveIf(p, node, out))
	case *ast.SwitchStmt:
		return w.leave(p, w.leaveSwitch(node, p.Init, nil, p.Body, p.Tag == nil, out))
	case *ast.TypeSwitchStmt:
		return w.leave(p, w.leaveSwitch(node, p.Init, p.Assign, p.Body, false, out))
	case *ast.ForStmt:
		return w.leaveLoop(p, node, p.Init, p.Cond == nil, p.Body, out)
	case *ast.RangeStmt:
		return w.leaveLoop(p, node, nil, false, p.Body, out)
	}
	return true
}

// leaveComm is the paths leaving a select case that node, its communication
// or a statement of its body, sent them out of.
func (w *pathWalk) leaveComm(clause *ast.CommClause, node ast.Node, out pathOut) pathOut {
	if node == clause.Comm {
		return w.then(out, func() pathOut { return w.list(clause.Body) })
	}
	return w.rest(clause.Body, node, out)
}

// leaveIf is the paths leaving an if that node sent them out of: from its init
// they still pass through the arms.
func (w *pathWalk) leaveIf(stmt *ast.IfStmt, node ast.Node, out pathOut) pathOut {
	if node == stmt.Init {
		return w.then(out, func() pathOut { return w.branches(stmt) })
	}
	return out
}

// leaveBlock resumes after node in block: the function ends at its body, and a
// switch or select is left from one of its clauses.
func (w *pathWalk) leaveBlock(block *ast.BlockStmt, node ast.Node, out pathOut) bool {
	parent := w.parents[block]
	switch parent.(type) {
	case *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
		return w.leave(block, pathOut{falls: out.falls || out.breaks, continues: out.continues})
	}
	out = w.rest(block.List, node, out)
	if _, literal := parent.(*ast.FuncLit); literal || block == w.Body {
		return out.falls || out.exits
	}
	return w.leave(block, out)
}

// rest walks the statements after node in list for the paths falling out of it.
func (w *pathWalk) rest(list []ast.Stmt, node ast.Node, out pathOut) pathOut {
	if !out.falls {
		return out
	}
	for i, stmt := range list {
		if ast.Node(stmt) == node {
			next := w.list(list[i+1:])
			return pathOut{
				falls: next.falls, breaks: out.breaks || next.breaks,
				continues: out.continues || next.continues, exits: next.exits,
			}
		}
	}
	return out
}

// leaveSwitch is the paths leaving a switch that node, its init or assign, or
// its body, sent them out of.
func (w *pathWalk) leaveSwitch(node ast.Node, init, assign ast.Stmt, body *ast.BlockStmt, tagless bool, out pathOut) pathOut {
	if node == init {
		return w.then(out, func() pathOut {
			return w.then(w.simple(assign), func() pathOut { return w.clauses(body, tagless) })
		})
	}
	if node == assign && assign != nil {
		return w.then(out, func() pathOut { return w.clauses(body, tagless) })
	}
	return out
}

// leaveLoop follows the paths leaving part of a loop. From its init they enter
// the loop; from its body, a path that falls through or continues comes round
// to the top again, where reaching start binds a new value over the owed one.
func (w *pathWalk) leaveLoop(loop ast.Stmt, node, init ast.Node, forever bool, body *ast.BlockStmt, out pathOut) bool {
	if node == init && init != nil {
		return w.leave(loop, w.then(out, func() pathOut { return w.loop(forever, body) }))
	}
	if node != body {
		// A loop's condition and post statement are not where a value is bound.
		return true
	}
	next := pathOut{falls: out.breaks || !forever}
	if out.falls || out.continues {
		again := w.list(body.List)
		next.exits = again.exits
		next.falls = next.falls || again.breaks
	}
	return w.leave(loop, next)
}
