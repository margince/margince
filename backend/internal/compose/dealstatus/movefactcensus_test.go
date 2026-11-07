// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

func moveProductionSources(t *testing.T) []*ast.File {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var sources []*ast.File
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		source, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		sources = append(sources, source)
	}
	return sources
}

func TestEveryFactTheMoveReadsIsRepresentedByItsBasis(t *testing.T) {
	sources := moveProductionSources(t)
	chosen := moveFactFields(sources, "decideMove")
	for field := range moveFactFields(sources, "assessMove") {
		chosen[field] = true
	}
	// Private capture observations qualify wording without changing acceptance or dismissals.
	if !chosen["mailCoverage"] {
		t.Fatal("the private-overlay exception no longer matches a fact")
	}
	delete(chosen, "mailCoverage")
	fingerprinted := moveFactFields(sources, "moveBasisFingerprint")
	if len(chosen) == 0 || len(fingerprinted) == 0 {
		t.Fatal("the census did not reach the production choice and basis")
	}
	for field := range chosen {
		if !fingerprinted[field] {
			t.Errorf("the recommendation reads facts.%s without representing it in its basis", field)
		}
	}
}

func moveFactFields(sources []*ast.File, root string) map[string]bool {
	functions := map[string][]*ast.FuncDecl{}
	for _, source := range sources {
		for _, decl := range source.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok {
				functions[fn.Name.Name] = append(functions[fn.Name.Name], fn)
			}
		}
	}
	fields, visited := map[string]bool{}, map[string]bool{}
	wanted := []string{root}
	for len(wanted) > 0 {
		name := wanted[0]
		wanted = wanted[1:]
		if visited[name] {
			continue
		}
		visited[name] = true
		for _, fn := range functions[name] {
			parameters := factParameters(fn)
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				if selector, ok := node.(*ast.SelectorExpr); ok {
					if source, ok := selector.X.(*ast.Ident); ok && parameters[source.Name] {
						fields[selector.Sel.Name] = true
					}
				}
				if call, ok := node.(*ast.CallExpr); ok {
					switch callee := call.Fun.(type) {
					case *ast.Ident:
						wanted = append(wanted, callee.Name)
					case *ast.SelectorExpr:
						wanted = append(wanted, callee.Sel.Name)
					}
				}
				return true
			})
		}
	}
	return fields
}

func factParameters(fn *ast.FuncDecl) map[string]bool {
	out := map[string]bool{}
	addParameters := func(parameters *ast.FieldList) {
		if parameters == nil {
			return
		}
		for _, param := range parameters.List {
			kind := param.Type
			if pointer, ok := kind.(*ast.StarExpr); ok {
				kind = pointer.X
			}
			if name, ok := kind.(*ast.Ident); ok && name.Name == "facts" {
				for _, name := range param.Names {
					out[name.Name] = true
				}
			}
		}
	}
	addParameters(fn.Type.Params)
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if closure, ok := node.(*ast.FuncLit); ok {
			addParameters(closure.Type.Params)
		}
		return true
	})
	for {
		before := len(out)
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			assignment, ok := node.(*ast.AssignStmt)
			if !ok || len(assignment.Lhs) != len(assignment.Rhs) {
				return true
			}
			for i, expr := range assignment.Rhs {
				if from, ok := expr.(*ast.Ident); ok && out[from.Name] {
					if to, ok := assignment.Lhs[i].(*ast.Ident); ok {
						out[to.Name] = true
					}
				}
			}
			return true
		})
		if len(out) == before {
			return out
		}
	}
}

func TestTheFactCensusFollowsAnAliasAndAClosure(t *testing.T) {
	source, err := parser.ParseFile(token.NewFileSet(), "fixture.go", `package dealstatus
 func choose(f facts) {
  another := f
  _ = another.requests
  visit(func(nested facts) { _ = nested.nextMeeting })
 }
 `, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := moveFactFields([]*ast.File{source}, "choose")
	if !found["requests"] || !found["nextMeeting"] {
		t.Fatalf("the census lost a fact behind an alias or closure: %v", found)
	}
}
