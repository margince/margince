// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

//go:build !integration

package gates

// A handler answers only the CHECK it names. An arm that answers every CHECK
// alike sits in front of httperr's constraint net. That net has its own answer
// for each of these: a too-long custom field, a retired member (409), a
// retention hold (423), an audit or outbox defect (500).
//
// The walk is syntactic: a name bound from storekit.CheckViolation must be
// compared, switched on or handed to a mapper. Only logging it is the old arm.

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

func TestNoHandlerAnswersEveryCheckViolationAlike(t *testing.T) {
	t.Parallel()
	files := gatekit.Scope{
		Roots:   []string{"internal"},
		Subject: func(path string, _ *ast.File) bool { return strings.HasPrefix(filepath.Base(path), "handlers") },
	}.Files(t)
	if len(files) == 0 {
		t.Fatal("no handler file found under internal/: the walk is broken")
	}
	for _, parsed := range files {
		for _, finding := range unnamedCheckArms(parsed.File) {
			t.Errorf("%s: %s\n\tAnswer only the constraints this module owns by name, and pass every "+
				"other error to httperr.Write, whose constraint net tells them apart.", parsed.Path, finding)
		}
	}
}

// unnamedCheckArms reports each CHECK binding in file whose name nothing reads
// except a log line.
func unnamedCheckArms(file *ast.File) []string {
	var findings []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			assign, ok := n.(*ast.AssignStmt)
			if !ok || len(assign.Rhs) != 1 || len(assign.Lhs) == 0 || !isCheckViolationCall(file, assign.Rhs[0]) {
				return true
			}
			name, _ := assign.Lhs[0].(*ast.Ident)
			if name == nil || name.Name == "_" {
				findings = append(findings, fn.Name.Name+" answers a CHECK without reading which one fired")
			} else if !constraintNameDecides(file, fn.Body, name.Name) {
				findings = append(findings, fn.Name.Name+" reads `"+name.Name+"` only to log it")
			}
			return true
		})
	}
	return findings
}

func isCheckViolationCall(file *ast.File, expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok || !storekitDoor(file, call.Fun) {
		return false
	}
	switch fun := call.Fun.(type) {
	case *ast.SelectorExpr:
		return fun.Sel.Name == "CheckViolation"
	case *ast.Ident:
		return fun.Name == "CheckViolation"
	}
	return false
}

// constraintNameDecides says name is compared, switched on, or passed to a
// call that is not a log line.
func constraintNameDecides(file *ast.File, body *ast.BlockStmt, name string) bool {
	slogName, _ := gatekit.ImportedAs(file, "log/slog")
	decides := false
	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.BinaryExpr:
			if (node.Op == token.EQL || node.Op == token.NEQ) && (identIs(node.X, name) || identIs(node.Y, name)) {
				decides = true
			}
		case *ast.SwitchStmt:
			if identIs(node.Tag, name) {
				decides = true
			}
		case *ast.IndexExpr:
			if identIs(node.Index, name) {
				decides = true
			}
		case *ast.CallExpr:
			if sel, ok := node.Fun.(*ast.SelectorExpr); ok && slogName != "" && identIs(sel.X, slogName) {
				return true
			}
			for _, arg := range node.Args {
				if identIs(arg, name) {
					decides = true
				}
			}
		}
		return true
	})
	return decides
}
