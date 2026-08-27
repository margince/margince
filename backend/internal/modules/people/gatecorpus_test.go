// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"sync"
	"testing"
)

// The corpus the gates in this package judge, and the recognisers they judge it
// with.
//
// Both are here for the same reason. A gate that walks the module its own way
// is a second copy of the walk, and the two drift until one of them reads a
// smaller tree than the other while still reporting PASS. A gate that decides
// "does this function build a T" its own way is a second copy of the same
// question, and every copy sees only the spellings its author happened to
// write: a review of seven gates in this package found each recognising the
// literal its author had in front of them and none recognising `var x T`,
// `new(T)`, or a zero literal filled in afterwards.
//
// So there is one walk and one set of recognisers, and a gate says what it is
// asking rather than how to look for it.

// moduleFile is one of the module's own non-test sources, parsed.
type moduleFile struct {
	name string
	fset *token.FileSet
	file *ast.File
}

var (
	corpusOnce  sync.Once
	corpusFiles []moduleFile
	corpusErr   error
)

// moduleFiles parses the module's own non-test sources, once per test binary.
//
// Test sources are left out deliberately: a literal a test builds is a fixture,
// and holding a fixture to the shape of a shipped surface would refuse cases
// written to exercise one field at a time.
func moduleFiles(t *testing.T) []moduleFile {
	t.Helper()
	corpusOnce.Do(func() {
		entries, err := os.ReadDir(".")
		if err != nil {
			corpusErr = err
			return
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			fset := token.NewFileSet()
			file, perr := parser.ParseFile(fset, name, nil, parser.ParseComments)
			if perr != nil {
				corpusErr = perr
				return
			}
			corpusFiles = append(corpusFiles, moduleFile{name: name, fset: fset, file: file})
		}
	})
	if corpusErr != nil {
		t.Fatalf("reading the module's sources: %v", corpusErr)
	}
	// A census that reads nothing is the one failure that looks like success,
	// so the corpus refuses to be empty rather than letting each caller
	// remember to check.
	if len(corpusFiles) == 0 {
		t.Fatal("the module directory holds no non-test Go source, so every gate reading this " +
			"corpus judged nothing")
	}
	return corpusFiles
}

// forEachModuleFile hands each parsed source to visit.
func forEachModuleFile(t *testing.T, visit func(name string, fset *token.FileSet, file *ast.File)) {
	t.Helper()
	for _, parsed := range moduleFiles(t) {
		visit(parsed.name, parsed.fset, parsed.file)
	}
}

// forEachModuleFunc hands each function declaration with a body to visit.
// Every gate here that judges functions wants exactly this population, and
// spelling the decl loop at each one is how a gate comes to skip method values,
// or generic declarations, that another gate reads.
func forEachModuleFunc(t *testing.T, visit func(parsed moduleFile, fn *ast.FuncDecl)) {
	t.Helper()
	for _, parsed := range moduleFiles(t) {
		for _, decl := range parsed.file.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			if !isFunc || fn.Body == nil || fn.Name == nil {
				continue
			}
			visit(parsed, fn)
		}
	}
}

// takesA reports whether fn is handed the named type — as a parameter, or as
// its receiver. Both are what make a body's reads and literals be ABOUT that
// type rather than about some other value that happens to share a field.
//
// The receiver is not a special case: a method ON the type reads its fields
// with the same authority a function taking one does, and a gate that read only
// parameters would let a second opinion be written as a method and stay
// invisible.
func takesA(fn *ast.FuncDecl, typeName string) bool {
	if fn.Recv != nil {
		for _, field := range fn.Recv.List {
			if typeText(field.Type) == typeName {
				return true
			}
		}
	}
	if fn.Type.Params == nil {
		return false
	}
	for _, param := range fn.Type.Params.List {
		if typeText(param.Type) == typeName {
			return true
		}
	}
	return false
}

// constructs reports whether body produces a POPULATED value of the named type.
//
// Populated, not merely mentioned, and by any of the spellings Go offers:
//
//	T{Field: v}          a literal with elements
//	&T{Field: v}         the same, addressed
//	var x T; x.F = v     a zero value filled in afterwards
//	x := T{}; x.F = v    the same, written as a literal
//	x := new(T); x.F = v the same, written as a pointer
//
// A bare zero value with nothing assigned to it is NOT construction: it is the
// error return every function that can fail has, and counting it would report
// every error path as a second derivation. What separates the two is whether a
// field is ever set on it, which is the question a reader asks too.
func constructs(body *ast.BlockStmt, typeName string) bool {
	if populatedLiteral(body, typeName) {
		return true
	}
	zeroed := zeroValuesOf(body, typeName)
	if len(zeroed) == 0 {
		return false
	}
	return anyFieldAssignedTo(body, zeroed)
}

// populatedLiteral reports a composite literal of the type carrying at least
// one element.
func populatedLiteral(body *ast.BlockStmt, typeName string) bool {
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		lit, isLit := node.(*ast.CompositeLit)
		if isLit && lit.Type != nil && typeText(lit.Type) == typeName && len(lit.Elts) > 0 {
			found = true
		}
		return !found
	})
	return found
}

// zeroValuesOf names the local variables that start life as a zero value of the
// type: `var x T`, `x := T{}`, and `x := new(T)`.
func zeroValuesOf(body *ast.BlockStmt, typeName string) map[string]bool {
	zeroed := map[string]bool{}
	ast.Inspect(body, func(node ast.Node) bool {
		switch stmt := node.(type) {
		case *ast.DeclStmt:
			gen, isGen := stmt.Decl.(*ast.GenDecl)
			if !isGen || gen.Tok != token.VAR {
				return true
			}
			for _, spec := range gen.Specs {
				value, isValue := spec.(*ast.ValueSpec)
				if !isValue || value.Type == nil || typeText(value.Type) != typeName {
					continue
				}
				for _, name := range value.Names {
					zeroed[name.Name] = true
				}
			}
		case *ast.AssignStmt:
			for i, rhs := range stmt.Rhs {
				if i >= len(stmt.Lhs) || !isZeroValueOf(rhs, typeName) {
					continue
				}
				if name, isIdent := stmt.Lhs[i].(*ast.Ident); isIdent {
					zeroed[name.Name] = true
				}
			}
		}
		return true
	})
	return zeroed
}

// isZeroValueOf reports whether expr is `T{}` or `new(T)`.
func isZeroValueOf(expr ast.Expr, typeName string) bool {
	switch value := expr.(type) {
	case *ast.CompositeLit:
		return value.Type != nil && typeText(value.Type) == typeName && len(value.Elts) == 0
	case *ast.UnaryExpr:
		return value.Op == token.AND && isZeroValueOf(value.X, typeName)
	case *ast.CallExpr:
		ident, isIdent := value.Fun.(*ast.Ident)
		if !isIdent || ident.Name != "new" || len(value.Args) != 1 {
			return false
		}
		return typeText(value.Args[0]) == typeName
	}
	return false
}

// anyFieldAssignedTo reports whether any of the named variables has a field
// written to it — `x.Field = v`, `x.Field += v`, and the same through a
// pointer, which parses identically.
func anyFieldAssignedTo(body *ast.BlockStmt, names map[string]bool) bool {
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		assign, isAssign := node.(*ast.AssignStmt)
		if !isAssign {
			return true
		}
		for _, lhs := range assign.Lhs {
			sel, isSel := lhs.(*ast.SelectorExpr)
			if !isSel {
				continue
			}
			if base, isIdent := sel.X.(*ast.Ident); isIdent && names[base.Name] {
				found = true
			}
		}
		return !found
	})
	return found
}

// mutatesResultOf reports whether body takes the value the named function
// returns and then writes a field on it.
//
// This is the shape neither "builds a T" nor "guards a T" can see, and it is a
// derivation just the same: a function that calls the one builder and then
// changes what came back has produced a second answer, in a form that reads at
// the call site as though it were still the first.
func mutatesResultOf(body *ast.BlockStmt, callee string) bool {
	held := map[string]bool{}
	ast.Inspect(body, func(node ast.Node) bool {
		assign, isAssign := node.(*ast.AssignStmt)
		if !isAssign {
			return true
		}
		for _, rhs := range assign.Rhs {
			if !callsNamed(rhs, callee) {
				continue
			}
			// A multi-value call binds its results positionally; the value is
			// whichever name sits where the type does, and taking all of them
			// costs nothing because a field write to an error or an id does
			// not parse.
			for _, lhs := range assign.Lhs {
				if name, isIdent := lhs.(*ast.Ident); isIdent && name.Name != "_" {
					held[name.Name] = true
				}
			}
		}
		return true
	})
	if len(held) == 0 {
		return false
	}
	return anyFieldAssignedTo(body, held)
}

// callsNamed reports whether expr is a call of the named function, written
// bare or as a selector — `leadPersonCandidate(…)` and `s.leadPersonCandidate(…)`
// are the same function reached two ways.
func callsNamed(expr ast.Expr, name string) bool {
	call, isCall := expr.(*ast.CallExpr)
	if !isCall {
		return false
	}
	switch target := call.Fun.(type) {
	case *ast.Ident:
		return target.Name == name
	case *ast.SelectorExpr:
		return target.Sel != nil && target.Sel.Name == name
	}
	return false
}
