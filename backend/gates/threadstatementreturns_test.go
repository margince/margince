// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

//gate:kind census H2

package gates

import (
	"go/ast"
	"go/token"
	"strconv"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const threadSQLBefore = "$threadSQLBefore"

func threadNilErrorCondition(expr ast.Expr, name string) bool {
	condition, ok := expr.(*ast.BinaryExpr)
	if !ok || condition.Op != token.NEQ {
		return false
	}
	left, leftOK := condition.X.(*ast.Ident)
	right, rightOK := condition.Y.(*ast.Ident)
	return leftOK && rightOK && ((left.Name == name && right.Name == "nil") || (right.Name == name && left.Name == "nil"))
}

func threadFailureReturn(fn *ast.FuncDecl, result *ast.ReturnStmt) bool {
	if len(result.Results) < 2 {
		return false
	}
	text, literal := gatekit.LiteralText(result.Results[0])
	if !literal || text != "" {
		return false
	}
	failure, ok := result.Results[len(result.Results)-1].(*ast.Ident)
	if !ok || failure.Name == "nil" {
		return false
	}
	proven := false
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		branch, ok := node.(*ast.IfStmt)
		if ok && result.Pos() > branch.Body.Pos() && result.End() < branch.Body.End() && threadNilErrorCondition(branch.Cond, failure.Name) {
			proven = true
		}
		return !proven
	})
	return proven
}

func threadCallerChecksError(call *ast.CallExpr, fn *ast.FuncDecl, bindings map[string][]ast.Expr) bool {
	cutoff := fn.End()
	if limit := bindings[threadSQLBefore]; len(limit) == 1 {
		if value, ok := limit[0].(*ast.BasicLit); ok {
			if parsed, err := strconv.Atoi(value.Value); err == nil {
				cutoff = token.Pos(parsed)
			}
		}
	}
	failure := ""
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, right := range assignment.Rhs {
			if right == call && len(assignment.Lhs) > 1 {
				if name, ok := assignment.Lhs[len(assignment.Lhs)-1].(*ast.Ident); ok {
					failure = name.Name
				}
			}
		}
		return true
	})
	if failure == "" || failure == "_" {
		return false
	}
	checked := false
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		branch, ok := node.(*ast.IfStmt)
		if !ok || branch.Pos() < call.End() || branch.End() >= cutoff || !threadNilErrorCondition(branch.Cond, failure) {
			return true
		}
		for _, statement := range branch.Body.List {
			if _, returns := statement.(*ast.ReturnStmt); returns {
				checked = true
			}
		}
		return !checked
	})
	return checked
}

func threadSQLStringResult(fn *ast.FuncDecl) bool {
	if fn.Type.Results == nil || len(fn.Type.Results.List) == 0 {
		return false
	}
	name, ok := fn.Type.Results.List[0].Type.(*ast.Ident)
	return ok && name.Name == "string"
}
