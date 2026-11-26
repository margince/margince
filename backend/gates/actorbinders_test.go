// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

// A principal is not always built where its id is written. SystemActing builds
// a system principal from the name its caller passes. The id sits at the call
// site, and the literal holds only a parameter. So a function that sets a
// principal's ID from a parameter is a binder. The actor-kind census reads
// each call to one as a principal of the binder's type, with the argument as
// its id.

import (
	"go/ast"
	"io/fs"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// actorBinder is a function that binds a principal of a written type with the
// id one of its parameters carries.
type actorBinder struct {
	name  string
	typ   string
	param int
}

// binderCandidate is one plain function, with the file and package it sits in.
type binderCandidate struct {
	pkg  string
	file *ast.File
	fn   *ast.FuncDecl
}

// actorBinders maps "<import path>.<func>" to every binder in the tree. A
// function that passes a parameter on as a binder's id is a binder too. So the
// census follows a name through a helper such as scoutWorkspace to the call
// that writes it down.
func actorBinders(t *testing.T) map[string]actorBinder {
	t.Helper()
	out := map[string]actorBinder{}
	var funcs []binderCandidate
	err := gatekit.WalkDir("internal", func(dir string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return err
		}
		pkg := modulePath + "/" + dir
		files, _ := packageSource(t, dir)
		for _, file := range files {
			qualifier, dotImported := gatekit.ImportedAs(file, principalImportPath)
			// Inside the principal package the type is spelled bare, as a
			// dot import spells it.
			dotImported = dotImported || pkg == principalImportPath
			for _, decl := range file.Decls {
				fn, isFunc := decl.(*ast.FuncDecl)
				if !isFunc || fn.Recv != nil || fn.Body == nil {
					continue
				}
				funcs = append(funcs, binderCandidate{pkg: pkg, file: file, fn: fn})
				if qualifier == "" && !dotImported {
					continue
				}
				if binder, ok := binderIn(fn, qualifier, dotImported); ok {
					binder.name = pkg + "." + fn.Name.Name
					out[binder.name] = binder
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for passOn(funcs, out) {
	}
	return out
}

// passOn adds every function that hands a parameter to a known binder as its
// id, and reports whether it added one. Repeated until nothing is added.
func passOn(funcs []binderCandidate, binders map[string]actorBinder) bool {
	added := false
	for _, f := range funcs {
		name := f.pkg + "." + f.fn.Name.Name
		if _, known := binders[name]; known {
			continue
		}
		params := paramPositions(f.fn)
		ast.Inspect(f.fn.Body, func(n ast.Node) bool {
			call, isCall := n.(*ast.CallExpr)
			if !isCall {
				return true
			}
			binder, ok := calledBinder(f.file, f.pkg, call.Fun, binders)
			if !ok || binder.param >= len(call.Args) {
				return true
			}
			arg, isIdent := call.Args[binder.param].(*ast.Ident)
			if !isIdent {
				return true
			}
			if index, isParam := params[arg.Name]; isParam {
				binders[name] = actorBinder{name: name, typ: binder.typ, param: index}
				added = true
				return false
			}
			return true
		})
	}
	return added
}

// paramPositions maps each parameter name of fn to its position.
func paramPositions(fn *ast.FuncDecl) map[string]int {
	params := map[string]int{}
	index := 0
	for _, field := range fn.Type.Params.List {
		for _, name := range field.Names {
			params[name.Name] = index
			index++
		}
		if len(field.Names) == 0 {
			index++
		}
	}
	return params
}

// binderIn finds a principal literal in fn whose ID is one of fn's parameters
// and whose type is written down.
func binderIn(fn *ast.FuncDecl, qualifier string, dotImported bool) (actorBinder, bool) {
	params := paramPositions(fn)
	if binder, ok := reboundID(fn, params); ok {
		return binder, true
	}
	var found actorBinder
	ok := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		lit, isLit := n.(*ast.CompositeLit)
		if ok || !isLit || !namesAPrincipal(lit.Type, qualifier, dotImported) {
			return !ok
		}
		typ := principalFieldType(lit)
		for _, element := range lit.Elts {
			field, isField := element.(*ast.KeyValueExpr)
			if !isField {
				continue
			}
			key, isKey := field.Key.(*ast.Ident)
			value, isIdent := field.Value.(*ast.Ident)
			if !isKey || key.Name != "ID" || !isIdent {
				continue
			}
			if param, isParam := params[value.Name]; isParam && typ != "" {
				found, ok = actorBinder{typ: typ, param: param}, true
			}
		}
		return !ok
	})
	return found, ok
}

// reboundID finds the other spelling of a binder: a principal read from the
// context, with its ID assigned from a parameter. The function holds the
// principal to one type (`if actor.Type != principal.PrincipalSystem`).
func reboundID(fn *ast.FuncDecl, params map[string]int) (actorBinder, bool) {
	param, typ := -1, ""
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.AssignStmt:
			if len(node.Lhs) != 1 || len(node.Rhs) != 1 {
				return true
			}
			target, isSel := node.Lhs[0].(*ast.SelectorExpr)
			value, isIdent := node.Rhs[0].(*ast.Ident)
			if isSel && isIdent && target.Sel.Name == "ID" {
				if index, isParam := params[value.Name]; isParam {
					param = index
				}
			}
		case *ast.BinaryExpr:
			if field, isSel := node.X.(*ast.SelectorExpr); isSel && field.Sel.Name == "Type" {
				typ = principalTypeName(node.Y)
			}
		}
		return true
	})
	return actorBinder{typ: typ, param: param}, param >= 0 && typ != ""
}

// principalTypeName is the PrincipalX an expression names, or empty.
func principalTypeName(expr ast.Expr) string {
	name := ""
	switch typed := expr.(type) {
	case *ast.SelectorExpr:
		name = typed.Sel.Name
	case *ast.Ident:
		name = typed.Name
	}
	if !strings.HasPrefix(name, "Principal") {
		return ""
	}
	return name
}

// binderCalls reads every call in a package to a binder as the principal it
// binds. A call whose id argument cannot be folded still counts as read, with
// no kind to judge.
func binderCalls(dir string, files map[string]*ast.File, constants, kinds map[string]string, binders map[string]actorBinder) []writtenActorID {
	pkg := modulePath + "/" + dir
	var out []writtenActorID
	for path, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			call, isCall := n.(*ast.CallExpr)
			if !isCall {
				return true
			}
			binder, ok := calledBinder(file, pkg, call.Fun, binders)
			if !ok || binder.param >= len(call.Args) {
				return true
			}
			out = append(out, writtenActorID{
				file: path, typ: binder.typ, binder: binder.name,
				kind: kindOf(call.Args[binder.param], constants, kinds),
			})
			return true
		})
	}
	return out
}

// calledBinder resolves a call's function to a binder, in this package or
// through the import that names it.
func calledBinder(file *ast.File, pkg string, fun ast.Expr, binders map[string]actorBinder) (actorBinder, bool) {
	switch callee := fun.(type) {
	case *ast.Ident:
		binder, ok := binders[pkg+"."+callee.Name]
		return binder, ok
	case *ast.SelectorExpr:
		qualifier, isIdent := callee.X.(*ast.Ident)
		if !isIdent {
			return actorBinder{}, false
		}
		for key, binder := range binders {
			at := strings.LastIndex(key, ".")
			if key[at+1:] != callee.Sel.Name {
				continue
			}
			if imported, _ := gatekit.ImportedAs(file, key[:at]); imported == qualifier.Name {
				return binder, true
			}
		}
	}
	return actorBinder{}, false
}

// boundKinds maps each name a package binds to the actor kind its value starts
// with. The whole value may not fold: `"agent:" + kind`, with the kind declared
// in another package, is still an agent id.
func boundKinds(files map[string]*ast.File, constants map[string]string) map[string]string {
	bound := map[string][]ast.Expr{}
	for _, file := range files {
		indexBindings(file, bound)
	}
	kinds := map[string]string{}
	for name, exprs := range bound {
		for _, expr := range exprs {
			if prefix := actorKindPrefix(expr, constants); prefix != "" {
				kinds[name] = prefix
			}
		}
	}
	return kinds
}

// kindOf is actorKindPrefix, falling back to the kind a name was bound with.
func kindOf(expr ast.Expr, constants, kinds map[string]string) string {
	if prefix := actorKindPrefix(expr, constants); prefix != "" {
		return prefix
	}
	if ident, isIdent := expr.(*ast.Ident); isIdent {
		return kinds[ident.Name]
	}
	return ""
}

// systemActing is the binder every scheduled pass names itself through.
const systemActing = principalImportPath + ".SystemActing"

// systemActingCallsInText counts the calls to principal.SystemActing that a
// plain text search finds in the files the census reads. It is a second
// reading the AST one must agree with, so a binder reader that stops matching
// fails instead of reading a smaller tree.
func systemActingCallsInText(t *testing.T) int {
	t.Helper()
	count := 0
	err := gatekit.WalkDir("internal", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") || isIntegrationTagged(path) {
			return err
		}
		source, err := gatekit.ReadFile(path)
		if err != nil {
			return err
		}
		for line := range strings.SplitSeq(string(source), "\n") {
			if !strings.HasPrefix(strings.TrimSpace(line), "//") {
				count += strings.Count(line, "principal.SystemActing(")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return count
}
