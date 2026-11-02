// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

import "go/ast"

func promptReceiverType(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}
	return promptTypeName(fn.Recv.List[0].Type)
}

func promptTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return promptTypeName(t.X)
	case *ast.IndexExpr:
		return promptTypeName(t.X)
	case *ast.IndexListExpr:
		return promptTypeName(t.X)
	case *ast.SelectorExpr:
		return t.Sel.Name
	}
	return ""
}
