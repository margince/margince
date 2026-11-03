// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"go/ast"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

func TestContactsCountSortAndCompanyParameterReachTheRosterOwner(t *testing.T) {
	functions := map[string]*ast.FuncDecl{}
	subjects := map[string]*ast.FuncDecl{}
	counts, filters, sorts := 0, 0, 0
	for _, file := range moduleFiles(t) {
		for _, decl := range file.file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			if fn.Recv == nil {
				functions[fn.Name.Name] = fn
			}
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				if text, ok := node.(*ast.BasicLit); ok {
					literal, isText := gatekit.LiteralText(text)
					if isText && strings.Contains(strings.ToLower(literal), "contact_count") {
						subjects[fn.Name.Name] = fn
					}
				}
				if assignment, ok := node.(*ast.AssignStmt); ok {
					for _, left := range assignment.Lhs {
						if field, ok := left.(*ast.SelectorExpr); ok && field.Sel.Name == "ContactCount" {
							subjects[fn.Name.Name] = fn
							counts++
						}
					}
				}
				if call, ok := node.(*ast.CallExpr); ok && takesContactList(fn) {
					for _, arg := range call.Args {
						if field, ok := arg.(*ast.SelectorExpr); ok && field.Sel.Name == "CompanyID" {
							subjects[fn.Name.Name] = fn
							filters++
						}
					}
				}
				return true
			})
		}
		ast.Inspect(file.file, func(node ast.Node) bool {
			pair, ok := node.(*ast.KeyValueExpr)
			if !ok {
				return true
			}
			key, ok := gatekit.LiteralText(pair.Key)
			if !ok || key != "contact_count" {
				return true
			}
			literal, ok := pair.Value.(*ast.CompositeLit)
			if !ok {
				return true
			}
			for _, element := range literal.Elts {
				field, ok := element.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				name, ok := field.Key.(*ast.Ident)
				if !ok || name.Name != "Expr" {
					continue
				}
				expression, ok := field.Value.(*ast.Ident)
				if !ok {
					t.Error("contact count sort expression is not a named roster reader")
					continue
				}
				subjects[expression.Name] = nil
				sorts++
			}
			return true
		})
	}
	if counts == 0 || filters == 0 || sorts == 0 {
		t.Fatalf("short roster census: counts=%d filters=%d sorts=%d", counts, filters, sorts)
	}
	for subject, fn := range subjects {
		if fn == nil {
			fn = functions[subject]
		}
		if !reachesRosterOwner(fn, functions, map[string]bool{}) {
			t.Errorf("%s derives company membership without the roster owner", subject)
		}
	}
}

func takesContactList(fn *ast.FuncDecl) bool {
	for _, parameter := range fn.Type.Params.List {
		if name, ok := parameter.Type.(*ast.Ident); ok && name.Name == "ListContactsInput" {
			return true
		}
	}
	return false
}

func reachesRosterOwner(fn *ast.FuncDecl, functions map[string]*ast.FuncDecl, seen map[string]bool) bool {
	if fn == nil {
		return false
	}
	name := fn.Name.Name
	if name == "countedEmploymentFrom" {
		return true
	}
	if seen[name] {
		return false
	}
	seen[name] = true
	found := false
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		target, ok := call.Fun.(*ast.Ident)
		if ok && reachesRosterOwner(functions[target.Name], functions, seen) {
			found = true
		}
		return true
	})
	return found
}
