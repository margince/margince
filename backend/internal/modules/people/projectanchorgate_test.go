// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// A project is readable across the workspace, so the role grant `project.update`
// says a seat may change PROJECTS — not that it may change THIS one. The row
// half is what closes the difference, and a stakeholder verb that skips it lets
// any seat holding the grant rewrite any team's roster. Nothing else refuses:
// the person exists, the project exists, the write succeeds.
//
// The subject is derived from the signature rather than listed. A verb is a
// method that takes a project to act on, so a third one added tomorrow is
// judged without anyone remembering this file. A read that legitimately wants
// visibility rather than writability will fail here — correctly, because it
// should say so where it is, rather than being quietly absent from a list.

const projectAnchorGate = "ensureProjectWritable"

// stakeholderFile is where the verbs and their gate live together. The claim is
// about that pairing, so the file is the honest unit: a verb that moves out of
// it moves out of the claim, and this fails rather than silently covering less.
const stakeholderFile = "projectstakeholder.go"

// projectIDType is what makes a method a verb ON a project rather than one that
// merely mentions them.
const projectIDType = "ids.ProjectID"

func TestEveryProjectStakeholderVerbTakesTheRowAnchorGate(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, stakeholderFile, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", stakeholderFile, err)
	}
	gated := gatedHelpers(file)
	verbs := 0
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil || !fn.Name.IsExported() || !takesA(fn, projectIDType) {
			continue
		}
		verbs++
		if reachesGate(fn, gated) {
			continue
		}
		t.Errorf("%s in %s takes a project to act on and never reaches %s.\n\n"+
			"`project.update` says the seat may change projects, not that it may change this "+
			"one, and a project is readable across the whole workspace — so without the row "+
			"anchor any seat holding the grant can rewrite any team's roster, and the write "+
			"succeeds. Take the gate, or, if this is a read, say so and take the visibility "+
			"check instead.", fn.Name.Name, stakeholderFile, projectAnchorGate)
	}
	if verbs == 0 {
		t.Errorf("no exported method in %s takes a project, so this gate judged nothing — the "+
			"verbs have moved and the claim on %s no longer describes this file",
			stakeholderFile, projectAnchorGate)
	}
}

// gatedHelpers names the file's own unexported methods that reach the gate, so
// a verb calling one of them is covered. One hop, not a full call graph: a verb
// two removes from its own authorization check is worth failing over.
func gatedHelpers(file *ast.File) map[string]bool {
	gated := map[string]bool{projectAnchorGate: true}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil || fn.Name.IsExported() {
			continue
		}
		if callsAny(fn, map[string]bool{projectAnchorGate: true}) {
			gated[fn.Name.Name] = true
		}
	}
	return gated
}

func reachesGate(fn *ast.FuncDecl, gated map[string]bool) bool {
	return callsAny(fn, gated)
}

// callsAny reports whether fn's body calls any of names as a method on its own
// receiver. Method calls only: a free function sharing the name would be a
// different subject, and matching it would report coverage that is not there.
func callsAny(fn *ast.FuncDecl, names map[string]bool) bool {
	if fn.Body == nil {
		return false
	}
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && names[sel.Sel.Name] {
			found = true
		}
		return !found
	})
	return found
}

func typeText(expr ast.Expr) string {
	switch node := expr.(type) {
	case *ast.Ident:
		return node.Name
	case *ast.SelectorExpr:
		return typeText(node.X) + "." + node.Sel.Name
	case *ast.StarExpr:
		return typeText(node.X)
	}
	return ""
}
