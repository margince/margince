// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

// A principal is not always built where its id is written. SystemActing builds
// a system principal from the name its caller passes. The id sits at the call
// site, and the literal holds only a parameter. So a function that sets a
// principal's ID from a parameter, or from a field of one, is a binder. The
// actor-kind census reads each call to one as a principal of the binder's
// type. Its id is the argument, or that field of it.

import (
	"go/ast"
	"io/fs"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// actorBinder is one principal a function binds with the id a parameter
// carries. field names the parameter's field the id is read from, such as
// colleagueJob.books; it is empty when the parameter is the id itself.
type actorBinder struct {
	name  string
	typ   string
	param int
	field string
}

// binderSet maps "<import path>.<func>" to every principal that function
// binds from its parameters.
type binderSet map[string][]actorBinder

// add records b for its function, once, and reports whether it was new.
func (set binderSet) add(b actorBinder) bool {
	if slices.Contains(set[b.name], b) {
		return false
	}
	set[b.name] = append(set[b.name], b)
	return true
}

// binderCandidate is one plain function, with the file and package it sits in.
type binderCandidate struct {
	pkg  string
	file *ast.File
	fn   *ast.FuncDecl
}

// actorBinders finds every binder in the tree.
func actorBinders(t *testing.T) binderSet {
	t.Helper()
	var funcs []binderCandidate
	err := gatekit.WalkDir("internal", func(dir string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return err
		}
		files, _ := packageSource(t, dir)
		funcs = append(funcs, candidatesIn(modulePath+"/"+dir, files)...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return bindersAmong(funcs)
}

// candidatesIn lists the plain functions of one package.
func candidatesIn(pkg string, files map[string]*ast.File) []binderCandidate {
	var out []binderCandidate
	for _, file := range files {
		for _, decl := range file.Decls {
			if fn, isFunc := decl.(*ast.FuncDecl); isFunc && fn.Recv == nil && fn.Body != nil {
				out = append(out, binderCandidate{pkg: pkg, file: file, fn: fn})
			}
		}
	}
	return out
}

// bindersAmong finds the functions that bind a principal from a parameter.
// A function that passes a parameter on as a binder's id is a binder too. So
// the census follows a name through a helper such as forColleague to the call
// that writes it down.
func bindersAmong(funcs []binderCandidate) binderSet {
	set := binderSet{}
	for _, f := range funcs {
		qualifier, dotImported := gatekit.ImportedAs(f.file, principalImportPath)
		// Inside the principal package the type is spelled bare, as a dot
		// import spells it.
		dotImported = dotImported || f.pkg == principalImportPath
		if qualifier == "" && !dotImported {
			continue
		}
		for _, b := range bindersIn(f.fn, qualifier, dotImported) {
			b.name = f.pkg + "." + f.fn.Name.Name
			set.add(b)
		}
	}
	for passOn(funcs, set) {
		continue
	}
	return set
}

// passOn adds every function that hands a parameter, or a field of one, to a
// known binder as its id. It reports whether it added one, and is repeated
// until nothing is added.
func passOn(funcs []binderCandidate, set binderSet) bool {
	added := false
	for _, f := range funcs {
		name := f.pkg + "." + f.fn.Name.Name
		params := paramPositions(f.fn)
		ast.Inspect(f.fn.Body, func(n ast.Node) bool {
			call, isCall := n.(*ast.CallExpr)
			if !isCall {
				return true
			}
			for _, b := range calledBinders(f.file, f.pkg, call.Fun, set) {
				if b.param >= len(call.Args) {
					continue
				}
				param, field, ok := binderArgRef(call.Args[b.param], params)
				if !ok || (field != "" && b.field != "") {
					continue
				}
				if field == "" {
					field = b.field
				}
				if set.add(actorBinder{name: name, typ: b.typ, param: param, field: field}) {
					added = true
				}
			}
			return true
		})
	}
	return added
}

// binderArgRef reads an expression as a parameter, or a field of one.
func binderArgRef(expr ast.Expr, params map[string]int) (param int, field string, ok bool) {
	switch node := expr.(type) {
	case *ast.Ident:
		param, ok = params[node.Name]
		return param, "", ok
	case *ast.SelectorExpr:
		base, isIdent := node.X.(*ast.Ident)
		if !isIdent {
			return 0, "", false
		}
		param, ok = params[base.Name]
		return param, node.Sel.Name, ok
	}
	return 0, "", false
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

// bindersIn finds every principal literal in fn whose ID is one of fn's
// parameters or a field of one, and whose type is written down.
func bindersIn(fn *ast.FuncDecl, qualifier string, dotImported bool) []actorBinder {
	params := paramPositions(fn)
	var found []actorBinder
	if b, ok := reboundID(fn, params); ok {
		found = append(found, b)
	}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		lit, isLit := n.(*ast.CompositeLit)
		if !isLit || !namesAPrincipal(lit.Type, qualifier, dotImported) {
			return true
		}
		typ := principalFieldType(lit)
		for _, element := range lit.Elts {
			field, isField := element.(*ast.KeyValueExpr)
			if !isField {
				continue
			}
			if key, isKey := field.Key.(*ast.Ident); !isKey || key.Name != "ID" || typ == "" {
				continue
			}
			if param, from, ok := binderArgRef(field.Value, params); ok {
				found = append(found, actorBinder{typ: typ, param: param, field: from})
			}
		}
		return true
	})
	return found
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
// binds. A call whose id cannot be folded still counts as read, with no kind
// to judge.
func binderCalls(dir string, files map[string]*ast.File, constants, kinds map[string]string, set binderSet) []writtenActorID {
	pkg := modulePath + "/" + dir
	bound := map[string][]ast.Expr{}
	for _, file := range files {
		indexBindings(file, bound)
	}
	var out []writtenActorID
	for path, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			call, isCall := n.(*ast.CallExpr)
			if !isCall {
				return true
			}
			for _, b := range calledBinders(file, pkg, call.Fun, set) {
				if b.param >= len(call.Args) {
					continue
				}
				out = append(out, writtenActorID{
					file: path, typ: b.typ, binder: b.name,
					kind: kindOf(fieldOf(call.Args[b.param], b.field, bound), constants, kinds),
				})
			}
			return true
		})
	}
	return out
}

// fieldOf is the expression a binder reads its id from. That is the argument
// itself, or the named field of a struct literal the argument is or was bound
// to, such as voiceBuilder. It is nil when no literal is in sight.
func fieldOf(arg ast.Expr, field string, bound map[string][]ast.Expr) ast.Expr {
	if field == "" {
		return arg
	}
	candidates := []ast.Expr{arg}
	if ident, isIdent := arg.(*ast.Ident); isIdent {
		candidates = bound[ident.Name]
	}
	for _, candidate := range candidates {
		if unary, isUnary := candidate.(*ast.UnaryExpr); isUnary {
			candidate = unary.X
		}
		lit, isLit := candidate.(*ast.CompositeLit)
		if !isLit {
			continue
		}
		for _, element := range lit.Elts {
			kv, isKV := element.(*ast.KeyValueExpr)
			if !isKV {
				continue
			}
			if key, isKey := kv.Key.(*ast.Ident); isKey && key.Name == field {
				return kv.Value
			}
		}
	}
	return nil
}

// calledBinders resolves a call's function to its binders, in this package or
// through the import that names it.
func calledBinders(file *ast.File, pkg string, fun ast.Expr, set binderSet) []actorBinder {
	switch callee := fun.(type) {
	case *ast.Ident:
		return set[pkg+"."+callee.Name]
	case *ast.SelectorExpr:
		qualifier, isIdent := callee.X.(*ast.Ident)
		if !isIdent {
			return nil
		}
		for key, binders := range set {
			at := strings.LastIndex(key, ".")
			if key[at+1:] != callee.Sel.Name {
				continue
			}
			if imported, _ := gatekit.ImportedAs(file, key[:at]); imported == qualifier.Name {
				return binders
			}
		}
	}
	return nil
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
	if expr == nil {
		return ""
	}
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
