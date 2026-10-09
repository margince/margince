// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/agentbundle"
)

// A handler refuses every passport when it, or one call down, calls auth.RequireHuman or
// answers the missing session identity with a refusal: a passport never binds that identity.
// So the skill must not list it. The router calls the operationId with its first letter raised.
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
	sessionReaders, err := sessionIdentityReaders(filepath.Join("..", "modules", "identity"))
	if err != nil {
		t.Fatalf("scanning the identity module: %v", err)
	}
	if len(sessionReaders) == 0 {
		t.Fatal("found no function in the identity module that reads the session Identity from a context; the scan for handlers that refuse without one would check nothing")
	}
	refusesAgents := func(body *ast.BlockStmt) string {
		if callsRequireHuman(body) {
			return "calls auth.RequireHuman"
		}
		if reader := refusesWithoutSession(body, sessionReaders); reader != "" {
			return "refuses a caller " + reader + " finds no session for"
		}
		return ""
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
				if why := requiresHuman(handler, byName, refusesAgents); why != "" {
					t.Errorf("the skill lists %s, and in %s %s, so a passport is always refused",
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

// requiresHuman names the handler, or the method of its package it calls, whose body refuses agents, and why.
// A parse has no types, so a callee matches any same-named method of the package, and other packages go unread.
func requiresHuman(handler *ast.FuncDecl, pkgMethods map[string][]*ast.FuncDecl, refusesAgents func(*ast.BlockStmt) string) string {
	if why := refusesAgents(handler.Body); why != "" {
		return handler.Name.Name + " " + why
	}
	for _, callee := range calledMethodNames(handler.Body) {
		for _, decl := range pkgMethods[callee] {
			if why := refusesAgents(decl.Body); decl != handler && why != "" {
				return handler.Name.Name + " → " + callee + " " + why
			}
		}
	}
	return ""
}

// sessionIdentityReaders names the functions of the identity module that return
// (Identity, bool) from a context value: the session identity a passport never binds.
func sessionIdentityReaders(dir string) (map[string]bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	readers := map[string]bool{}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Body != nil &&
				returnsIdentityAndOK(fn.Type) && len(calledSelectors(fn.Body, "Value")) > 0 {
				readers[fn.Name.Name] = true
			}
		}
	}
	return readers, nil
}

func returnsIdentityAndOK(fn *ast.FuncType) bool {
	if fn.Results == nil || len(fn.Results.List) != 2 {
		return false
	}
	first, okFirst := fn.Results.List[0].Type.(*ast.Ident)
	second, okSecond := fn.Results.List[1].Type.(*ast.Ident)
	return okFirst && okSecond && first.Name == "Identity" && second.Name == "bool"
}

// refusesWithoutSession names the reader whose ok result body branches on as `!ok`.
// A body that only uses the identity when present (`if id, ok := reader(); ok`) serves agents too.
func refusesWithoutSession(body *ast.BlockStmt, readers map[string]bool) string {
	okFrom := map[string]string{}
	refusedBy := ""
	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.AssignStmt:
			if len(node.Lhs) != 2 || len(node.Rhs) != 1 {
				return true
			}
			call, isCall := node.Rhs[0].(*ast.CallExpr)
			okVar, isVar := node.Lhs[1].(*ast.Ident)
			if !isCall || !isVar {
				return true
			}
			if fn, isIdent := call.Fun.(*ast.Ident); isIdent && readers[fn.Name] {
				okFrom[okVar.Name] = fn.Name
			}
		case *ast.UnaryExpr:
			if operand, isIdent := node.X.(*ast.Ident); isIdent && node.Op == token.NOT && okFrom[operand.Name] != "" {
				refusedBy = okFrom[operand.Name]
			}
		}
		return refusedBy == ""
	})
	return refusedBy
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
	return calledSelectors(body, "")
}

// calledSelectors lists the selector calls in body, only those named name when name is set.
func calledSelectors(body *ast.BlockStmt, name string) []string {
	var names []string
	ast.Inspect(body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && (name == "" || sel.Sel.Name == name) {
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
