// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gatekit

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"testing"
)

// Each planted body follows the value v from where it is first bound — or,
// when nothing binds it, from the top, as a parameter — and says whether some
// path leaves without settling it. settle(v) and a return reading v settle
// it; assigning v again drops it.
func TestPathsFindTheOnePathThatLeavesAValueUnsettled(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		body      string
		unsettled bool
	}{
		"settled on the straight path":         {`v := f(); settle(v)`, false},
		"settled on one arm of an if only":     {`v := f(); if c { settle(v) }`, true},
		"settled on both arms":                 {`v := f(); if c { settle(v) } else { settle(v) }`, false},
		"settled on each arm of an else-if":    {`v := f(); if c { settle(v) } else if d { settle(v) } else { return v }`, false},
		"an else-if arm missing":               {`v := f(); if c { settle(v) } else if d { settle(v) }`, true},
		"nil on the arm that skips it":         {`v := f(); if v != nil { settle(v) }`, false},
		"nil proved by ==, then settled":       {`v := f(); if v == nil { return nil }; settle(v)`, false},
		"nil proved through !":                 {`v := f(); if !(v != nil) { return nil }; settle(v)`, false},
		"non-nil and another condition":        {`v := f(); if v != nil && c { settle(v) }`, true},
		"nil or another condition returning":   {`v := f(); if v == nil || c { return nil }; settle(v)`, true},
		"nil proved by an or failing":          {`v := f(); if v == nil && c { return nil }; if v != nil || c { settle(v) }`, false},
		"returned early without it":            {`v := f(); if c { return nil }; settle(v)`, true},
		"returned":                             {`v := f(); return v`, false},
		"falling off the end":                  {`v := f(); _ = c`, true},
		"overwritten before it is settled":     {`v := f(); v = f(); settle(v)`, true},
		"a loop that may not run":              {`v := f(); for c { settle(v); return nil }; return nil`, true},
		"a loop without a condition":           {`v := f(); for { if c { settle(v); return nil } }`, false},
		"a break out of a loop without one":    {`v := f(); for { if c { break }; settle(v); return nil }; return nil`, true},
		"a range settling each binding":        {`for range xs { v := f(); settle(v) }; return nil`, false},
		"a continue skipping the settle":       {`for range xs { v := f(); if c { continue }; settle(v) }; return nil`, true},
		"coming round to the binding again":    {`for { v := f(); if c { settle(v); return nil } }`, true},
		"coming round inside an outer loop":    {`for { for c { v := f(); settle(v); break }; if d { return nil } }`, false},
		"leaving an inner loop unsettled":      {`for range xs { v := f(); for c { settle(v) } }; return nil`, true},
		"a switch case proving it non-nil":     {`v := f(); switch { case v != nil: settle(v) }; return nil`, false},
		"a switch case proving it nil":         {`v := f(); switch { case v == nil: return nil; default: settle(v) }; return nil`, false},
		"a case listing two proofs":            {`v := f(); switch { case v == nil, !c && v == nil: return nil }; settle(v); return nil`, false},
		"a case listing one proof of two":      {`v := f(); switch { case v == nil, c: return nil }; settle(v); return nil`, true},
		"a case whose failing proves":          {`v := f(); switch { case c, v != nil: settle(v) }; return nil`, false},
		"a switch case on something else":      {`v := f(); switch { case c: settle(v) }; return nil`, true},
		"a switch settling in every case":      {`v := f(); switch x { case 1: settle(v); default: settle(v) }; return nil`, false},
		"a break out of a switch":              {`v := f(); switch x { case 1: if c { break }; settle(v); default: settle(v) }; return nil`, true},
		"a type switch missing a case":         {`v := f(); switch v.(type) { case int: settle(v) }; return nil`, true},
		"a fallthrough is not followed":        {`v := f(); switch x { case 1: fallthrough; default: settle(v) }; return nil`, true},
		"a select with a default":              {`v := f(); select { case <-ch: settle(v); default: }; return nil`, true},
		"a select settling in every case":      {`v := f(); select { case <-ch: settle(v); case ch <- 1: settle(v) }; return nil`, false},
		"a goto is not followed":               {`v := f(); goto end; end: settle(v); return nil`, true},
		"a labelled break is not followed":     {"v := f(); outer: for { for { break outer } }; settle(v); return nil", true},
		"bound in an if's init and checked":    {`if v := f(); v != nil { settle(v) }; return nil`, false},
		"bound in an if's init, checked else":  {`if v := f(); c { settle(v) }; return nil`, true},
		"bound in a switch's init":             {`switch v := f(); { case v != nil: settle(v) }; return nil`, false},
		"bound in a type switch's init":        {`switch v := f(); v.(type) { default: settle(v) }; return nil`, false},
		"bound in a type switch's assign":      {`switch v := f().(type) { case error: settle(v) }; return nil`, true},
		"bound in a loop's init":               {`for v := f(); c; { settle(v); return nil }; return nil`, true},
		"bound in a select case":               {`select { case v := <-errs: settle(v) }; return nil`, false},
		"bound in a function literal":          {`go func() { v := f(); _ = v }(); return nil`, true},
		"returned from a function literal":     {`run := func() error { v := f(); return v }; return run()`, false},
		"a deferred settle":                    {`v := f(); defer settle(v); return nil`, false},
		"a parameter settled on every path":    {`if c { settle(v) } else { return v }; return nil`, false},
		"a parameter settled on one path only": {`if c { settle(v) }; return nil`, true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			body, start := plantedPathBody(t, tc.body)
			paths := Paths{
				Body: body,
				Step: plantedStep(start),
				Owed: func(cond ast.Expr, outcome bool) bool {
					return !ErrorSettled(cond, outcome, &types.Info{}, func(e ast.Expr) bool { return isIdent(e, "v") })
				},
			}
			if got := paths.Unsettled(start); got != tc.unsettled {
				t.Errorf("unsettled = %v; want %v", got, tc.unsettled)
			}
		})
	}
}

// A walk handed no Owed treats every condition as leaving the value owed.
func TestPathsWithoutOwedFollowEveryArm(t *testing.T) {
	t.Parallel()
	body, start := plantedPathBody(t, `v := f(); if v != nil { settle(v) }; return nil`)
	if !(Paths{Body: body, Step: plantedStep(start)}).Unsettled(start) {
		t.Error("the arm where v is nil was skipped with no Owed to say it could be")
	}
}

func plantedPathBody(t *testing.T, body string) (*ast.BlockStmt, ast.Stmt) {
	t.Helper()
	src := "package p\nfunc fn(v error, c, d bool, x int, xs []int, ch chan int, errs chan error) error {\n" + body + "\n}"
	file, err := parser.ParseFile(token.NewFileSet(), "planted.go", src, 0)
	if err != nil {
		t.Fatalf("parse the planted body: %v", err)
	}
	fn, ok := file.Decls[0].(*ast.FuncDecl)
	if !ok {
		t.Fatal("the planted file declares no function first")
	}
	var start ast.Stmt
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if assign, ok := n.(*ast.AssignStmt); ok && start == nil && isIdent(assign.Lhs[0], "v") {
			start = assign
		}
		return start == nil
	})
	return fn.Body, start
}

// plantedStep settles v at settle(v) and at a return reading it, and drops it
// at any assignment to v but start.
func plantedStep(start ast.Stmt) func(ast.Stmt) Step {
	return func(stmt ast.Stmt) Step {
		if assign, ok := stmt.(*ast.AssignStmt); ok && stmt != start && isIdent(assign.Lhs[0], "v") {
			return Drops
		}
		step := Passes
		ast.Inspect(stmt, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.CallExpr:
				if isIdent(n.Fun, "settle") {
					step = Settles
				}
			case *ast.ReturnStmt:
				for _, result := range n.Results {
					if isIdent(result, "v") {
						step = Settles
					}
				}
			}
			return true
		})
		return step
	}
}

func isIdent(e ast.Expr, name string) bool {
	ident, ok := ast.Unparen(e).(*ast.Ident)
	return ok && ident.Name == name
}

// A condition settles an error when it proves it nil or recognises its kind.
// Recognising it as any error at all recognises nothing, and a local value
// named errors is not the errors package.
func TestErrorSettledReadsNilAndRecognisedKinds(t *testing.T) {
	t.Parallel()
	const src = `package p
import ("errors"; "io"; "io/fs")
type fake struct{}
func (fake) Is(error, error) bool { return false }
var _ = []bool{
	err == nil,
	err != nil,
	errors.Is(err, io.EOF),
	!errors.Is(err, io.EOF),
	errors.As(err, &pathErr),
	errors.As(err, &anyErr),
	errors.As(err, &anything),
	errors.Is(other, io.EOF),
	local.Is(err, io.EOF),
}
var err, other, anyErr error
var pathErr *fs.PathError
var anything any
var local fake
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "p.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Uses: map[*ast.Ident]types.Object{}, Defs: map[*ast.Ident]types.Object{}}
	if _, err := (&types.Config{Importer: importer.ForCompiler(fset, "source", nil)}).Check("p", fset, []*ast.File{file}, info); err != nil {
		t.Fatalf("type-check: %v", err)
	}
	var conds []ast.Expr
	ast.Inspect(file, func(n ast.Node) bool {
		if lit, ok := n.(*ast.CompositeLit); ok {
			conds = lit.Elts
			return false
		}
		return true
	})
	// Each condition, settled when true and when false.
	want := [][2]bool{
		{true, false},
		{false, true},
		{true, false},
		{false, true},
		{true, false},
		{false, false},
		{false, false},
		{false, false},
		{false, false},
	}
	if len(conds) != len(want) {
		t.Fatalf("read %d conditions; want %d", len(conds), len(want))
	}
	subject := func(e ast.Expr) bool { return isIdent(e, "err") }
	for i, cond := range conds {
		got := [2]bool{ErrorSettled(cond, true, info, subject), ErrorSettled(cond, false, info, subject)}
		if got != want[i] {
			t.Errorf("%s settles (true, false) = %v; want %v", types.ExprString(cond), got, want[i])
		}
	}
}
