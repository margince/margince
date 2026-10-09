// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/agentbundle"
)

// A handler that calls auth.RequireHuman, itself or one call down, refuses every
// passport, so the skill must not list it. The router calls the operationId with its first letter raised.
func TestTheSkillListsNoOperationWhoseHandlerRequiresAHuman(t *testing.T) {
	archive, err := (&agentbundle.Builder{}).Build("https://crm.example.test/v1")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	listed := map[string]string{}
	for id := range operationIDsIn(t, bundledSpec(t, archive)) {
		listed[strings.ToUpper(id[:1])+id[1:]] = id
	}
	methods, err := methodsByDir("..")
	if err != nil {
		t.Fatalf("scanning internal/: %v", err)
	}

	found := map[string]bool{}
	for dir, byName := range methods {
		for name, decls := range byName {
			if _, isListed := listed[name]; !isListed {
				continue
			}
			for _, handler := range decls {
				if !isHTTPHandler(handler) {
					continue
				}
				found[name] = true
				if why := requiresHuman(handler, byName); why != "" {
					t.Errorf("the skill lists %s, and in %s %s calls auth.RequireHuman, so a passport is always refused",
						listed[name], dir, why)
				}
			}
		}
	}
	if len(found) != len(listed) {
		missing := map[string]bool{}
		for name, id := range listed {
			if !found[name] {
				missing[id] = true
			}
		}
		t.Fatalf("found a handler for %d of %d listed operations, none for %v; the scan is short, so a pass would prove nothing",
			len(found), len(listed), sortedIDs(missing))
	}
}

// methodsByDir indexes every hand-written method under root by its directory
// and name. The generated router and its stubs carry every operation's name,
// so reading them would find a handler where none is written.
func methodsByDir(root string) (map[string]map[string][]*ast.FuncDecl, error) {
	methods := map[string]map[string][]*ast.FuncDecl{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() && path == filepath.Join(root, "contracts") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "_gen.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		dir := filepath.Dir(path)
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Body == nil {
				continue
			}
			if methods[dir] == nil {
				methods[dir] = map[string][]*ast.FuncDecl{}
			}
			methods[dir][fn.Name.Name] = append(methods[dir][fn.Name.Name], fn)
		}
		return nil
	})
	return methods, err
}

// requiresHuman names the handler, or the method of its package it calls, that calls auth.RequireHuman.
// A parse has no types, so a callee matches any same-named method of the package, and other packages go unread.
func requiresHuman(handler *ast.FuncDecl, pkgMethods map[string][]*ast.FuncDecl) string {
	if callsRequireHuman(handler.Body) {
		return handler.Name.Name
	}
	for _, callee := range calledMethodNames(handler.Body) {
		for _, decl := range pkgMethods[callee] {
			if decl != handler && callsRequireHuman(decl.Body) {
				return handler.Name.Name + " → " + callee
			}
		}
	}
	return ""
}

// isHTTPHandler tells the handler apart from a store method of the same name.
func isHTTPHandler(fn *ast.FuncDecl) bool {
	params := fn.Type.Params.List
	if len(params) == 0 {
		return false
	}
	sel, ok := params[0].Type.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "ResponseWriter"
}

func calledMethodNames(body *ast.BlockStmt) []string {
	var names []string
	ast.Inspect(body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				names = append(names, sel.Sel.Name)
			}
		}
		return true
	})
	return names
}

func callsRequireHuman(body *ast.BlockStmt) bool {
	calls := false
	ast.Inspect(body, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == "RequireHuman" {
			calls = true
		}
		return !calls
	})
	return calls
}
