// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

type taskValueReader struct {
	consts   packageConsts
	parsed   gatekit.ParsedFile
	bindings map[string][]ast.Expr
}

func taskValues(consts packageConsts, parsed gatekit.ParsedFile, fn *ast.FuncDecl) taskValueReader {
	values := taskValueReader{consts: consts, parsed: parsed, bindings: map[string][]ast.Expr{}}
	for _, binding := range stringBindings(&ast.File{Decls: []ast.Decl{fn}}) {
		values.bindings[binding.name] = append(values.bindings[binding.name], binding.expr)
	}
	return values
}

// Pointer syntax and local bindings feed the existing shared constant reader.
func (r taskValueReader) read(expr ast.Expr, active map[string]bool) (string, bool) {
	switch node := expr.(type) {
	case *ast.UnaryExpr:
		if node.Op == token.AND {
			return r.read(node.X, active)
		}
	case *ast.ParenExpr:
		return r.read(node.X, active)
	case *ast.CallExpr:
		if name, ok := node.Fun.(*ast.Ident); ok && (name.Name == "new" || name.Name == "string") && len(node.Args) == 1 {
			return r.read(node.Args[0], active)
		}
	case *ast.Ident:
		if bindings := r.bindings[node.Name]; len(bindings) > 0 {
			return r.sameValue(node.Name, bindings, active)
		}
		if node.Obj != nil {
			if decl, ok := node.Obj.Decl.(*ast.ValueSpec); ok {
				for i, name := range decl.Names {
					if name.Name == node.Name && i < len(decl.Values) {
						return r.sameValue(node.Name, []ast.Expr{decl.Values[i]}, active)
					}
				}
			}
			return "", false
		}
	case *ast.BinaryExpr:
		left, leftOK := r.read(node.X, active)
		right, rightOK := r.read(node.Y, active)
		if leftOK && rightOK {
			return gatekit.StringExpr(&ast.BinaryExpr{
				X: &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(left)}, Op: node.Op,
				Y: &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(right)},
			}, nil, gatekit.FoldStrict)
		}
	}
	return foldConstant(r.consts, r.parsed, expr)
}

func (r taskValueReader) sameValue(name string, bindings []ast.Expr, active map[string]bool) (string, bool) {
	if active[name] {
		return "", false
	}
	active[name] = true
	defer delete(active, name)
	var value string
	for i, expr := range bindings {
		resolved, known := r.read(expr, active)
		if !known || (i > 0 && resolved != value) {
			return "", false
		}
		value = resolved
	}
	return value, true
}

func (r taskValueReader) provesTaskStamp(body *ast.BlockStmt) bool {
	found, valid := false, true
	check := func(expr ast.Expr) {
		found = true
		value, known := r.read(expr, map[string]bool{})
		valid = valid && known && strings.TrimSpace(value) != ""
	}
	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CompositeLit:
			if lastName(node.Type) == "LogActivityInput" {
				for _, elt := range node.Elts {
					if field, ok := elt.(*ast.KeyValueExpr); ok && lastName(field.Key) == "SourceSystem" {
						check(field.Value)
					}
				}
			}
		case *ast.AssignStmt:
			for i, lhs := range node.Lhs {
				if field, ok := lhs.(*ast.SelectorExpr); ok && field.Sel.Name == "SourceSystem" && i < len(node.Rhs) {
					check(node.Rhs[i])
				}
			}
		}
		return true
	})
	return found && valid
}
