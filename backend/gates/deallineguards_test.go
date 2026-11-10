// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

import (
	"go/ast"
	"go/token"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

func rejectsCallError(file *ast.File, locals map[string]bool, branch *ast.IfStmt, previous ast.Stmt) (*ast.CallExpr, bool) {
	assignment, ok := branch.Init.(*ast.AssignStmt)
	if branch.Init == nil {
		assignment, ok = previous.(*ast.AssignStmt)
	}
	if !ok || len(assignment.Lhs) == 0 || len(assignment.Rhs) != 1 || branch.Else != nil {
		return nil, false
	}
	name, ok := assignment.Lhs[len(assignment.Lhs)-1].(*ast.Ident)
	if !ok {
		return nil, false
	}
	call, ok := assignment.Rhs[0].(*ast.CallExpr)
	if !ok || !nonNilName(branch.Cond, name.Name) || len(branch.Body.List) != 1 {
		return nil, false
	}
	returned, ok := branch.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(returned.Results) == 0 {
		return nil, false
	}
	last := returned.Results[len(returned.Results)-1]
	if err, ok := last.(*ast.Ident); ok {
		return call, err.Name == name.Name
	}
	if wrapped, ok := last.(*ast.CallExpr); ok {
		return call, unshadowedPackageCall(file, locals, wrapped.Fun, "fmt", "Errorf")
	}
	return nil, false
}

func nonNilName(expr ast.Expr, name string) bool {
	condition, ok := expr.(*ast.BinaryExpr)
	if !ok || condition.Op != token.NEQ {
		return false
	}
	left, lok := condition.X.(*ast.Ident)
	right, rok := condition.Y.(*ast.Ident)
	return lok && rok && ((left.Name == name && right.Name == "nil") || (right.Name == name && left.Name == "nil"))
}

func (f *lineLockFlow) patchGuarded(site linePatchSite, lineSeam string) bool {
	if len(site.call.Args) < 3 {
		return false
	}
	if site.name == lineSeam {
		return true
	}
	writer, ok := site.call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	return f.handledBefore(site.name, site.call.Pos(), func(call *ast.CallExpr) bool {
		called, ok := call.Fun.(*ast.Ident)
		if !ok || f.locals[site.name][called.Name] || called.Name != "refuseTypedMoneyOnLines" || len(call.Args) != 4 {
			return false
		}
		if !sameExpr(call.Args[3], writer.X) || !f.unchangedBetween(site.name, call.End(), site.call.Pos(), writer.X, site.call.Args[2]) {
			return false
		}
		if writer.Sel.Name != "ApplyLocked" {
			return len(site.call.Args) > 3 && strings.TrimSuffix(exprKey(call.Args[2]), ".UUID") == strings.TrimSuffix(exprKey(site.call.Args[3]), ".UUID")
		}
		sameID := false
		ast.Inspect(call.Args[2], func(node ast.Node) bool {
			id, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := id.Fun.(*ast.SelectorExpr)
			if ok && selector.Sel.Name == "ID" && sameExpr(selector.X, site.call.Args[2]) {
				sameID = true
			}
			return true
		})
		return sameID
	})
}

func (f *lineLockFlow) handledBefore(name string, pos token.Pos, matches func(*ast.CallExpr) bool) bool {
	found := false
	ast.Inspect(f.functions[name].Body, func(node ast.Node) bool {
		block, ok := node.(*ast.BlockStmt)
		if !ok || pos <= block.Pos() || pos >= block.End() {
			return true
		}
		for i, stmt := range block.List {
			branch, ok := stmt.(*ast.IfStmt)
			if !ok || branch.End() >= pos {
				continue
			}
			var previous ast.Stmt
			if i > 0 {
				previous = block.List[i-1]
			}
			call, handled := rejectsCallError(f.files[name], f.locals[name], branch, previous)
			if handled && matches(call) {
				found = true
			}
		}
		return true
	})
	return found
}

func exprKey(expr ast.Expr) string { return exprText(gatekit.SourceFileSet(), expr) }
func sameExpr(a, b ast.Expr) bool  { return exprKey(a) == exprKey(b) }

type lineModeGuard struct {
	end     token.Pos
	rejects bool
	deals   map[string]bool
}

func modeGuards(fn *ast.FuncDecl, constants map[string]string, file *ast.File) []lineModeGuard {
	heads := map[string]string{}
	var guards []lineModeGuard
	for _, statement := range fn.Body.List {
		if assign, ok := statement.(*ast.AssignStmt); ok && len(assign.Rhs) == 1 && len(assign.Lhs) > 0 {
			call, ok := assign.Rhs[0].(*ast.CallExpr)
			if ok {
				reader, ok := call.Fun.(*ast.Ident)
				head, named := assign.Lhs[0].(*ast.Ident)
				if ok && named && !declaredNames(fn)[reader.Name] && reader.Name == "readDealCommercialHead" && len(call.Args) > 2 {
					heads[head.Name] = exprKey(call.Args[2])
				}
			}
		}
		branch, ok := statement.(*ast.IfStmt)
		if !ok || len(lineExitDeals(branch.Cond, heads, constants)) == 0 || len(branch.Body.List) == 0 {
			continue
		}
		returned, ok := branch.Body.List[len(branch.Body.List)-1].(*ast.ReturnStmt)
		if !ok {
			continue
		}
		rejects := false
		if len(returned.Results) > 0 {
			address, ok := returned.Results[len(returned.Results)-1].(*ast.UnaryExpr)
			if ok && address.Op == token.AND {
				_, rejects = address.X.(*ast.CompositeLit)
			}
		}
		guards = append(guards, lineModeGuard{branch.End(), rejects && noEarlySuccess(fn, branch.Pos(), file), lineExitDeals(branch.Cond, heads, constants)})
	}
	return guards
}

func lineExitDeals(expr ast.Expr, heads map[string]string, constants map[string]string) map[string]bool {
	if paren, ok := expr.(*ast.ParenExpr); ok {
		return lineExitDeals(paren.X, heads, constants)
	}
	binary, ok := expr.(*ast.BinaryExpr)
	if !ok {
		return nil
	}
	if binary.Op == token.LOR {
		deals := map[string]bool{}
		for _, part := range []ast.Expr{binary.X, binary.Y} {
			for deal := range lineExitDeals(part, heads, constants) {
				deals[deal] = true
			}
		}
		return deals
	}
	if binary.Op != token.EQL {
		return nil
	}
	for _, pair := range [][2]ast.Expr{{binary.X, binary.Y}, {binary.Y, binary.X}} {
		field, ok := pair[0].(*ast.SelectorExpr)
		if !ok || field.Sel.Name != "Mode" {
			continue
		}
		head, ok := field.X.(*ast.Ident)
		mode, resolved := tableArgText(pair[1], constants)
		if ok && heads[head.Name] != "" && resolved && mode == constants["modeLines"] {
			return map[string]bool{heads[head.Name]: true}
		}
	}
	return nil
}

func (f *lineLockFlow) productsGuarded(name string, pos token.Pos, lineSeam string, visiting map[string]bool, expected ...string) bool {
	wanted := f.dealAt(name, pos)
	if len(expected) > 0 {
		wanted = expected[0]
	}
	if wanted == "" {
		return false
	}
	if visiting[name] {
		return false
	}
	visiting[name] = true
	defer delete(visiting, name)
	for _, guard := range modeGuards(f.functions[name], f.constants, f.files[name]) {
		if guard.end < pos && guard.deals[wanted] && f.unchangedBetween(name, guard.end, pos, ast.NewIdent(wanted)) {
			return true
		}
	}
	if f.handledBefore(name, pos, func(call *ast.CallExpr) bool {
		callee := f.called(name, call)
		if !f.unchangedBetween(name, call.End(), pos, ast.NewIdent(wanted)) {
			return false
		}
		if callee == lineSeam {
			return len(call.Args) > 3 && strings.HasSuffix(wanted, ".DealID") && exprKey(call.Args[3]) == strings.TrimSuffix(wanted, ".DealID")+".Lock"
		}
		if callee == "" {
			return false
		}
		for _, guard := range modeGuards(f.functions[callee], f.constants, f.files[callee]) {
			if guard.rejects && guard.deals[f.dealParameter(callee, call, wanted)] {
				return true
			}
		}
		return false
	}) {
		return true
	}
	if f.external[name] {
		return false
	}
	callers := 0
	safe := true
	for caller, fn := range f.functions {
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if ok && f.called(caller, call) == name {
				callers++
				if !f.productsGuarded(caller, call.Pos(), lineSeam, visiting, f.dealArgument(name, call, wanted)) {
					safe = false
				}
			}
			return true
		})
	}
	return safe && callers > 0
}

func noEarlySuccess(fn *ast.FuncDecl, before token.Pos, file *ast.File) bool {
	for _, statement := range fn.Body.List {
		if statement.Pos() >= before {
			break
		}
		hasReturn := false
		ast.Inspect(statement, func(node ast.Node) bool {
			if _, ok := node.(*ast.ReturnStmt); ok {
				hasReturn = true
			}
			return true
		})
		if !hasReturn {
			continue
		}
		branch, ok := statement.(*ast.IfStmt)
		if !ok || len(branch.Body.List) != 1 {
			return false
		}
		returned, ok := branch.Body.List[0].(*ast.ReturnStmt)
		if !ok || len(returned.Results) == 0 {
			return false
		}
		switch err := returned.Results[len(returned.Results)-1].(type) {
		case *ast.Ident:
			if !nonNilName(branch.Cond, err.Name) {
				return false
			}
		case *ast.CallExpr:
			if !unshadowedPackageCall(file, declaredNames(fn), err.Fun, "fmt", "Errorf") {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func (f *lineLockFlow) unchangedBetween(name string, start, end token.Pos, expressions ...ast.Expr) bool {
	changed := false
	tracked := func(expression ast.Expr) bool {
		text := exprKey(expression)
		for _, target := range expressions {
			key := exprKey(target)
			if text == key || strings.HasPrefix(text, key+".") || strings.HasPrefix(key, text+".") {
				return true
			}
		}
		return false
	}
	ast.Inspect(f.functions[name].Body, func(node ast.Node) bool {
		if node == nil || node.Pos() >= end || node.End() <= start {
			return false
		}
		if node.Pos() <= start {
			return true
		}
		switch value := node.(type) {
		case *ast.FuncLit:
			changed = value.Pos() > start && value.Pos() < end && end < value.End()
		case *ast.AssignStmt:
			for _, target := range value.Lhs {
				changed = changed || tracked(target)
			}
		case *ast.ValueSpec:
			for _, target := range value.Names {
				changed = changed || tracked(target)
			}
		case *ast.CallExpr:
			if method, ok := value.Fun.(*ast.SelectorExpr); ok {
				changed = changed || tracked(method.X)
			}
		}
		return !changed
	})
	return !changed
}

func (f *lineLockFlow) dealAt(name string, pos token.Pos) string {
	var deal string
	ast.Inspect(f.functions[name].Body, func(node ast.Node) bool {
		if call, ok := node.(*ast.CallExpr); ok && call.Pos() <= pos && pos < call.End() && len(call.Args) > 2 {
			deal = exprKey(call.Args[2])
			if call.Ellipsis != token.NoPos {
				deal = f.insertDealArgument(name, call.Args[2], call.Pos())
			}
		}
		return true
	})
	return deal
}

func (f *lineLockFlow) dealParameter(name string, call *ast.CallExpr, wanted string) string {
	i := 0
	for _, field := range f.functions[name].Type.Params.List {
		for _, param := range field.Names {
			if i < len(call.Args) && exprKey(call.Args[i]) == wanted {
				return param.Name
			}
			i++
		}
	}
	return wanted
}

func (f *lineLockFlow) dealArgument(name string, call *ast.CallExpr, wanted string) string {
	i := 0
	for _, field := range f.functions[name].Type.Params.List {
		for _, param := range field.Names {
			if i < len(call.Args) {
				if wanted == param.Name {
					return exprKey(call.Args[i])
				}
				if suffix, ok := strings.CutPrefix(wanted, param.Name+"."); ok {
					return exprKey(call.Args[i]) + "." + suffix
				}
			}
			i++
		}
	}
	return wanted
}

func (f *lineLockFlow) insertDealArgument(name string, argument ast.Expr, before token.Pos) string {
	var deal string
	ast.Inspect(f.functions[name].Body, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok || assignment.End() >= before || len(assignment.Rhs) != 1 || len(assignment.Lhs) == 0 || !sameExpr(assignment.Lhs[len(assignment.Lhs)-1], argument) {
			return true
		}
		call, ok := assignment.Rhs[0].(*ast.CallExpr)
		if !ok || f.called(name, call) != "columnValues" {
			return true
		}
		for _, arg := range call.Args {
			pair, ok := arg.(*ast.CompositeLit)
			if !ok || len(pair.Elts) != 2 {
				continue
			}
			column, ok := tableArgText(pair.Elts[0], f.constants)
			if ok && column == f.constants["dealIDColumn"] && column != "" {
				deal = exprKey(pair.Elts[1])
			}
		}
		return true
	})
	return deal
}

func (f *lineLockFlow) canonicalDealPatch(site linePatchSite) bool {
	return slices.Contains(linePatchSeams, site.name) && f.patchGuarded(site, lineValueSeam)
}
