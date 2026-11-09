// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

import (
	"go/ast"
	"go/token"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

type threadGuardCall struct {
	caller, called string
	pos            token.Pos
}
type threadGuardOrder struct {
	functions map[string]*ast.FuncDecl
	calls     map[string][]threadGuardCall
	parents   map[string][]threadGuardCall
	guards    map[string]token.Pos
}

func threadOrderOf(files []*ast.File) *threadGuardOrder {
	order := &threadGuardOrder{functions: map[string]*ast.FuncDecl{}, calls: map[string][]threadGuardCall{}, parents: map[string][]threadGuardCall{}, guards: map[string]token.Pos{}}
	for _, file := range files {
		aliases := map[string]bool{}
		for _, imp := range file.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			if err != nil || path != "github.com/margince/margince/backend/internal/platform/database/storekit" {
				continue
			}
			name := "storekit"
			if imp.Name != nil {
				name = imp.Name.Name
			}
			aliases[name] = true
		}
		for _, declaration := range file.Decls {
			fn, ok := declaration.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			name := scrubKey(receiverTypeName(fn), fn.Name.Name)
			order.functions[name] = fn
			order.readCalls(name, fn, aliases)
		}
	}
	for changed := true; changed; {
		changed = false
		for name, calls := range order.calls {
			for _, call := range calls {
				if order.guards[call.called] != 0 && !order.callsBack(call.called, name, map[string]bool{}) && (order.guards[name] == 0 || call.pos < order.guards[name]) {
					order.guards[name], changed = call.pos, true
				}
			}
		}
	}
	return order
}

func (o *threadGuardOrder) readCalls(name string, fn *ast.FuncDecl, aliases map[string]bool) {
	invoked := invokedLiterals(fn)
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if lit, ok := n.(*ast.FuncLit); ok && !invoked[lit] {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		called := ""
		switch fun := call.Fun.(type) {
		case *ast.Ident:
			called = fun.Name
		case *ast.SelectorExpr:
			base, ok := fun.X.(*ast.Ident)
			if !ok {
				return true
			}
			if aliases[base.Name] && slices.Contains([]string{"WaitThreadKeysTx", "LockThreadKeysTx", "LockActivityThreadTx"}, fun.Sel.Name) {
				if o.guards[name] == 0 || call.Pos() < o.guards[name] {
					o.guards[name] = call.Pos()
				}
			}
			if base.Name == receiverVarName(fn) {
				called = scrubKey(receiverTypeName(fn), fun.Sel.Name)
			}
		}
		if called != "" {
			edge := threadGuardCall{name, called, call.Pos()}
			o.calls[name] = append(o.calls[name], edge)
			o.parents[called] = append(o.parents[called], edge)
		}
		return true
	})
}

func (o *threadGuardOrder) guardedBefore(name string, pos token.Pos, visiting map[string]bool) bool {
	if guard := o.guards[name]; guard != 0 && guard < pos {
		return true
	}
	if fn := o.functions[name]; fn != nil && ast.IsExported(fn.Name.Name) {
		return false
	}
	if visiting[name] || len(o.parents[name]) == 0 {
		return false
	}
	visiting[name] = true
	defer delete(visiting, name)
	for _, parent := range o.parents[name] {
		if !o.guardedBefore(parent.caller, parent.pos, visiting) {
			return false
		}
	}
	return true
}

func (o *threadGuardOrder) writePosition(t *testing.T, name, statement string, target sqlTarget, writes []tableWrite) token.Pos {
	t.Helper()
	fn := o.functions[name]
	if fn == nil {
		t.Fatalf("thread writer %s has no declaration", name)
	}
	for _, write := range writes {
		_, site, found := strings.Cut(write.site, ":")
		if !found || site != name || write.table != target.table || write.verb != target.verb || write.stmt != gatekit.FirstLineOf(statement) {
			continue
		}
		location := strings.Split(write.pos, ":")
		line, lineErr := strconv.Atoi(location[len(location)-2])
		column, columnErr := strconv.Atoi(location[len(location)-1])
		if lineErr != nil || columnErr != nil {
			t.Fatalf("invalid writer position %s", write.pos)
		}
		return gatekit.SourceFileSet().File(fn.Pos()).LineStart(line) + token.Pos(column-1)
	}
	t.Fatalf("thread writer %s.%s has no ownership-census position", name, target.table)
	return token.NoPos
}

func (o *threadGuardOrder) callsBack(from, target string, visited map[string]bool) bool {
	if from == target {
		return true
	}
	if visited[from] {
		return false
	}
	visited[from] = true
	for _, call := range o.calls[from] {
		if o.callsBack(call.called, target, visited) {
			return true
		}
	}
	return false
}

// A lock in an unused closure says nothing about the writer that contains it.
func invokedLiterals(fn *ast.FuncDecl) map[*ast.FuncLit]bool {
	invoked := map[*ast.FuncLit]bool{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if lit, isLit := call.Fun.(*ast.FuncLit); isLit {
			invoked[lit] = true
		}
		for _, arg := range call.Args {
			if lit, isLit := arg.(*ast.FuncLit); isLit {
				invoked[lit] = true
			}
		}
		return true
	})
	return invoked
}
