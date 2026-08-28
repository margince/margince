// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// Reading one VALUE out of a unit's declaration: a published constant, a string,
// a position for an error to point at.
//
// Split from astreader.go, which walks the declaration's SHAPE. The two are read
// at different times — the shape when a capability is added, these when a
// declaration will not derive and the author needs to know which spelling this
// generator can compute.
//
// Everything here computes rather than evaluates. The manifest is derived from
// SOURCE, without compiling the unit, so a value that depends on running code is
// a value the manifest could not state.

import (
	"fmt"
	"go/ast"
	"go/token"
	"strconv"
	"strings"
)

// constValue resolves a published constant (extension.X) through the
// source-derived vocabulary, or accepts a plain string literal.
func (r *unitReader) constValue(expr ast.Expr, ext string) (string, error) {
	switch v := expr.(type) {
	case *ast.SelectorExpr:
		pkg, ok := v.X.(*ast.Ident)
		if !ok || pkg.Name != ext {
			return "", r.errAt(expr, "constants must come from the published extension package")
		}
		value, ok := r.vocab[v.Sel.Name]
		if !ok {
			return "", r.errAt(expr, "%s.%s is not a published extension constant", pkg.Name, v.Sel.Name)
		}
		return value, nil
	case *ast.BasicLit:
		if v.Kind != token.STRING {
			return "", r.errAt(expr, "expected a string literal or a published extension constant")
		}
		return strconv.Unquote(v.Value)
	}
	return "", r.errAt(expr, "expected a string literal or a published extension constant")
}

// errAt names the position and restates the rule: the fix is to make the
// declaration a literal, so a SHAPE error (a computed value, a non-literal
// field) carries that prescription.
func (r *unitReader) errAt(n ast.Node, format string, args ...any) error {
	return fmt.Errorf("%s: %s — manifest derivation reads declarations statically; declare literal values",
		r.fset.Position(n.Pos()), fmt.Sprintf(format, args...))
}

// errPos names the position only, for a SEMANTIC error (a literal that is
// present but invalid — a bad version, an out-of-vocabulary scope) whose
// fix is not "make it a literal".
func (r *unitReader) errPos(n ast.Node, format string, args ...any) error {
	return fmt.Errorf("%s: %s", r.fset.Position(n.Pos()), fmt.Sprintf(format, args...))
}

// singleReturn enforces the declaration-constructor shape: exactly one
// statement, a return of exactly one expression.
func (r *unitReader) singleReturn(fn *ast.FuncDecl) (ast.Expr, error) {
	if fn.Body == nil || len(fn.Body.List) != 1 {
		return nil, r.errAt(fn, "%s must hold exactly one return statement", fn.Name.Name)
	}
	ret, ok := fn.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return nil, r.errAt(fn, "%s must hold exactly one return statement", fn.Name.Name)
	}
	return ret.Results[0], nil
}

func (r *unitReader) stringLit(expr ast.Expr, field string) (string, error) {
	// A concatenation of literals is still a literal: the value is fixed at the
	// declaration and this reader can compute it without evaluating anything.
	// Prose that will not fit on one line — a tool's description — has no other
	// way to be written, and refusing it would push a unit author into a single
	// unreadable line to satisfy a generator.
	if bin, ok := expr.(*ast.BinaryExpr); ok && bin.Op == token.ADD {
		left, err := r.stringLit(bin.X, field)
		if err != nil {
			return "", err
		}
		right, err := r.stringLit(bin.Y, field)
		if err != nil {
			return "", err
		}
		return left + right, nil
	}
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", r.errAt(expr, "%s must be a string literal (or literals joined by +)", field)
	}
	return strconv.Unquote(lit.Value)
}

// importAlias resolves the file-local name of an imported package path.
func importAlias(file *ast.File, path string) string {
	for _, imp := range file.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil || p != path {
			continue
		}
		if imp.Name != nil {
			return imp.Name.Name
		}
		return p[strings.LastIndex(p, "/")+1:]
	}
	return ""
}

func isSelector(expr ast.Expr, pkg, name string) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	return ok && pkg != "" && ident.Name == pkg && sel.Sel.Name == name
}
