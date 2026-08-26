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
//
// "Takes a project" has to mean both spellings this module uses. A verb whose
// project arrives inside an input struct is still a verb on that project, and
// recognising only the bare parameter is the way this gate fails worst: it
// examines a smaller file, finds every member of what it did examine gated,
// and reports PASS.
//
// So the file is classified exhaustively rather than filtered. An exported
// method here is a project verb and takes the row anchor, or it is a person
// read and takes the person's own object grant. Something that is neither is
// a spelling the census cannot see, and it fails saying so — because a method
// this gate silently skips is indistinguishable from one it cleared.

const projectAnchorGate = "ensureProjectWritable"

// personReadGate is what the file's other kind of exported method owes: a name
// read is not a write on a project and has no row anchor to take, but it is
// still a read, and one that trusted its argument would be a side door onto
// any id somebody could guess.
const personReadGate = "Require"

// stakeholderFile is where the verbs and their gate live together. The claim is
// about that pairing, so the file is the honest unit: a verb that moves out of
// it moves out of the claim, and this fails rather than silently covering less.
const stakeholderFile = "projectstakeholder.go"

// projectIDType is what makes a method a verb ON a project rather than one that
// merely mentions them — either handed over directly or carried in by an input
// struct that holds one.
const projectIDType = "ids.ProjectID"

func TestEveryProjectStakeholderVerbTakesTheRowAnchorGate(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, stakeholderFile, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", stakeholderFile, err)
	}
	gated := gatedHelpers(file)
	carriers := projectCarryingTypes(t)
	verbs := 0
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil || !fn.Name.IsExported() {
			continue
		}
		if !takesAProject(fn, carriers) {
			// Not skipped: a method the census cannot place is the census
			// being wrong, not the method being exempt. The one thing it may
			// be instead of a project verb is a read, and a read owes its own
			// grant.
			if callsAny(fn, map[string]bool{personReadGate: true}) {
				continue
			}
			t.Errorf("%s in %s names no project, by parameter or by input struct, and takes no "+
				"%s either.\n\nSo it is neither of the two things this file holds. Either it "+
				"reaches a project by a spelling the census cannot see — in which case the gate "+
				"is judging less than it reports — or it is an ungated read.",
				fn.Name.Name, stakeholderFile, personReadGate)
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

// takesAProject reports whether fn is handed a project to act on — as a bare
// ids.ProjectID, or inside an input struct that carries one. The second
// spelling is the module's dominant one for a write verb, and a gate that
// knows only the first holds half the file while reporting all of it.
func takesAProject(fn *ast.FuncDecl, carriers map[string]bool) bool {
	if takesA(fn, projectIDType) {
		return true
	}
	if fn.Type.Params == nil {
		return false
	}
	for _, param := range fn.Type.Params.List {
		if carriers[typeText(param.Type)] {
			return true
		}
	}
	return false
}

// projectCarryingTypes names every struct the module declares that holds an
// ids.ProjectID field. Derived from the tree rather than listed, so an input
// type renamed or introduced tomorrow carries its verb into the census on its
// own.
func projectCarryingTypes(t *testing.T) map[string]bool {
	t.Helper()
	carriers := map[string]bool{}
	forEachModuleFile(t, func(_ string, _ *token.FileSet, file *ast.File) {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok || ts.Name == nil {
					continue
				}
				st, ok := ts.Type.(*ast.StructType)
				if !ok || st.Fields == nil {
					continue
				}
				for _, field := range st.Fields.List {
					if typeText(field.Type) == projectIDType {
						carriers[ts.Name.Name] = true
					}
				}
			}
		}
	})
	if len(carriers) == 0 {
		t.Fatalf("no struct in this module carries an %s field, so the half of the census that "+
			"finds a verb by its input type is examining nothing", projectIDType)
	}
	return carriers
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
