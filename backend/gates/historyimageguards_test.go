// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"slices"
)

func historyGuardedKeys(in historyImageArg, write *ast.AssignStmt, key ast.Expr, keys []string) []string {
	name, ok := ast.Unparen(key).(*ast.Ident)
	if !ok || in.decl == nil {
		return keys
	}
	object := in.pkg.info.ObjectOf(name)
	if object == nil || historyKeyMayChange(in, object, write) {
		return keys
	}
	kept := slices.Clone(keys)
	ast.Inspect(in.decl.Body, func(node ast.Node) bool {
		branch, ok := node.(*ast.IfStmt)
		if !ok || write.Pos() <= branch.Body.Pos() || write.End() >= branch.Body.End() {
			return true
		}
		condition, ok := ast.Unparen(branch.Cond).(*ast.BinaryExpr)
		if !ok || (condition.Op != token.EQL && condition.Op != token.NEQ) {
			return true
		}
		var compared ast.Expr
		if left, ok := ast.Unparen(condition.X).(*ast.Ident); ok && in.pkg.info.ObjectOf(left) == object {
			compared = condition.Y
		} else if right, ok := ast.Unparen(condition.Y).(*ast.Ident); ok && in.pkg.info.ObjectOf(right) == object {
			compared = condition.X
		}
		if compared == nil {
			return true
		}
		value := in.pkg.info.Types[compared].Value
		if value == nil || value.Kind() != constant.String {
			return true
		}
		match := constant.StringVal(value)
		kept = slices.DeleteFunc(kept, func(field string) bool {
			if condition.Op == token.EQL {
				return field != match
			}
			return field == match
		})
		return true
	})
	return kept
}

func historyKeyMayChange(in historyImageArg, key types.Object, write *ast.AssignStmt) bool {
	unstable := false
	same := func(expression ast.Expr) bool {
		name, ok := ast.Unparen(expression).(*ast.Ident)
		return ok && in.pkg.info.ObjectOf(name) == key
	}
	ast.Inspect(in.decl.Body, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.AssignStmt:
			for _, target := range value.Lhs {
				unstable = unstable || same(target)
			}
		case *ast.UnaryExpr:
			unstable = unstable || value.Op == token.AND && same(value.X)
		case *ast.RangeStmt:
			unstable = unstable || value.Tok == token.ASSIGN && value.Key != nil && same(value.Key)
		case *ast.FuncLit:
			unstable = unstable || value.Body.Pos() < write.Pos() && write.End() < value.Body.End()
		}
		return !unstable
	})
	return unstable
}
