// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// wireBuilders turn a row into the contract shape for a preview build, which
// reads as the effect's principal and never reaches a reader.
var wireBuilders = map[string]bool{"editedProposal": true}

// Every function that hands out an approval in the contract shape also asks
// readPlans, directly or through the readers that wrap it. Otherwise the plan
// leaves the module past the sight and mask checks.
func TestEveryApprovalInTheContractShapeIsReadThroughItsPlanCheck(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	callers := 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !callsAny(fn.Body, "wire") {
				continue
			}
			callers++
			if !wireBuilders[fn.Name.Name] && !callsAny(fn.Body, "readPlans", "readDecisionContext", "readCommittedContext") {
				t.Errorf("%s: %s builds an approval in the contract shape without reading its plan check", fset.Position(fn.Pos()), fn.Name.Name)
			}
		}
	}
	if callers == 0 {
		t.Fatal("no function builds the contract shape, so this census read nothing")
	}
}

// callsAny answers whether body calls a function or method of one of names.
func callsAny(body *ast.BlockStmt, names ...string) bool {
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return !found
		}
		switch fun := call.Fun.(type) {
		case *ast.Ident:
			found = found || slices.Contains(names, fun.Name)
		case *ast.SelectorExpr:
			found = found || slices.Contains(names, fun.Sel.Name)
		}
		return !found
	})
	return found
}
