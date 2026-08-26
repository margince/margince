// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strings"
	"testing"
)

// Correcting a profile field and confirming one are the same write with a
// different provenance, and they are believed to be the same because they are
// literally the same call. Four effects ride on it — the provenance flip, the
// canonical-column write, the audit image and the event — and a verb that grows
// its own copy of any one of them makes the two disagree about what happened,
// while both still return a well-formed field.
//
// The forbidden set is DERIVED from writeProfileField's own callees rather than
// listed here. Whatever it reaches is what a verb must not reach around it, so
// a fifth effect added inside it is protected the day it is added and nobody
// has to remember this file. Listing the primitives by hand would mean the gate
// protects the four that existed when it was written.

const profileFieldOnePath = "writeProfileField"

const profileFieldFile = "organization_profile_field_write.go"

func TestBothProfileFieldVerbsWriteThroughTheOnePath(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, profileFieldFile, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", profileFieldFile, err)
	}
	onePath := funcNamed(file, profileFieldOnePath)
	if onePath == nil {
		t.Fatalf("%s declares no %s, so this gate judged nothing", profileFieldFile, profileFieldOnePath)
	}
	reserved := directCallees(onePath)
	if len(reserved) == 0 {
		t.Fatalf("%s reaches nothing, so there is no effect for a verb to duplicate — either the "+
			"write moved out of it or the callee walk is broken", profileFieldOnePath)
	}
	verbs := 0
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil || !fn.Name.IsExported() || fn.Recv == nil {
			continue
		}
		verbs++
		if !callsAny(fn, receiverName(fn), map[string]bool{profileFieldOnePath: true}) {
			t.Errorf("%s does not go through %s.\n\nCorrecting and confirming are the same write "+
				"with a different provenance; a verb that writes on its own makes them disagree "+
				"about what happened while both still answer with a well-formed field.",
				fn.Name.Name, profileFieldOnePath)
			continue
		}
		if around := reachedAround(fn, reserved); len(around) > 0 {
			t.Errorf("%s goes through %s AND reaches %s directly.\n\nThose are effects the one "+
				"path owns. Reaching one around it is how the correct and the confirm come to "+
				"record different things: move the work inside %s.",
				fn.Name.Name, profileFieldOnePath, strings.Join(around, ", "), profileFieldOnePath)
		}
	}
	if verbs < 2 {
		t.Errorf("%s declares %d exported verb(s); the claim is that BOTH take one path, and one "+
			"verb cannot disagree with itself", profileFieldFile, verbs)
	}
}

// directCallees names what fn calls, by the selector's final identifier — which
// is how a call reads at the site whether it is a method on the receiver, a
// package function, or a method on a value it holds.
func directCallees(fn *ast.FuncDecl) map[string]bool {
	callees := map[string]bool{}
	if fn.Body == nil {
		return callees
	}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch target := call.Fun.(type) {
		case *ast.SelectorExpr:
			callees[target.Sel.Name] = true
		case *ast.Ident:
			callees[target.Name] = true
		}
		return true
	})
	return callees
}

// reachedAround reports which of the one path's own callees fn reaches
// directly, sorted so a failure reads the same on every run.
func reachedAround(fn *ast.FuncDecl, reserved map[string]bool) []string {
	reached := directCallees(fn)
	var around []string
	for name := range reserved {
		if reached[name] {
			around = append(around, name)
		}
	}
	sort.Strings(around)
	return around
}

func funcNamed(file *ast.File, name string) *ast.FuncDecl {
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name != nil && fn.Name.Name == name {
			return fn
		}
	}
	return nil
}
